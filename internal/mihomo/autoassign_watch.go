package mihomo

import (
	"context"
	"ds2api/internal/config"
	"math/rand"
	"time"
)

// startWatcher 启动后台健康巡检（幂等，重复调用只启动一次）。
func (m *Manager) startWatcher() {
	if m == nil {
		return
	}
	m.watchOnce.Do(func() {
		m.watchMu.Lock()
		stop := make(chan struct{})
		m.watchStop = stop
		m.watchMu.Unlock()
		go m.watcherLoop(stop)
	})
}

// stopWatcher 停止后台巡检（幂等，watchStop 受锁保护，避免与启动并发写）。
func (m *Manager) stopWatcher() {
	if m == nil {
		return
	}
	m.watchMu.Lock()
	stop := m.watchStop
	m.watchStop = nil
	m.watchMu.Unlock()
	if stop != nil {
		close(stop)
	}
}

func (m *Manager) watcherLoop(stop chan struct{}) {
	interval := watcherTickInterval()
	// 服务启动 5 秒后立即触发第一轮"测速 + 自动分配"，
	// 之后按 interval（默认 60s，DS2API_MIHOMO_INTERVAL 可配置）定时循环。
	initial := time.NewTimer(initialSweepDelay)
	defer initial.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-initial.C:
			m.watchTick()
		case <-ticker.C:
			m.watchTick()
		case <-m.reconcileCh:
			// 真实请求连续失败达到阈值触发：立即调和，不等待下一拍。
			// reconcileBindings 自带 enabled && auto_bind 门卫，串行于 watcher。
			m.reconcileBindings()
		}
	}
}

// watchTick 单轮巡检（测速 + 自动分配）：
//  1. 自愈：桥已启用但 external-controller 不可达（子进程崩溃/未拉起）时，
//     节流后尝试重新 Apply 拉起进程；
//  2. 测速：对"已绑定 + 已标记异常"的节点测速并落盘
//     （恢复的节点自动纳回可用池），保持延迟数据最新；
//  3. 绑定调和：坏节点故障转移（含换号冷却）/ 补位挂载 / 禁用账号回收。
func (m *Manager) watchTick() {
	defer func() {
		if r := recover(); r != nil {
			config.Logger.Error("[mihomo] watcher tick panic", "recover", r)
		}
	}()
	snap := m.store.Snapshot()
	if !snap.Mihomo.Enabled || !snap.Mihomo.AutoBind {
		return
	}
	m.trySelfHeal(snap)
	if m.runningWithAPI() {
		targets := sweepTargets(snap, m.NodeHealthMap())
		if len(targets) > 0 {
			before := m.NodeHealthMap()
			results := m.testNodeDelays(context.Background(), snap.Mihomo.APIPort, targets)
			// 自动测速结果直接写回订阅节点字段并落盘，重启后不丢失。
			m.applyHealthResults(results)
			for _, r := range results {
				prev := before[r.NodeKey]
				if prev.Status == NodeHealthFail && r.Health.Status == NodeHealthOK {
					config.Logger.Info("[mihomo] node recovered, back to available pool",
						"node", r.Name, "latency_ms", r.Health.LatencyMS)
				}
			}
		}
	}
	m.reconcileBindings()
}

// trySelfHeal 在桥启用但 external-controller 端口不可达时，节流地重新 Apply 拉起进程。
// 进程崩溃后 Wait goroutine 会置 m.running=false，watcher 若不做处理桥会一直停摆；
// 这里只要端口不可达就尝试重启，同时支持“用户事后放入二进制”的场景自动生效。
func (m *Manager) trySelfHeal(snap config.Config) {
	if m.apiReachable() {
		return
	}
	m.healthMu.Lock()
	due := time.Since(m.lastRestart) >= selfHealMinInterval
	if due {
		m.lastRestart = time.Now()
	}
	m.healthMu.Unlock()
	if !due {
		return
	}
	config.Logger.Warn("[mihomo] external-controller unreachable while bridge enabled; attempting restart")
	if err := m.Apply(context.Background()); err != nil {
		config.Logger.Warn("[mihomo] self-heal apply failed, will retry later", "error", err)
	}
}

// coldProbePerTick 每轮巡检随机补测的“未绑定”节点数，保持候选池新鲜，
// 避免新添加/未使用的节点长期停留在 unknown，补位与故障转移时无新鲜数据可用。
const coldProbePerTick = 10

// sweepTargets 汇总本轮需要测速的节点：
//   - 有账号绑定的节点 ∪ 已标记异常的节点（必测）；
//   - 再随机补测最多 coldProbePerTick 个当前未绑定的节点。
func sweepTargets(snap config.Config, health map[string]NodeHealth) []nodeRef {
	bound := map[string]struct{}{}
	for nodeKey := range nodeBoundCounts(snap) {
		bound[nodeKey] = struct{}{}
	}
	want := map[string]struct{}{}
	for nodeKey := range bound {
		want[nodeKey] = struct{}{}
	}
	for nodeKey, h := range health {
		if h.Status == NodeHealthFail {
			want[nodeKey] = struct{}{}
		}
	}
	out := []nodeRef{}
	cold := []nodeRef{}
	for _, sub := range snap.Mihomo.Subscriptions {
		for _, node := range sub.Nodes {
			key := config.MihomoNodeKey(sub.ID, node.Name)
			if _, ok := want[key]; ok {
				out = append(out, nodeRef{Key: key, Name: node.Name})
				continue
			}
			cold = append(cold, nodeRef{Key: key, Name: node.Name})
		}
	}
	return append(out, sampleNodes(cold, coldProbePerTick)...)
}

// sampleNodes 从节点列表中随机抽取至多 n 个（len<=n 时原样返回），
// 保证每轮巡检抽到的未绑定节点不同。
func sampleNodes(nodes []nodeRef, n int) []nodeRef {
	if len(nodes) == 0 || n <= 0 {
		return nil
	}
	if len(nodes) <= n {
		return nodes
	}
	perm := rand.Perm(len(nodes))
	out := make([]nodeRef, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, nodes[perm[i]])
	}
	return out
}
