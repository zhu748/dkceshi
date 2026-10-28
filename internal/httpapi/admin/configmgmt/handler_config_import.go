package configmgmt

import (
	"encoding/json"
	"net/http"
	"strings"

	"ds2api/internal/config"
)

func (h *Handler) configImport(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}

	mode := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = strings.TrimSpace(strings.ToLower(fieldString(req, "mode")))
	}
	if mode == "" {
		mode = "merge"
	}
	if mode != "merge" && mode != "replace" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "mode must be merge or replace"})
		return
	}

	payload := req
	if raw, ok := req["config"].(map[string]any); ok && len(raw) > 0 {
		payload = raw
	}
	rawJSON, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid config payload"})
		return
	}
	var incoming config.Config
	if err := json.Unmarshal(rawJSON, &incoming); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	incoming.ClearAccountTokens()

	importedKeys, importedAccounts := 0, 0
	err = h.Store.Update(func(c *config.Config) error {
		next := c.Clone()
		if mode == "replace" {
			next = incoming.Clone()
			next.Accounts = normalizeAndDedupeAccounts(next.Accounts)
			next.VercelSyncHash = c.VercelSyncHash
			next.VercelSyncTime = c.VercelSyncTime
			// tool_marker_secret is a server-generated, never-exported secret:
			// an imported payload cannot carry it, so a replace must keep the
			// running instance's value. Otherwise every API key's tool-call
			// marker would silently change until the next process start
			// regenerates it.
			next.ToolMarkerSecret = c.ToolMarkerSecret
			if len(incoming.Mihomo.Subscriptions) == 0 && len(incoming.Mihomo.PortMap) == 0 {
				// 订阅与端口映射由 mihomo_subscriptions.json 独立维护，
				// 导入的 config 通常不再携带；若未提供则保留现有状态，避免误删。
				next.Mihomo.Subscriptions = c.Mihomo.Subscriptions
				next.Mihomo.PortMap = c.Mihomo.PortMap
			}
			importedKeys = len(next.APIKeys)
			importedAccounts = len(next.Accounts)
		} else {
			var changed int
			next.APIKeys, changed = mergeAPIKeysPreferStructured(next.APIKeys, incoming.APIKeys)
			importedKeys += changed

			existingAccounts := map[string]struct{}{}
			for _, acc := range next.Accounts {
				acc = normalizeAccountForStorage(acc)
				key := accountDedupeKey(acc)
				if key != "" {
					existingAccounts[key] = struct{}{}
				}
			}
			for _, acc := range incoming.Accounts {
				acc = normalizeAccountForStorage(acc)
				key := accountDedupeKey(acc)
				if key == "" {
					continue
				}
				if _, ok := existingAccounts[key]; ok {
					continue
				}
				existingAccounts[key] = struct{}{}
				next.Accounts = append(next.Accounts, acc)
				importedAccounts++
			}

			// Registered settings (internal/config/settings.go) are merged
			// generically: every setting the incoming config actually carries is
			// applied, and one it does not mention is left alone. Before the
			// registry this list was hand-written and had drifted — it merged
			// the Responses TTL but silently dropped the input-file threshold
			// and the two injection prompts.
			mergeRegisteredSettings(&next, incoming)
			// Registered sections (internal/config/sections.go) merge the same
			// way: each one declares its own rule (the admin section keeps its
			// password hash, a form section merges field by field), so a new
			// section cannot be dropped from an import by forgetting it here.
			if err := config.MergeRegisteredSections(&next, incoming); err != nil {
				return newRequestError(err.Error())
			}
		}

		reconcileImportedBindings(&next, incoming, mode)

		normalizeSettingsConfig(&next)
		if err := validateSettingsConfig(next); err != nil {
			return newRequestError(err.Error())
		}

		*c = next
		return nil
	})
	if err != nil {
		if detail, ok := requestErrorDetail(err); ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": detail})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}

	h.Pool.Reset()
	h.Pool.SyncRateLimits()
	h.Pool.SyncDailyLimits()
	if h.OnAccountsChanged != nil {
		h.OnAccountsChanged()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"mode":              mode,
		"imported_keys":     importedKeys,
		"imported_accounts": importedAccounts,
		"message":           "config imported",
	})
}

// reconcileImportedBindings 调和导入配置中的账号节点绑定与代理：
// 在跨设备或备份导入场景下，若账号绑定的 Mihomo 节点在当前系统中不存在，
// 自动清除该账号的绑定（ProxyID 清空、清除 ManualBinding 标记及换号冷却）；
// 若节点存在（通过受管代理 ID 精确匹配，或通过节点名唯一匹配），则正常保留该绑定，
// 确保其本地监听端口与受管代理条目齐备；最后清理不再被引用的孤立受管代理。
func reconcileImportedBindings(next *config.Config, incoming config.Config, mode string) {
	if next == nil {
		return
	}

	// 1. 在 merge 模式下，incoming 中携带的有效自定义代理（非 mihomo 受管代理）
	// 按 ID 去重并入 next.Proxies，避免因代理未合并导致引用校验失败。
	if mode == "merge" {
		existingProxyIDs := make(map[string]struct{}, len(next.Proxies))
		for _, p := range next.Proxies {
			existingProxyIDs[config.NormalizeProxy(p).ID] = struct{}{}
		}
		for _, p := range incoming.Proxies {
			if config.IsMihomoManagedProxyID(p.ID) {
				continue
			}
			np := config.NormalizeProxy(p)
			if np.ID == "" {
				continue
			}
			if _, exists := existingProxyIDs[np.ID]; !exists {
				next.Proxies = append(next.Proxies, p)
				existingProxyIDs[np.ID] = struct{}{}
			}
		}
	}

	// 2. 建立当前目标系统（next.Mihomo）中已知节点的索引。
	type mihomoNodeTarget struct {
		subID   string
		nodeKey string
		proxyID string
		node    config.MihomoNode
	}

	byProxyID := make(map[string]mihomoNodeTarget)
	byNodeName := make(map[string][]mihomoNodeTarget)

	for _, sub := range next.Mihomo.Subscriptions {
		for _, node := range sub.Nodes {
			nodeKey := config.MihomoNodeKey(sub.ID, node.Name)
			proxyID := config.MihomoManagedProxyID(nodeKey)
			target := mihomoNodeTarget{
				subID:   sub.ID,
				nodeKey: nodeKey,
				proxyID: proxyID,
				node:    node,
			}
			byProxyID[proxyID] = target
			byNodeName[node.Name] = append(byNodeName[node.Name], target)
		}
	}

	// 3. 收集原代理列表中受管代理的节点名称，用于跨设备（订阅 ID 变动）时的节点名映射。
	managedProxyNames := make(map[string]string)
	for _, p := range incoming.Proxies {
		id := strings.TrimSpace(p.ID)
		if config.IsMihomoManagedProxyID(id) && strings.TrimSpace(p.Name) != "" {
			managedProxyNames[id] = strings.TrimSpace(p.Name)
		}
	}
	for _, p := range next.Proxies {
		id := strings.TrimSpace(p.ID)
		if config.IsMihomoManagedProxyID(id) && strings.TrimSpace(p.Name) != "" {
			if _, exists := managedProxyNames[id]; !exists {
				managedProxyNames[id] = strings.TrimSpace(p.Name)
			}
		}
	}

	// 4. 检查每个账号的 Mihomo 受管代理绑定
	for i := range next.Accounts {
		proxyID := strings.TrimSpace(next.Accounts[i].ProxyID)
		if proxyID == "" || !config.IsMihomoManagedProxyID(proxyID) {
			continue
		}

		var matched *mihomoNodeTarget

		// 4.1 精确匹配 proxyID（订阅 ID 与节点名均相同）
		if target, ok := byProxyID[proxyID]; ok {
			t := target
			matched = &t
		} else if name, ok := managedProxyNames[proxyID]; ok && name != "" {
			// 4.2 跨设备模糊匹配：通过原始受管代理名称查找当前系统中唯一存在的同名节点
			if candidates, found := byNodeName[name]; found && len(candidates) == 1 {
				t := candidates[0]
				matched = &t
			}
		}

		if matched != nil {
			// 节点存在：正常保留绑定
			targetProxyID := matched.proxyID
			next.Accounts[i].ProxyID = targetProxyID

			// 确保目标系统分配了该节点的端口
			port := next.Mihomo.AllocateMihomoPort(matched.nodeKey)

			// 确保 next.Proxies 包含对应的受管代理条目
			found := false
			for j := range next.Proxies {
				if strings.TrimSpace(next.Proxies[j].ID) == targetProxyID {
					next.Proxies[j].Name = matched.node.Name
					next.Proxies[j].Type = "socks5"
					next.Proxies[j].Host = "127.0.0.1"
					next.Proxies[j].Port = port
					found = true
					break
				}
			}
			if !found {
				next.Proxies = append(next.Proxies, config.Proxy{
					ID:   targetProxyID,
					Name: matched.node.Name,
					Type: "socks5",
					Host: "127.0.0.1",
					Port: port,
				})
			}
		} else {
			// 节点不存在：直接自动清除绑定
			next.Accounts[i].ProxyID = ""
			next.Accounts[i].ManualBinding = false
			next.Accounts[i].NodeCooldownUntil = 0
		}
	}

	// 5. 垃圾回收：清理未被任何账号引用的受管代理条目与孤立端口
	usedManagedIDs := make(map[string]struct{})
	for _, acc := range next.Accounts {
		id := strings.TrimSpace(acc.ProxyID)
		if config.IsMihomoManagedProxyID(id) {
			usedManagedIDs[id] = struct{}{}
		}
	}

	keptProxies := make([]config.Proxy, 0, len(next.Proxies))
	for _, p := range next.Proxies {
		id := strings.TrimSpace(p.ID)
		if config.IsMihomoManagedProxyID(id) {
			if _, ok := usedManagedIDs[id]; !ok {
				continue
			}
		}
		keptProxies = append(keptProxies, p)
	}
	next.Proxies = keptProxies

	if next.Mihomo.PortMap != nil {
		for nodeKey := range next.Mihomo.PortMap {
			proxyID := config.MihomoManagedProxyID(nodeKey)
			if _, ok := usedManagedIDs[proxyID]; !ok {
				delete(next.Mihomo.PortMap, nodeKey)
			}
		}
	}
}

// mergeRegisteredSettings applies every registered setting the incoming config
// carries onto next. It is generic over internal/config/settings.go, so a new
// setting is merged by the registry entry alone.
func mergeRegisteredSettings(next *config.Config, incoming config.Config) {
	for key, fields := range incoming.Settings {
		if len(fields) == 0 {
			continue
		}
		for field, value := range fields {
			next.SetSetting(key, field, value)
		}
	}
}
