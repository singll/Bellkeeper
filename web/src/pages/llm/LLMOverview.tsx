import { Component, For, Show, createMemo, createResource } from 'solid-js'
import { A } from '@solidjs/router'
import type { LLMUsageByModel } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import { getCircuitDot, getSeverityDot, getSeverityBadge, getSeverityLabel, formatCents, formatDateShort } from './shared'

const LLMOverview: Component = () => {
  const toast = useToast()

  const [channelsData, { refetch: refetchChannels }] = createResource(() => llmProxyApi.channelsStatus())
  const [groupsData, { refetch: refetchGroups }] = createResource(() => llmProxyApi.groupsStatus())
  const [usageData, { refetch: refetchUsage }] = createResource(() => llmProxyApi.getUsage('model'))
  const [balancesData, { refetch: refetchBalances }] = createResource(() => llmProxyApi.allBalances())
  const [alertsData, { refetch: refetchAlerts }] = createResource(() => llmProxyApi.listAlerts({ limit: 6 }))

  const channels = () => channelsData()?.data || []
  const groups = () => groupsData()?.data || []
  const usageRows = () => (usageData()?.data || []) as LLMUsageByModel[]
  const balancesMap = () => balancesData()?.data || {}
  const recentAlerts = () => alertsData()?.data || []

  const totalChannels = createMemo(() => channels().length)
  const healthyChannels = createMemo(() => channels().filter((item) => item.health.state === 'closed').length)
  const circuitBrokenChannels = createMemo(() => channels().filter((item) => item.health.state === 'open').length)
  const halfOpenChannels = createMemo(() => channels().filter((item) => item.health.state === 'half_open').length)

  // Estimated cost (from usage aggregates) vs real balance (from upstream snapshots)
  const estCostCents = createMemo(() => usageRows().reduce((sum, r) => sum + (r.cost_cents || 0), 0))
  const topModels = createMemo(() =>
    [...usageRows()].sort((a, b) => (b.cost_cents || 0) - (a.cost_cents || 0)).slice(0, 5)
  )
  const balanceList = createMemo(() => Object.values(balancesMap()))
  const totalUsdBalance = createMemo(() =>
    balanceList()
      .filter((b) => (b.currency || '').toUpperCase() === 'USD')
      .reduce((sum, b) => sum + (b.balance || 0), 0)
  )

  const overviewStatus = createMemo(() => {
    if (totalChannels() === 0) {
      return { label: '未配置', badge: 'badge-gray', description: '当前没有可用渠道数据' }
    }
    if (circuitBrokenChannels() > 0) {
      return { label: '部分降级', badge: 'badge-warning', description: `有 ${circuitBrokenChannels()} 个渠道处于熔断状态` }
    }
    if (halfOpenChannels() > 0) {
      return { label: '恢复观察中', badge: 'badge-warning', description: `有 ${halfOpenChannels()} 个渠道处于半开探测状态` }
    }
    return { label: '运行正常', badge: 'badge-success', description: '所有已启用渠道均可正常服务' }
  })

  // Health-derived issues (live inference from channel/group state, distinct from persisted alert events)
  const healthIssues = createMemo(() => {
    const items: string[] = []
    const openChs = channels().filter((item) => item.health.state === 'open')
    if (openChs.length > 0) items.push(`熔断渠道：${openChs.map((item) => item.name).join('、')}`)
    const unstableChs = channels().filter((item) => item.health.state !== 'open' && item.health.recent_success_rate < 0.7)
    if (unstableChs.length > 0) items.push(`成功率偏低：${unstableChs.map((item) => item.name).join('、')}`)
    const stickyGrps = groups().filter((group) => group.sticky_bindings > 0)
    if (stickyGrps.length > 0) items.push(`存在粘性绑定：${stickyGrps.map((group) => `${group.name}(${group.sticky_bindings})`).join('、')}`)
    return items
  })

  const handleRefresh = async () => {
    await Promise.all([refetchChannels(), refetchGroups(), refetchUsage(), refetchBalances(), refetchAlerts()])
    toast.success('LLM Proxy 状态已刷新')
  }

  const renderProgress = (current: number, total: number, label: string) => {
    const ratio = total > 0 ? Math.min(current / total, 1) : 0
    return (
      <div>
        <div class="flex items-center justify-between text-xs text-dark-400 mb-1.5">
          <span>{label}</span>
          <span>{current} / {total || '--'}</span>
        </div>
        <div class="h-2 rounded-full bg-dark-700/70 overflow-hidden">
          <div
            class={`h-full rounded-full transition-all ${ratio >= 0.7 ? 'bg-emerald-500' : ratio >= 0.35 ? 'bg-amber-500' : 'bg-red-500'}`}
            style={{ width: `${Math.max(ratio * 100, ratio > 0 ? 6 : 0)}%` }}
          />
        </div>
      </div>
    )
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">LLM Proxy</h1>
          <p class="text-sm text-dark-400 mt-1">渠道健康、成本与余额、模型组路由与告警一览</p>
        </div>
        <button class="btn btn-primary" onClick={handleRefresh} disabled={channelsData.loading || groupsData.loading}>
          <svg class={`w-4 h-4 ${channelsData.loading ? 'animate-spin' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          刷新状态
        </button>
      </div>

      <Show when={!channelsData.loading && !groupsData.loading} fallback={
        <div class="card py-12">
          <div class="flex items-center justify-center">
            <div class="loading-spinner" />
            <span class="ml-3 text-dark-400">加载 LLM Proxy 状态中...</span>
          </div>
        </div>
      }>
        <div class="space-y-6">
          {/* Stats Cards */}
          <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
            <div class="stat-card">
              <div class="stat-label">渠道总数</div>
              <div class="stat-value">{totalChannels()}</div>
              <div class="stat-trend text-dark-400">已启用上游渠道</div>
            </div>
            <div class="stat-card">
              <div class="stat-label">健康渠道</div>
              <div class="stat-value text-emerald-400">{healthyChannels()}</div>
              <div class="stat-trend stat-trend-up">状态 closed</div>
            </div>
            <div class="stat-card">
              <div class="stat-label">熔断渠道</div>
              <div class="stat-value text-red-400">{circuitBrokenChannels()}</div>
              <div class="stat-trend stat-trend-down">状态 open</div>
            </div>
            <div class="stat-card">
              <div class="stat-label">模型组</div>
              <div class="stat-value text-primary-300">{groups().length}</div>
              <div class="stat-trend text-dark-400">虚拟模型池</div>
            </div>
          </div>

          {/* Estimated cost vs real balance */}
          <div class="grid grid-cols-1 xl:grid-cols-2 gap-6">
            {/* Estimated cost (from usage) */}
            <div class="card">
              <div class="flex items-center justify-between mb-4">
                <h2 class="text-lg font-semibold text-white">预估成本</h2>
                <span class="text-sm text-dark-400">近 30 天 · 按用量计费</span>
              </div>
              <div class="text-3xl font-bold text-white mb-1">{formatCents(estCostCents())}</div>
              <div class="text-xs text-dark-400 mb-4">基于 llm_proxy_logs 按模型计费聚合</div>
              <Show
                when={topModels().length > 0}
                fallback={<div class="text-sm text-dark-500">暂无用量数据。</div>}
              >
                <div class="space-y-0">
                  <For each={topModels()}>
                    {(m) => (
                      <div class="flex items-center justify-between py-2 border-b border-dark-700/40 last:border-0">
                        <span class="text-sm text-dark-200 font-mono truncate max-w-[200px]" title={m.model}>{m.model}</span>
                        <div class="flex items-center gap-4">
                          <span class="text-xs text-dark-500">{(m.requests || 0).toLocaleString()} 次</span>
                          <span class="text-sm font-medium text-white">{formatCents(m.cost_cents || 0)}</span>
                        </div>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            </div>

            {/* Real balance (from /llm/balances) */}
            <div class="card">
              <div class="flex items-center justify-between mb-4">
                <h2 class="text-lg font-semibold text-white">真实余额</h2>
                <span class="text-sm text-dark-400">上游渠道快照</span>
              </div>
              <div class="text-3xl font-bold text-emerald-400 mb-1">${totalUsdBalance().toFixed(2)}</div>
              <div class="text-xs text-dark-400 mb-4">USD 渠道合计（其他币种单列见下）</div>
              <Show
                when={balanceList().length > 0}
                fallback={<div class="text-sm text-dark-500">暂无余额数据（可在配置后触发刷新）。</div>}
              >
                <div class="space-y-0">
                  <For each={balanceList()}>
                    {(b) => (
                      <div class="flex items-center justify-between py-2 border-b border-dark-700/40 last:border-0">
                        <div class="flex items-center gap-2">
                          <span class="text-sm text-dark-200">{b.channel_name}</span>
                          <span class="text-xs text-dark-500">{b.provider_type}</span>
                        </div>
                        <Show
                          when={!b.error}
                          fallback={<span class="text-xs text-red-400 max-w-[160px] truncate" title={b.error}>{b.error}</span>}
                        >
                          <span class="text-sm font-medium text-white">{(b.balance || 0).toFixed(2)} {b.currency || ''}</span>
                        </Show>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            </div>
          </div>

          {/* Recent alerts strip */}
          <div class="card">
            <div class="flex items-center justify-between mb-4">
              <h2 class="text-lg font-semibold text-white">最近告警</h2>
              <A href="/llm/alerts" class="text-sm text-primary-400 hover:text-primary-300">查看全部 →</A>
            </div>
            <Show
              when={recentAlerts().length > 0}
              fallback={<div class="text-sm text-emerald-300">近期没有告警事件。</div>}
            >
              <div class="space-y-2">
                <For each={recentAlerts()}>
                  {(a) => (
                    <div class="flex items-center gap-3 p-3 rounded-xl bg-dark-700/40 border border-dark-600/50">
                      <span class={`status-dot ${getSeverityDot(a.severity)}`} />
                      <span class={`badge ${getSeverityBadge(a.severity)}`}>{getSeverityLabel(a.severity)}</span>
                      <span class="text-xs font-mono text-dark-400 hidden sm:inline">{a.alert_type}</span>
                      <span class="text-sm text-dark-200 flex-1 truncate" title={a.message}>{a.message}</span>
                      <span class="text-xs text-dark-500 hidden md:inline">{a.channel_name}</span>
                      <span class="text-xs text-dark-500 whitespace-nowrap">{formatDateShort(a.created_at)}</span>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>

          {/* Health summary + resource overview */}
          <div class="grid grid-cols-1 xl:grid-cols-[1.3fr_1fr] gap-6">
            {/* Global Health Summary */}
            <div class="card">
              <div class="flex items-center justify-between mb-4">
                <h2 class="text-lg font-semibold text-white">全局健康摘要</h2>
                <span class={`badge ${overviewStatus().badge}`}>{overviewStatus().label}</span>
              </div>
              <p class="text-dark-300 mb-4">{overviewStatus().description}</p>
              <Show
                when={healthIssues().length > 0}
                fallback={<div class="text-sm text-emerald-300">当前没有需要处理的健康问题。</div>}
              >
                <div class="space-y-2">
                  <For each={healthIssues()}>
                    {(issue) => (
                      <div class="flex items-start gap-2 p-3 rounded-xl bg-dark-700/50 border border-dark-600/50 text-sm text-dark-200">
                        <span class="status-dot status-dot-warning mt-1" />
                        <span>{issue}</span>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            </div>

            {/* Resource Overview */}
            <div class="card">
              <h2 class="text-lg font-semibold text-white mb-4">资源概览</h2>
              <div class="space-y-4">
                {renderProgress(healthyChannels(), totalChannels(), '健康渠道占比')}
                {renderProgress(circuitBrokenChannels(), totalChannels(), '熔断渠道占比')}
                {renderProgress(
                  groups().reduce((sum, item) => sum + item.sticky_bindings, 0),
                  Math.max(groups().length * 5, 1),
                  '粘性绑定热度'
                )}
              </div>
            </div>
          </div>

          {/* Model Group Summary */}
          <div>
            <div class="flex items-center justify-between mb-4">
              <h2 class="text-lg font-semibold text-white">模型组摘要</h2>
              <span class="text-sm text-dark-400">共 {groups().length} 个模型组</span>
            </div>
            <Show
              when={groups().length > 0}
              fallback={
                <div class="card empty-state">
                  <p class="empty-state-title">暂无模型组</p>
                  <p class="empty-state-description">请在<A href="/llm/config" class="text-primary-400 hover:text-primary-300 ml-1">配置</A>页面中创建模型组。</p>
                </div>
              }
            >
              <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
                <For each={groups()}>
                  {(group) => (
                    <div class="card card-hover">
                      <div class="flex items-start justify-between gap-3 mb-4">
                        <div>
                          <div class="text-lg font-semibold text-white">{group.name}</div>
                          <div class="text-sm text-dark-400 mt-1">{group.description || '未填写描述'}</div>
                        </div>
                      </div>
                      <div class="grid grid-cols-2 gap-3 mb-4 text-sm">
                        <div class="p-3 bg-dark-700/40 rounded-xl">
                          <div class="text-dark-400 mb-1">策略</div>
                          <div class="font-medium text-white">{group.strategy}</div>
                        </div>
                        <div class="p-3 bg-dark-700/40 rounded-xl">
                          <div class="text-dark-400 mb-1">粘性绑定</div>
                          <div class="font-medium text-white">{group.sticky_bindings}</div>
                        </div>
                        <div class="p-3 bg-dark-700/40 rounded-xl">
                          <div class="text-dark-400 mb-1">成员数</div>
                          <div class="font-medium text-white">{group.members.length}</div>
                        </div>
                        <div class="p-3 bg-dark-700/40 rounded-xl">
                          <div class="text-dark-400 mb-1">Sticky TTL</div>
                          <div class="font-medium text-white">{group.sticky_ttl_seconds}s</div>
                        </div>
                      </div>
                      <div class="flex items-center gap-2 flex-wrap">
                        <For each={group.members}>
                          {(member) => (
                            <div class="inline-flex items-center gap-2 px-2.5 py-1.5 rounded-lg bg-dark-700/50 text-xs text-dark-200">
                              <span class={`status-dot ${member.available ? getCircuitDot(member.health.state) : 'status-dot-gray'}`} />
                              <span>{member.channel}</span>
                            </div>
                          )}
                        </For>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default LLMOverview
