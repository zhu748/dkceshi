package configmgmt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/config"
)

// postConfigImport sends one import payload through h and returns the recorder.
func postConfigImport(t *testing.T, h *Handler, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/config/import", strings.NewReader(string(b)))
	rec := httptest.NewRecorder()
	h.configImport(rec, req)
	return rec
}

// auto_delete declares an explicit no-op Merge, so a merge import must never
// turn on post-request deletion of remote conversations: only replace applies
// it. A merge that wrote the imported mode would silently switch on a
// destructive behavior from a backup file.
func TestConfigImportMergeLeavesAutoDeleteAlone(t *testing.T) {
	h := newAdminTestHandler(t, `{"keys":["k1"],"auto_delete":{"mode":"none"}}`)
	if got := h.Store.AutoDeleteMode(); got != "none" {
		t.Fatalf("precondition: AutoDeleteMode()=%q want none", got)
	}

	rec := postConfigImport(t, h, map[string]any{
		"mode": "merge",
		"config": map[string]any{
			"keys":        []any{"k1"},
			"auto_delete": map[string]any{"mode": "all"},
			// A registered setting travels with the same payload, so the test
			// shows the merge ran and skipped auto_delete rather than the whole
			// import doing nothing.
			"responses": map[string]any{"store_ttl_seconds": 600},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := h.Store.SettingInt("responses", "store_ttl_seconds"); got != 600 {
		t.Fatalf("the merge must apply the settings it carries, store_ttl_seconds=%d", got)
	}
	if got := h.Store.AutoDeleteMode(); got != "none" {
		t.Fatalf("a merge import must leave auto_delete alone, AutoDeleteMode()=%q", got)
	}
}

// The replace half of the same contract: replace applies auto_delete the way it
// applies everything else, so a backup restores the configured behavior.
func TestConfigImportReplaceAppliesAutoDelete(t *testing.T) {
	h := newAdminTestHandler(t, `{"keys":["k1"],"auto_delete":{"mode":"none"}}`)

	rec := postConfigImport(t, h, map[string]any{
		"mode": "replace",
		"config": map[string]any{
			"keys":        []any{"k1"},
			"auto_delete": map[string]any{"mode": "all"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := h.Store.AutoDeleteMode(); got != "all" {
		t.Fatalf("replace must apply the imported auto_delete, AutoDeleteMode()=%q", got)
	}
}

// 跨设备导入测试：导入绑定的 Mihomo 节点在当前系统中不存在时，
// 导入应当成功（不报 unknown proxy 400），且自动清除该账号的节点绑定与 manual_binding 标记。
func TestConfigImportCleansMissingMihomoNodeBinding(t *testing.T) {
	h := newAdminTestHandler(t, `{"keys":["k1"]}`)

	missingProxyID := "mihomo-deadbeef01234567"
	rec := postConfigImport(t, h, map[string]any{
		"mode": "merge",
		"config": map[string]any{
			"accounts": []any{
				map[string]any{
					"email":          "user1@test.com",
					"proxy_id":       missingProxyID,
					"manual_binding": true,
				},
			},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	acc, found := h.Store.FindAccount("user1@test.com")
	if !found {
		t.Fatalf("account user1@test.com must be imported")
	}
	if acc.ProxyID != "" {
		t.Fatalf("missing node binding must be cleared, got proxy_id=%q", acc.ProxyID)
	}
	if acc.ManualBinding {
		t.Fatalf("manual_binding flag must be reset to false when binding is cleared")
	}
	if acc.NodeCooldownUntil != 0 {
		t.Fatalf("node_cooldown_until must be reset to 0, got %v", acc.NodeCooldownUntil)
	}

	// 同样验证 replace 模式下节点不存在时的自动清理行为
	recReplace := postConfigImport(t, h, map[string]any{
		"mode": "replace",
		"config": map[string]any{
			"keys": []any{"k1"},
			"accounts": []any{
				map[string]any{
					"email":          "user2@test.com",
					"proxy_id":       missingProxyID,
					"manual_binding": true,
				},
			},
		},
	})
	if recReplace.Code != http.StatusOK {
		t.Fatalf("expected 200 on replace import, got %d body=%s", recReplace.Code, recReplace.Body.String())
	}
	acc2, found2 := h.Store.FindAccount("user2@test.com")
	if !found2 {
		t.Fatalf("account user2@test.com must be imported")
	}
	if acc2.ProxyID != "" {
		t.Fatalf("missing node binding must be cleared on replace, got proxy_id=%q", acc2.ProxyID)
	}
	if acc2.ManualBinding {
		t.Fatalf("manual_binding must be cleared on replace")
	}
}

// 节点存在时测试：导入绑定的 Mihomo 节点在当前系统中存在时，
// 正常保留该绑定，并确保受管代理与本地端口正常就绪。
func TestConfigImportPreservesExistingMihomoNodeBinding(t *testing.T) {
	// 初始化当前系统配置：包含一个订阅 sub-local，节点为 "香港 01"
	subID := "sub-local"
	nodeName := "香港 01"
	nodeKey := subID + "::" + nodeName
	validProxyID := config.MihomoManagedProxyID(nodeKey)

	rawCfg := map[string]any{
		"keys": []any{"k1"},
		"mihomo": map[string]any{
			"enabled":   true,
			"base_port": 10801,
			"subscriptions": []any{
				map[string]any{
					"id":   subID,
					"name": "Local Sub",
					"url":  "https://airport.com/sub",
					"nodes": []any{
						map[string]any{
							"name": nodeName,
							"type": "shadowsocks",
						},
					},
				},
			},
		},
	}
	rawJSON, _ := json.Marshal(rawCfg)
	h := newAdminTestHandler(t, string(rawJSON))

	rec := postConfigImport(t, h, map[string]any{
		"mode": "merge",
		"config": map[string]any{
			"accounts": []any{
				map[string]any{
					"email":          "user_preserved@test.com",
					"proxy_id":       validProxyID,
					"manual_binding": true,
				},
			},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	acc, found := h.Store.FindAccount("user_preserved@test.com")
	if !found {
		t.Fatalf("account user_preserved@test.com must be imported")
	}
	if acc.ProxyID != validProxyID {
		t.Fatalf("existing node binding must be preserved, want %q got %q", validProxyID, acc.ProxyID)
	}
	if !acc.ManualBinding {
		t.Fatalf("manual_binding flag must be preserved")
	}

	// 验证 Proxies 中确实维护了该受管代理
	updatedCfg := h.Store.Snapshot()
	foundProxy := false
	for _, p := range updatedCfg.Proxies {
		if p.ID == validProxyID {
			foundProxy = true
			if p.Port <= 0 {
				t.Fatalf("managed proxy port must be allocated, got %d", p.Port)
			}
			break
		}
	}
	if !foundProxy {
		t.Fatalf("managed proxy %q must exist in updated config.Proxies", validProxyID)
	}
}

// 混合节点测试：部分节点存在，部分不存在。
// 验证存在的保留，不存在的自动清除。
func TestConfigImportMixedNodesAndCallback(t *testing.T) {
	subID := "sub-target"
	nodeName := "日本 01"
	nodeKey := subID + "::" + nodeName
	validProxyID := config.MihomoManagedProxyID(nodeKey)
	invalidProxyID := "mihomo-9999999999999999"

	rawCfg := map[string]any{
		"keys": []any{"k1"},
		"mihomo": map[string]any{
			"enabled":   true,
			"base_port": 10801,
			"subscriptions": []any{
				map[string]any{
					"id":   subID,
					"name": "Target Airport",
					"url":  "https://example.com/sub",
					"nodes": []any{
						map[string]any{
							"name": nodeName,
							"type": "trojan",
						},
					},
				},
			},
		},
	}
	rawJSON, _ := json.Marshal(rawCfg)
	h := newAdminTestHandler(t, string(rawJSON))

	callbackCalled := false
	h.OnAccountsChanged = func() {
		callbackCalled = true
	}

	rec := postConfigImport(t, h, map[string]any{
		"mode": "merge",
		"config": map[string]any{
			"accounts": []any{
				map[string]any{
					"email":          "valid@test.com",
					"proxy_id":       validProxyID,
					"manual_binding": true,
				},
				map[string]any{
					"email":          "invalid@test.com",
					"proxy_id":       invalidProxyID,
					"manual_binding": true,
				},
			},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	if !callbackCalled {
		t.Fatalf("OnAccountsChanged callback must be triggered upon successful import")
	}

	accValid, _ := h.Store.FindAccount("valid@test.com")
	if accValid.ProxyID != validProxyID {
		t.Fatalf("valid node binding must be preserved, want %q got %q", validProxyID, accValid.ProxyID)
	}

	accInvalid, _ := h.Store.FindAccount("invalid@test.com")
	if accInvalid.ProxyID != "" {
		t.Fatalf("invalid node binding must be cleared, got %q", accInvalid.ProxyID)
	}
	if accInvalid.ManualBinding {
		t.Fatalf("invalid node manual_binding must be cleared")
	}
}

// 跨设备名称模糊匹配测试：原设备与目标设备的 subID 不同，但节点名称相同且在目标设备中唯一。
func TestConfigImportFuzzyMatchByNodeName(t *testing.T) {
	// 目标系统订阅
	targetSubID := "sub-device-b"
	nodeName := "新加坡 01"
	targetNodeKey := targetSubID + "::" + nodeName
	targetProxyID := config.MihomoManagedProxyID(targetNodeKey)

	rawCfg := map[string]any{
		"keys": []any{"k1"},
		"mihomo": map[string]any{
			"enabled":   true,
			"base_port": 10801,
			"subscriptions": []any{
				map[string]any{
					"id":   targetSubID,
					"name": "Device B Sub",
					"url":  "https://example.com/sub",
					"nodes": []any{
						map[string]any{
							"name": nodeName,
							"type": "vmess",
						},
					},
				},
			},
		},
	}
	rawJSON, _ := json.Marshal(rawCfg)
	h := newAdminTestHandler(t, string(rawJSON))

	// 来自设备 A 的数据：拥有不同的 subID 计算出的代理 ID
	deviceAProxyID := "mihomo-deviceA1234567"
	rec := postConfigImport(t, h, map[string]any{
		"mode": "replace",
		"config": map[string]any{
			"keys": []any{"k1"},
			"proxies": []any{
				map[string]any{
					"id":   deviceAProxyID,
					"name": nodeName, // 携带了节点名称
					"type": "socks5",
					"host": "127.0.0.1",
					"port": 10801,
				},
			},
			"accounts": []any{
				map[string]any{
					"email":          "fuzzy@test.com",
					"proxy_id":       deviceAProxyID,
					"manual_binding": true,
				},
			},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	acc, found := h.Store.FindAccount("fuzzy@test.com")
	if !found {
		t.Fatalf("account fuzzy@test.com must be imported")
	}
	if acc.ProxyID != targetProxyID {
		t.Fatalf("binding should be mapped to target device proxy ID %q, got %q", targetProxyID, acc.ProxyID)
	}
	if !acc.ManualBinding {
		t.Fatalf("manual_binding flag must be preserved")
	}
}
