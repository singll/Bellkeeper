import { Component, For, Show, createMemo, createResource } from 'solid-js'
import { A } from '@solidjs/router'
import type { LLMChannelStatus, LLMGroupStatus } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import { getCircuitDot } from './shared'

const LLMOverview: Component = () => {
  const toast = useToast()

  const [channelsData, { refetch: refetchChannels }] = createResource(
    () => llmProxyApi.channelsStatus()
  )
  const [groupsData, { refetch: refetchGroups }] = createResource(
    () => llmProxyApi.groupsStatus()
  )

  const channels = () => channelsData()?.data || []
  const groups = () => groupsData()?.data || []

  const totalChannels = createMemo(() => channels().length)
  const healthyChannels = createMemo(() => channels().filter((item) => item.health.state === 'closed').length)
  const circuitBrokenChannels = createMemo(() => channels().filter((item) => item.health.state === 'open').length)
  const halfOpenChannels = createMemo(() => channels().filter((item) => item.health.state === 'half_open').length)

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

  const alerts = createMemo(() => {
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
    await Promise.all([refetchChannels(), refetchGroups()])
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
          <p class="text-sm text-dark-400 mt-1">查看渠道健康、模型组路由与熔断/粘性运行态</p>
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

          {/* Main content grid */}
          <div class="grid grid-cols-1 xl:grid-cols-[1.3fr_1fr] gap-6">
            {/* Global Health Summary */}
            <div class="card">
              <div class="flex items-center justify-between mb-4">
                <h2 class="text-lg font-semibold text-white">全局健康摘要</h2>
                <span class={`badge ${overviewStatus().badge}`}>{overviewStatus().label}</span>
              </div>
              <p class="text-dark-300 mb-4">{overviewStatus().description}</p>
              <Show
                when={alerts().length > 0}
                fallback={<div class="text-sm text-emerald-300">当前没有需要处理的告警项。</div>}
              >
                <div class="space-y-2">
                  <For each={alerts()}>
                    {(alert) => (
                      <div class="flex items-start gap-2 p-3 rounded-xl bg-dark-700/50 border border-dark-600/50 text-sm text-dark-200">
                        <span class="status-dot status-dot-warning mt-1" />
                        <span>{alert}</span>
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
