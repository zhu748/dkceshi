package mihomo

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"ds2api/internal/config"
)

// 自动巡检调度参数：
//   - initialSweepDelay：服务启动后首轮"测速 + 自动分配"的触发延迟；
//   - watcherTickInterval()：后续定时循环节拍（默认 60s，环境变量
//     DS2API_MIHOMO_INTERVAL 可配置，最小 5s）；
//   - failoverCooldown()：坏节点安全转移后账号的换号冷却期（默认 15 分钟，
//     环境变量 DS2API_MIHOMO_FAILOVER_COOLDOWN_MINUTES 可配置），
//     冷却期内不再自动切换该账号，避免故障节点间反复横跳。
//   - selfHealMinInterval：子进程崩溃后自动重启的节流间隔，避免每拍都尝试拉起。
const (
	initialSweepDelay       = 5 * time.Second
	defaultTickInterval     = 60 * time.Second
	minTickInterval         = 5 * time.Second
	defaultFailoverCooldown = 15 * time.Minute
	selfHealMinInterval     = 60 * time.Second
)

func watcherTickInterval() time.Duration {
	d := defaultTickInterval
	if raw := strings.TrimSpace(os.Getenv("DS2API_MIHOMO_INTERVAL")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			d = time.Duration(secs) * time.Second
		}
	}
	if d < minTickInterval {
		d = minTickInterval
	}
	return d
}

func failoverCooldown() time.Duration {
	d := defaultFailoverCooldown
	if raw := strings.TrimSpace(os.Getenv("DS2API_MIHOMO_FAILOVER_COOLDOWN_MINUTES")); raw != "" {
		if mins, err := strconv.Atoi(raw); err == nil && mins > 0 {
			d = time.Duration(mins) * time.Minute
		}
	}
	return d
}

// findAccountIndexLocked 与 BindAccount 使用同一套账号匹配规则。
func findAccountIndexLocked(c *config.Config, identifier string) int {
	for i, acc := range c.Accounts {
		if accountMatches(acc, identifier) {
			return i
		}
	}
	return -1
}

// assignLocked 在 Store.Update 事务内把账号绑定到节点：
// 分配端口、upsert 托管代理并写回账号 ProxyID。
func assignLocked(c *config.Config, accIdx int, nodeKey string) error {
	node, ok := c.Mihomo.FindMihomoNode(nodeKey)
	if !ok {
		return fmt.Errorf("节点 %s 不存在（订阅可能已被删除）", nodeKey)
	}
	port := c.Mihomo.AllocateMihomoPort(nodeKey)
	proxy := upsertManagedProxyLocked(c, nodeKey, node.Name, port)
	c.Accounts[accIdx].ProxyID = proxy.ID
	return nil
}

// availableCandidates 返回健康可用的节点键，排序规则：
// 已测速 ok 节点按延迟升序在前，未测速（unknown）节点在后，最后按节点键字典序。
// fail 节点一律排除，不进入分配候选池。测速数据直接读订阅节点字段。
func (m *Manager) availableCandidates(snap config.Config) []string {
	type cand struct {
		key     string
		ok      bool
		latency int
	}
	list := make([]cand, 0)
	for _, sub := range snap.Mihomo.Subscriptions {
		for _, node := range sub.Nodes {
			if node.Status == NodeHealthFail {
				continue
			}
			key := config.MihomoNodeKey(sub.ID, node.Name)
			list = append(list, cand{key: key, ok: node.Status == NodeHealthOK, latency: node.LatencyMS})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].ok != list[j].ok {
			return list[i].ok // 测速通过的优先
		}
		if list[i].ok && list[i].latency != list[j].latency {
			return list[i].latency < list[j].latency
		}
		return list[i].key < list[j].key
	})
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.key)
	}
	return out
}

// nodeBoundCounts 统计每个节点键上绑定的【启用】账号数（经托管代理反查）。
// 禁用账号不计数：它们的绑定会被 reconcile 解除，不应影响负载均衡。
func nodeBoundCounts(snap config.Config) map[string]int {
	proxyToNode := map[string]string{}
	for nodeKey := range snap.Mihomo.PortMap {
		proxyToNode[config.MihomoManagedProxyID(nodeKey)] = nodeKey
	}
	counts := map[string]int{}
	for _, acc := range snap.Accounts {
		if acc.Disabled {
			continue
		}
		proxyID := strings.TrimSpace(acc.ProxyID)
		if nodeKey, ok := proxyToNode[proxyID]; ok {
			counts[nodeKey]++
		}
	}
	return counts
}

// proxyNodeMap 返回托管代理 ID -> 节点键的反查表。
func proxyNodeMap(snap config.Config) map[string]string {
	out := map[string]string{}
	for nodeKey := range snap.Mihomo.PortMap {
		out[config.MihomoManagedProxyID(nodeKey)] = nodeKey
	}
	return out
}

// pickLeastLoaded 在候选节点中选择当前绑定账号最少的一个（负载相同取候选序靠前者，
// 即延迟更低者），并把该节点负载 +1。
func pickLeastLoaded(candidates []string, counts map[string]int) string {
	best := candidates[0]
	for _, key := range candidates[1:] {
		if counts[key] < counts[best] {
			best = key
		}
	}
	counts[best]++
	return best
}

// reconcileBindings 基于当前健康池做一次绑定调和（无网络操作）：
//  1. 故障转移：启用账号绑定的节点连续失败达到阈值时，立即切换到负载最低的
//     健康节点，并给账号进入 15 分钟换号冷却期（冷却期内不再自动切换）；
//  2. 弹性补位挂载：启用且尚未配置任何代理的账号（如弹性号池刚补位上线的账号）
//     自动分配一个健康节点；
//  3. 回收：被禁用（弹性号池休眠/封号）账号的托管绑定解除，端口归还可用池。
//
// 手动绑定（ManualBinding）的账号不参与上述任何一项：其绑定完全由用户掌握，
// 节点失效也不会被自动换走（请求直接失败），休眠期间也保留绑定与端口，
// 因此弹性号池的休眠/唤醒不会丢掉用户选定的节点。
//
// 另外校验运行时监听是否落后于配置：手动绑定的账号睡着期间，它保留的绑定不会
// 产生变更，但任何一次别的重新应用写下的监听里都没有它（禁用账号不占端口）。
// 唤醒后配置层面无事可做，只有比对端口能发现落后并补一次重新应用，否则账号会
// 停在"绑定还在、端口没人监听"的状态（见 missingListeners）。
//
// 返回是否发生了配置变更（含为补齐缺失监听而做的重新应用）。
// 仅在 mihomo.enabled && mihomo.auto_bind 时生效。
func (m *Manager) reconcileBindings() bool {
	if m == nil || m.store == nil {
		return false
	}
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	select {
	case <-m.drainStop:
		return false
	default:
	}
	snap := m.store.Snapshot()
	if !snap.Mihomo.Enabled || !snap.Mihomo.AutoBind {
		return false
	}
	health := m.NodeHealthMap()
	threshold := failThreshold()
	now := time.Now().Unix()

	dead := map[string]struct{}{}
	for key, h := range health {
		if h.Status == NodeHealthFail && h.FailStreak >= threshold {
			dead[key] = struct{}{}
		}
	}

	candidates := m.availableCandidates(snap) // 已排除全部 fail 节点
	proxyToNode := proxyNodeMap(snap)
	counts := nodeBoundCounts(snap)
	type rebind struct {
		identifier string
		reason     string
	}
	rebinds := []rebind{}
	backfills := []rebind{}
	unbinds := []string{}
	for _, acc := range snap.Accounts {
		identifier := acc.Identifier()
		if identifier == "" {
			continue
		}
		// 手动绑定的账号整体退出自动调度：不回收（保留绑定与端口）、
		// 不故障转移、不补位。节点失效时由请求自己报错。
		if acc.ManualBinding {
			continue
		}
		if acc.Disabled {
			// 禁用账号不占用端口：解除托管绑定，交给 GC 回收。
			if config.IsMihomoManagedProxyID(strings.TrimSpace(acc.ProxyID)) {
				unbinds = append(unbinds, identifier)
			}
			continue
		}
		proxyID := strings.TrimSpace(acc.ProxyID)
		if config.IsMihomoManagedProxyID(proxyID) {
			nodeKey := proxyToNode[proxyID]
			if _, isDead := dead[nodeKey]; isDead {
				if acc.NodeCooldownUntil > float64(now) {
					// 换号冷却期内不再次自动切换（防止故障节点间反复横跳）。
					continue
				}
				rebinds = append(rebinds, rebind{identifier: identifier, reason: nodeKey})
			}
			continue
		}
		if proxyID == "" {
			// 显式解绑（NoProxy）的账号不补位，尊重用户"不走代理"的选择。
			if acc.NoProxy {
				continue
			}
			backfills = append(backfills, rebind{identifier: identifier})
		}
	}
	// 无健康候选时跳过故障转移/补位（保持现状等待恢复），但禁用账号回收仍然执行。
	if len(candidates) == 0 {
		rebinds = nil
		backfills = nil
	}
	if len(rebinds)+len(backfills)+len(unbinds) == 0 {
		// 没有绑定变更，不代表运行中的监听已经跟上配置：手动绑定的账号
		// 休眠期间被别的 Apply 摘掉监听端口后，唤醒不会产生任何配置变更，
		// 只能靠这里的比对补一次重新应用。
		if missing := m.missingListeners(snap); len(missing) > 0 {
			finish, ok := m.prepareReconcile()
			if !ok {
				return false
			}
			defer finish()
			config.Logger.Info("[mihomo] applied listeners lag behind config, re-applying", "missing_ports", missing)
			if err := m.Apply(context.Background()); err != nil {
				config.Logger.Warn("[mihomo] re-apply for missing listeners failed, will retry next tick", "error", err)
				return false
			}
			return true
		}
		return false
	}

	finish, ready := m.prepareReconcile()
	if !ready {
		return false
	}
	defer finish()
	cooldownSecs := failoverCooldown().Seconds()
	err := m.store.Update(func(c *config.Config) error {
		if !c.Mihomo.Enabled || !c.Mihomo.AutoBind {
			return nil
		}
		for _, id := range unbinds {
			idx := findAccountIndexLocked(c, id)
			if idx < 0 || !c.Accounts[idx].Disabled || c.Accounts[idx].ManualBinding {
				continue
			}
			if c.Accounts[idx].ProxyID != accountProxyInSnapshot(snap, id) {
				continue
			}
			c.Accounts[idx].ProxyID = ""
		}
		for _, rb := range rebinds {
			idx := findAccountIndexLocked(c, rb.identifier)
			if !eligibleReconcileAccount(c, idx, accountProxyInSnapshot(snap, rb.identifier)) {
				continue // 事务间隙账号状态变化，保守跳过
			}
			if err := assignLocked(c, idx, pickLeastLoaded(candidates, counts)); err != nil {
				return err
			}
			// 坏节点安全转移后进入换号冷却期，避免反复横跳。
			c.Accounts[idx].NodeCooldownUntil = float64(now) + cooldownSecs
		}
		for _, rb := range backfills {
			idx := findAccountIndexLocked(c, rb.identifier)
			if !eligibleReconcileAccount(c, idx, "") {
				continue
			}
			if err := assignLocked(c, idx, pickLeastLoaded(candidates, counts)); err != nil {
				return err
			}
		}
		gcLocked(c)
		return validateMutation(c)
	})
	if err != nil {
		config.Logger.Warn("[mihomo] reconcile bindings failed", "error", err)
		return false
	}
	for _, rb := range rebinds {
		config.Logger.Warn("[mihomo] failover: account moved off dead node (entering cooldown)", "account", rb.identifier, "dead_node", rb.reason, "cooldown_minutes", int(cooldownSecs/60))
	}
	for _, rb := range backfills {
		config.Logger.Info("[mihomo] auto bind: node assigned for newly enabled account", "account", rb.identifier)
	}
	for _, id := range unbinds {
		config.Logger.Info("[mihomo] auto unbind: disabled account released its node", "account", id)
	}
	m.resetPool()
	if err := m.Apply(context.Background()); err != nil {
		// 绑定已持久化；应用失败（如另一个 Apply 正在进行）交给下一轮重试。
		config.Logger.Warn("[mihomo] reconcile apply failed, will retry next tick", "error", err)
	}
	return true
}
