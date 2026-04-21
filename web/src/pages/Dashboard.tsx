import { Component, createSignal, createMemo, Show, For, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { healthApi, workflowsApi, llmProxyApi, logsApi } from '@/api'
import { useToast } from '@/components/Toast'
import { Skeleton } from '@/components/Skeleton'
import { useAutoRefresh } from '@/hooks/useAutoRefresh'
import { EmptyState, EmptyStateVariants } from '@/components/EmptyState'

const Dashboard: Component = () => {
  const toast = useToast()

  // Auto-refresh using the hook
  const {
    data: dashboardData,
    enabled: autoRefresh,
    setEnabled: setAutoRefresh,
    refresh,
    lastUpdated,
    countdown,
    loading: isLoading,
    error: refreshError,
  } = useAutoRefresh(
    async () => {
      const [h, w, llm, stats] = await Promise.all([
        healthApi.detailed(),
        workflowsApi.list(),
        llmProxyApi.channelsStatus(),
        logsApi.stats(),
      ])
      return { health: h, workflows: w, llmChannels: llm, activityStats: stats }
    },
    {
      interval: 30000,
      enabled: true,
      showCountdown: true,
    }
  )

  // Data state derived from hook data
  const health = () => dashboardData()?.health
  const workflows = () => dashboardData()?.workflows?.data || []
  const llmChannels = () => dashboardData()?.llmChannels?.data || []
  const activityStats = () => dashboardData()?.activityStats?.data || []
  const [error, setError] = createSignal<string | null>(null)
  const [triggeringWorkflow, setTriggeringWorkflow] = createSignal<string | null>(null)

  // Fetch all data
  const fetchAll = async () => {
    setError(null)
    try {
      const [h, w, llm, stats] = await Promise.all([
        healthApi.detailed(),
        workflowsApi.list(),
        llmProxyApi.channelsStatus(),
        logsApi.stats(),
      ])
      setHealth(h)
      setWorkflows(w?.data || [])
      setLlmChannels(llm?.data || [])
      setActivityStats(stats?.data || [])
      setLastUpdated(new Date())
      setCountdown(REFRESH_INTERVAL / 1000)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  // Initial fetch
  onMount(() => {
    refresh()
  })

  const getMetric = (key: string): number | string => {
    const metrics = health()?.metrics
    if (!metrics) return '--'
    const value = metrics[key]
    return typeof value === 'number' ? value : '--'
  }

  const healthyLLMChannels = createMemo(() => llmChannels().filter((channel) => channel.health.state === 'closed').length)
  const brokenLLMChannels = createMemo(() => llmChannels().filter((channel) => channel.health.state === 'open').length)
  const totalLLMChannels = createMemo(() => llmChannels().length)

  const handleTriggerWorkflow = async (name: string) => {
    setTriggeringWorkflow(name)
    try {
      await workflowsApi.trigger(name, {})
      toast.success(`工作流 "${name}" 已触发`)
    } catch (err) {
      toast.error('触发失败: ' + (err instanceof Error ? err.message : String(err)))
    } finally {
      setTriggeringWorkflow(null)
    }
  }

  // Activity chart data using real stats from API
  const activityChartData = createMemo(() => {
    const stats = activityStats()
    const total = stats.reduce((sum, s) => sum + s.count, 0) || 1
    return stats.slice(0, 5).map((stat) => ({
      label: stat.module.replace(/_/g, ' '),
      value: stat.count,
      percentage: Math.round((stat.count / total) * 100),
    }))
  })

  const stats = createMemo(() => [
    { label: '标签数量', key: 'tags_count', icon: 'M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z', color: 'text-primary-400' },
    { label: 'RSS 订阅', key: 'rss_feeds_count', icon: 'M6 5c7.18 0 13 5.82 13 13M6 11a7 7 0 017 7m-6 0a1 1 0 11-2 0 1 1 0 012 0z', color: 'text-orange-400' },
    { label: '知识库映射', key: 'datasets_count', icon: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10', color: 'text-purple-400' },
    { label: 'LLM 渠道', key: '_llm_channels', icon: 'M13 10V3L4 14h7v7l9-11h-7z', color: 'text-cyan-400' },
  ])

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">仪表盘</h1>
          <p class="text-sm text-dark-400 mt-1">系统状态概览</p>
        </div>
        <div class="flex items-center gap-3">
          {/* Auto-refresh toggle */}
          <button
            class={`btn ${autoRefresh() ? 'btn-primary' : 'btn-secondary'}`}
            onClick={() => setAutoRefresh(!autoRefresh())}
            title={autoRefresh() ? '自动刷新已开启' : '自动刷新已关闭'}
          >
            <svg class={`w-4 h-4 ${autoRefresh() ? '' : 'opacity-50'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            {autoRefresh() ? `${countdown()}s` : '已暂停'}
          </button>
          <button class="btn btn-secondary" onClick={() => refresh()}>
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            刷新状态
          </button>
        </div>
      </div>

      {/* System Status */}
      <div class="card mb-6">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div class="flex items-center gap-4">
            <div class={`w-12 h-12 rounded-xl flex items-center justify-center ${
              health()?.status === 'healthy'
                ? 'bg-emerald-500/20'
                : 'bg-amber-500/20'
            }`}>
              <svg class={`w-6 h-6 ${
                health()?.status === 'healthy' ? 'text-emerald-400' : 'text-amber-400'
              }`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <div>
              <div class="text-sm text-dark-400">系统状态</div>
              <div class={`text-xl font-bold ${
                health()?.status === 'healthy' ? 'text-emerald-400' : 'text-amber-400'
              }`}>
                <Show when={health()} fallback={<Skeleton width={80} height={24} />}>
                  {health()?.status === 'healthy' ? '运行正常' : '部分降级'}
                </Show>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-6">
            <Show when={health()?.version}>
              <div class="text-right">
                <div class="text-sm text-dark-400">版本</div>
                <div class="text-lg font-mono text-dark-200">{health()?.version}</div>
              </div>
            </Show>
            <Show when={lastUpdated()}>
              <div class="text-right">
                <div class="text-sm text-dark-400">最后更新</div>
                <div class="text-sm text-dark-300">{lastUpdated()?.toLocaleTimeString('zh-CN')}</div>
              </div>
            </Show>
          </div>
        </div>
      </div>

      {/* Stats Grid */}
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <For each={stats()}>
          {(stat) => (
            <div class="card card-hover">
              <div class="flex items-center gap-3">
                <div class={`w-10 h-10 rounded-lg flex items-center justify-center bg-dark-700/50 ${stat.color}`}>
                  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={stat.icon} />
                  </svg>
                </div>
                <div>
                  <div class="text-sm text-dark-400">{stat.label}</div>
                  <Show when={!isLoading()} fallback={<Skeleton width={60} height={28} />}>
                    <div class={`text-2xl font-bold ${stat.color}`}>
                      {stat.key === '_llm_channels'
                        ? `${healthyLLMChannels()}/${totalLLMChannels()}`
                        : getMetric(stat.key)}
                    </div>
                  </Show>
                </div>
              </div>
            </div>
          )}
        </For>
      </div>

      {/* Activity Trend Chart */}
      <div class="card mb-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h2 class="text-lg font-semibold text-white">模块活跃度</h2>
            <p class="text-sm text-dark-400 mt-1">操作量分布</p>
          </div>
        </div>
        <Show when={!isLoading() && activityStats().length > 0} fallback={
          <div class="h-32 flex items-center justify-center">
            <Skeleton height={100} class="w-full" />
          </div>
        }>
          <div class="space-y-3">
            <For each={activityChartData()}>
              {(item) => (
                <div class="flex items-center gap-4">
                  <div class="w-28 text-sm text-dark-300 truncate" title={item.label}>
                    {item.label}
                  </div>
                  <div class="flex-1 h-6 bg-dark-700/50 rounded-lg overflow-hidden">
                    <div
                      class="h-full bg-gradient-to-r from-primary-500/80 to-primary-400 rounded-lg transition-all duration-500"
                      style={{ width: `${item.percentage}%` }}
                    />
                  </div>
                  <div class="w-16 text-right text-sm text-dark-300">
                    {item.value.toLocaleString()}
                  </div>
                  <div class="w-12 text-right text-xs text-dark-500">
                    {item.percentage}%
                  </div>
                </div>
              )}
            </For>
          </div>
        </Show>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        {/* Service Status */}
        <div class="card">
          <h2 class="text-lg font-semibold text-white mb-4">服务状态</h2>
          <Show when={!isLoading()} fallback={
            <div class="space-y-3">
              <For each={[1, 2, 3]}>{() => <Skeleton height={56} class="rounded-xl" />}</For>
            </div>
          }>
            <Show when={!error()} fallback={
              <EmptyState
                title="加载失败"
                description={error() || ''}
                icon={
                  <svg class="w-12 h-12 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                  </svg>
                }
              />
            }>
              <div class="space-y-3">
                <For each={Object.entries(health()?.services || {})}>
                  {([name, status]) => (
                    <div class="flex items-center justify-between p-3 bg-dark-700/50 rounded-xl">
                      <div class="flex items-center gap-3">
                        <span class={`status-dot ${
                          status.status === 'up' ? 'status-dot-success' : 'status-dot-danger'
                        }`} />
                        <span class="font-medium text-white capitalize">{name}</span>
                      </div>
                      <span class={`badge ${
                        status.status === 'up' ? 'badge-success' : 'badge-danger'
                      }`}>
                        {status.status === 'up' ? '在线' : '离线'}
                        {status.latency_ms && ` (${status.latency_ms}ms)`}
                      </span>
                    </div>
                  )}
                </For>
                <Show when={!Object.keys(health()?.services || {}).length}>
                  <p class="text-dark-400 text-center py-4">暂无服务数据</p>
                </Show>
              </div>
            </Show>
          </Show>
        </div>

        {/* Quick Actions */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">快捷操作</h2>
            <A href="/workflows" class="text-sm text-primary-400 hover:text-primary-300">
              查看全部 →
            </A>
          </div>
          <Show when={!isLoading() && workflows().length > 0}
            fallback={
              <Show when={isLoading()} fallback={
                <EmptyState
                  title="暂无可用工作流"
                  description="配置 n8n API Key 后可查看更多工作流"
                  icon={
                    <svg class="w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                    </svg>
                  }
                />
              }>
                <div class="space-y-3">
                  <For each={[1, 2, 3]}>{() => <Skeleton height={56} class="rounded-xl" />}</For>
                </div>
              </Show>
            }
          >
            <div class="space-y-3">
              <For each={workflows().slice(0, 5)}>
                {(workflow) => (
                  <div class="flex items-center justify-between p-3 bg-dark-700/50 rounded-xl group hover:bg-dark-700/70 transition-colors">
                    <div class="flex items-center gap-3">
                      <span class={`status-dot ${
                        workflow.active ? 'status-dot-success' : 'status-dot-gray'
                      }`} />
                      <span class="font-medium text-white">{workflow.name}</span>
                    </div>
                    <button
                      class="btn btn-primary btn-sm opacity-0 group-hover:opacity-100 transition-opacity"
                      disabled={triggeringWorkflow() === workflow.name}
                      onClick={() => handleTriggerWorkflow(workflow.name)}
                    >
                      {triggeringWorkflow() === workflow.name ? (
                        <div class="loading-spinner w-4 h-4" />
                      ) : (
                        <>
                          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                          </svg>
                          触发
                        </>
                      )}
                    </button>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </div>
      </div>

      {/* LLM Proxy Status */}
      <div class="card mb-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h2 class="text-lg font-semibold text-white">LLM Proxy 状态</h2>
            <p class="text-sm text-dark-400 mt-1">渠道健康与熔断概览</p>
          </div>
          <A href="/llm-proxy" class="text-sm text-primary-400 hover:text-primary-300">
            查看详情 →
          </A>
        </div>
        <Show when={!isLoading()} fallback={
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <For each={[1, 2, 3]}>{() => <Skeleton height={80} class="rounded-xl" />}</For>
          </div>
        }>
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div class="p-4 bg-dark-700/50 rounded-xl">
              <div class="text-sm text-dark-400 mb-1">健康渠道</div>
              <div class="text-2xl font-bold text-emerald-400">
                {healthyLLMChannels()} / {totalLLMChannels()}
              </div>
            </div>
            <div class="p-4 bg-dark-700/50 rounded-xl">
              <div class="text-sm text-dark-400 mb-1">熔断渠道</div>
              <div class="text-2xl font-bold text-red-400">{brokenLLMChannels()}</div>
            </div>
            <div class="p-4 bg-dark-700/50 rounded-xl">
              <div class="text-sm text-dark-400 mb-1">运行结论</div>
              <div class={`text-2xl font-bold ${brokenLLMChannels() > 0 ? 'text-amber-400' : 'text-primary-300'}`}>
                {totalLLMChannels() === 0 ? '未配置' : brokenLLMChannels() > 0 ? '需关注' : '正常'}
              </div>
            </div>
          </div>
        </Show>
      </div>

      {/* Last Update */}
      <div class="mt-6 text-center text-sm text-dark-500">
        <Show when={health()?.metrics?.timestamp}>
          最后更新: {new Date(health()!.metrics!.timestamp as string).toLocaleString('zh-CN')}
        </Show>
      </div>
    </div>
  )
}

export default Dashboard
