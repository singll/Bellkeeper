import { Component, createSignal, createMemo, Show, For, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { healthApi, llmProxyApi, logCenterApi, matrixApi } from '@/api'
import { knowledgeFilesApi, knowledgeIndexApi } from '@/api/knowledge'
import { useToast } from '@/components/Toast'
import { Skeleton } from '@/components/Skeleton'
import { useAutoRefresh } from '@/hooks/useAutoRefresh'
import type { LLMChannelStatus, MatrixStats } from '@/types'

const Dashboard: Component = () => {
  const toast = useToast()

  const {
    data: dashboardData,
    enabled: autoRefresh,
    setEnabled: setAutoRefresh,
    refresh,
    lastUpdated,
    countdown,
    loading: isLoading,
  } = useAutoRefresh(
    async () => {
      const [h, llm, logDash, matrix, knowledgeStats] = await Promise.all([
        healthApi.detailed(),
        llmProxyApi.channelsStatus(),
        logCenterApi.getDashboard('24h'),
        matrixApi.getStats().catch(() => null),
        knowledgeFilesApi.getStats().catch(() => null),
      ])
      return { health: h, llmChannels: llm, logDashboard: logDash, matrixStats: matrix, knowledgeStats }
    },
    { interval: 30000, enabled: true, showCountdown: true }
  )

  const health = () => dashboardData()?.health
  const llmChannels = () => (dashboardData()?.llmChannels?.data || []) as LLMChannelStatus[]
  const logDashboard = () => dashboardData()?.logDashboard?.data
  const matrixStats = () => dashboardData()?.matrixStats as MatrixStats | null
  const knowledgeStats = () => dashboardData()?.knowledgeStats?.data

  const [error, setError] = createSignal<string | null>(null)

  onMount(() => { refresh() })

  const healthyLLMChannels = createMemo(() => llmChannels().filter((c) => c.health.state === 'closed').length)
  const brokenLLMChannels = createMemo(() => llmChannels().filter((c) => c.health.state === 'open').length)
  const totalLLMChannels = createMemo(() => llmChannels().length)

  const logErrors = createMemo(() => {
    const d = logDashboard()
    if (!d) return 0
    return d.by_level?.find((x: { level: string; count: number }) => x.level === 'error')?.count || 0
  })

  const logWarnings = createMemo(() => {
    const d = logDashboard()
    if (!d) return 0
    return d.by_level?.find((x: { level: string; count: number }) => x.level === 'warn')?.count || 0
  })

  // Activity chart data
  const activityChartData = createMemo(() => {
    const d = logDashboard()
    if (!d?.by_module) return []
    const stats = d.by_module
    const total = stats.reduce((sum: number, s: { count: number }) => sum + s.count, 0) || 1
    return stats.slice(0, 5).map((stat: { module: string; count: number }) => ({
      label: stat.module.replace(/_/g, ' '),
      value: stat.count,
      percentage: Math.round((stat.count / total) * 100),
    }))
  })

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">仪表盘</h1>
          <p class="text-sm text-dark-400 mt-1">系统状态概览</p>
        </div>
        <div class="flex items-center gap-3">
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
            刷新
          </button>
        </div>
      </div>

      {/* System Status Banner */}
      <div class="card mb-6">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div class="flex items-center gap-4">
            <div class={`w-12 h-12 rounded-xl flex items-center justify-center ${
              health()?.status === 'healthy' ? 'bg-emerald-500/20' : 'bg-amber-500/20'
            }`}>
              <svg class={`w-6 h-6 ${health()?.status === 'healthy' ? 'text-emerald-400' : 'text-amber-400'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <div>
              <div class="text-sm text-dark-400">系统状态</div>
              <Show when={health()} fallback={<Skeleton width={80} height={24} />}>
                <div class={`text-xl font-bold ${health()?.status === 'healthy' ? 'text-emerald-400' : 'text-amber-400'}`}>
                  {health()?.status === 'healthy' ? '运行正常' : '部分降级'}
                </div>
              </Show>
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

      {/* Hero System Cards */}
      <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-6">
        {/* Knowledge System Card */}
        <A href="/knowledge/files" class="card card-hover relative overflow-hidden border-l-4 border-l-blue-500 bg-gradient-to-br from-blue-500/10 to-blue-500/5 group">
          <div class="flex items-start justify-between">
            <div class="w-12 h-12 rounded-xl bg-blue-500/20 flex items-center justify-center">
              <svg class="w-6 h-6 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
              </svg>
            </div>
            <span class="text-xs text-dark-500 group-hover:text-blue-400 transition-colors">查看详情 →</span>
          </div>
          <div class="mt-4">
            <div class="text-sm text-dark-400">知识库</div>
            <Show when={!isLoading()} fallback={<Skeleton width={60} height={28} />}>
              <div class="text-2xl font-bold text-blue-400 mt-1">
                {knowledgeStats()?.indexed_count ?? '--'}
              </div>
            </Show>
            <div class="text-xs text-dark-500 mt-1">已索引文件</div>
          </div>
        </A>

        {/* LLM System Card */}
        <A href="/llm" class="card card-hover relative overflow-hidden border-l-4 border-l-amber-500 bg-gradient-to-br from-amber-500/10 to-amber-500/5 group">
          <div class="flex items-start justify-between">
            <div class="w-12 h-12 rounded-xl bg-amber-500/20 flex items-center justify-center">
              <svg class="w-6 h-6 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
            </div>
            <span class="text-xs text-dark-500 group-hover:text-amber-400 transition-colors">查看详情 →</span>
          </div>
          <div class="mt-4">
            <div class="text-sm text-dark-400">LLM</div>
            <Show when={!isLoading()} fallback={<Skeleton width={60} height={28} />}>
              <div class={`text-2xl font-bold ${brokenLLMChannels() > 0 ? 'text-red-400' : 'text-amber-400'} mt-1`}>
                {healthyLLMChannels()}/{totalLLMChannels()}
              </div>
            </Show>
            <div class="text-xs text-dark-500 mt-1">健康渠道</div>
          </div>
        </A>

        {/* Log System Card */}
        <A href="/logs" class="card card-hover relative overflow-hidden border-l-4 border-l-emerald-500 bg-gradient-to-br from-emerald-500/10 to-emerald-500/5 group">
          <div class="flex items-start justify-between">
            <div class="w-12 h-12 rounded-xl bg-emerald-500/20 flex items-center justify-center">
              <svg class="w-6 h-6 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01" />
              </svg>
            </div>
            <span class="text-xs text-dark-500 group-hover:text-emerald-400 transition-colors">查看详情 →</span>
          </div>
          <div class="mt-4">
            <div class="text-sm text-dark-400">日志</div>
            <Show when={!isLoading()} fallback={<Skeleton width={60} height={28} />}>
              <div class={`text-2xl font-bold ${logErrors() > 5 ? 'text-red-400' : 'text-emerald-400'} mt-1`}>
                {logErrors()}
              </div>
            </Show>
            <div class="text-xs text-dark-500 mt-1">24h 错误数</div>
          </div>
        </A>

        {/* Matrix System Card */}
        <A href="/matrix" class="card card-hover relative overflow-hidden border-l-4 border-l-violet-500 bg-gradient-to-br from-violet-500/10 to-violet-500/5 group">
          <div class="flex items-start justify-between">
            <div class="w-12 h-12 rounded-xl bg-violet-500/20 flex items-center justify-center">
              <svg class="w-6 h-6 text-violet-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
              </svg>
            </div>
            <span class="text-xs text-dark-500 group-hover:text-violet-400 transition-colors">查看详情 →</span>
          </div>
          <div class="mt-4">
            <div class="text-sm text-dark-400">Matrix</div>
            <Show when={!isLoading()} fallback={<Skeleton width={60} height={28} />}>
              <div class="text-2xl font-bold text-violet-400 mt-1">
                {matrixStats()?.rooms ?? '--'}
              </div>
            </Show>
            <div class="text-xs text-dark-500 mt-1">活跃房间</div>
          </div>
        </A>
      </div>

      {/* Service Status */}
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        <div class="card">
          <h2 class="text-lg font-semibold text-white mb-4">服务状态</h2>
          <Show when={!isLoading()} fallback={
            <div class="space-y-3">
              <For each={[1, 2, 3]}>{() => <Skeleton height={56} class="rounded-xl" />}</For>
            </div>
          }>
            <Show when={!error()} fallback={
              <div class="empty-state">
                <p class="empty-state-title">加载失败</p>
                <p class="empty-state-description">{error() || ''}</p>
              </div>
            }>
              <div class="space-y-3">
                <For each={Object.entries(health()?.services || {})}>
                  {([name, status]) => (
                    <div class="flex items-center justify-between p-3 bg-dark-700/50 rounded-xl">
                      <div class="flex items-center gap-3">
                        <span class={`status-dot ${status.status === 'up' ? 'status-dot-success' : 'status-dot-danger'}`} />
                        <span class="font-medium text-white capitalize">{name}</span>
                      </div>
                      <span class={`badge ${status.status === 'up' ? 'badge-success' : 'badge-danger'}`}>
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

        {/* Activity Trend Chart */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <div>
              <h2 class="text-lg font-semibold text-white">模块活跃度</h2>
              <p class="text-sm text-dark-400 mt-1">24h 操作量分布</p>
            </div>
          </div>
          <Show when={!isLoading() && activityChartData().length > 0} fallback={
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
      </div>

      {/* LLM + Log Quick Status */}
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        {/* LLM Quick Status */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">LLM 状态</h2>
            <A href="/llm" class="text-sm text-amber-400 hover:text-amber-300">
              查看详情 →
            </A>
          </div>
          <Show when={!isLoading()} fallback={
            <div class="grid grid-cols-3 gap-4">
              <For each={[1, 2, 3]}>{() => <Skeleton height={80} class="rounded-xl" />}</For>
            </div>
          }>
            <div class="grid grid-cols-3 gap-4">
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

        {/* Log Quick Status */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">日志状态</h2>
            <A href="/logs/dashboard" class="text-sm text-emerald-400 hover:text-emerald-300">
              查看详情 →
            </A>
          </div>
          <Show when={!isLoading()} fallback={
            <div class="grid grid-cols-3 gap-4">
              <For each={[1, 2, 3]}>{() => <Skeleton height={80} class="rounded-xl" />}</For>
            </div>
          }>
            <div class="grid grid-cols-3 gap-4">
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">24h 错误</div>
                <div class={`text-2xl font-bold ${logErrors() > 5 ? 'text-red-400' : 'text-emerald-400'}`}>{logErrors()}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">24h 警告</div>
                <div class={`text-2xl font-bold ${logWarnings() > 10 ? 'text-amber-400' : 'text-emerald-400'}`}>{logWarnings()}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">Matrix 24h</div>
                <div class="text-2xl font-bold text-violet-400">
                  {matrixStats()?.notifications_24h ?? '--'}
                </div>
              </div>
            </div>
          </Show>
        </div>
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
