import { Component, createMemo, Show, For, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { dashboardApi, healthApi, llmProxyApi, logCenterApi, matrixApi } from '@/api'
import { knowledgeFilesApi } from '@/api/knowledge'
import { Skeleton } from '@/components/Skeleton'
import { useAutoRefresh } from '@/hooks/useAutoRefresh'
import type { LLMChannelStatus, MatrixStats, DashboardStats } from '@/types'

const fmtNum = (n?: number | null): string => {
  if (n === undefined || n === null) return '--'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 10_000) return `${(n / 1_000).toFixed(1)}k`
  return n.toLocaleString()
}

const fmtPct = (r?: number | null): string => {
  if (r === undefined || r === null) return '--'
  return `${Math.round(r * 100)}%`
}

const fmtCost = (cents?: number | null): string => {
  if (cents === undefined || cents === null) return '--'
  return `$${(cents / 100).toFixed(2)}`
}

const fmtRelTime = (iso?: string | null): string => {
  if (!iso) return '从未'
  const diffMs = Date.now() - new Date(iso).getTime()
  if (diffMs < 0) return '刚刚'
  const mins = Math.floor(diffMs / 60_000)
  if (mins < 1) return '刚刚'
  if (mins < 60) return `${mins} 分钟前`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours} 小时前`
  return `${Math.floor(hours / 24)} 天前`
}

const Dashboard: Component = () => {
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
      const [h, stats, llm, logDash, matrix, knowledgeStats] = await Promise.all([
        healthApi.detailed(),
        dashboardApi.getStats().catch(() => null),
        llmProxyApi.channelsStatus(),
        logCenterApi.getDashboard('24h'),
        matrixApi.getStats().catch(() => null),
        knowledgeFilesApi.getStats().catch(() => null),
      ])
      return { health: h, stats: stats?.data ?? null, llmChannels: llm, logDashboard: logDash, matrixStats: matrix, knowledgeStats }
    },
    { interval: 30000, enabled: true, showCountdown: true }
  )

  const health = () => dashboardData()?.health
  const stats = () => dashboardData()?.stats as DashboardStats | null
  const crawl = () => stats()?.crawl
  const pkb = () => stats()?.pkb
  const llm = () => stats()?.llm
  const llmChannels = () => (dashboardData()?.llmChannels?.data || []) as LLMChannelStatus[]
  const logDashboard = () => dashboardData()?.logDashboard?.data
  const matrixStats = () => dashboardData()?.matrixStats as MatrixStats | null
  const knowledgeStats = () => dashboardData()?.knowledgeStats?.data

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
    const statsArr = d.by_module
    const total = statsArr.reduce((sum: number, s: { count: number }) => sum + s.count, 0) || 1
    return statsArr.slice(0, 5).map((stat: { module: string; count: number }) => ({
      label: stat.module.replace(/_/g, ' '),
      value: stat.count,
      percentage: Math.round((stat.count / total) * 100),
    }))
  })

  const byLayer = createMemo(() => {
    const layers = knowledgeStats()?.by_layer as Record<string, number> | undefined
    if (!layers) return []
    const order = ['raw', 'archive', 'vault']
    return Object.entries(layers).sort(
      (a, b) => (order.indexOf(a[0]) + 1 || 99) - (order.indexOf(b[0]) + 1 || 99)
    )
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
            <div class="text-right">
              <div class="text-sm text-dark-400">最近爬取</div>
              <div class={`text-lg font-semibold ${crawl()?.last_crawl_at ? 'text-dark-200' : 'text-red-400'}`}>
                {fmtRelTime(crawl()?.last_crawl_at)}
              </div>
            </div>
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

      {/* Hero Cards */}
      <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-6">
        {/* Crawl Card */}
        <A href="/rss" class="card card-hover relative overflow-hidden border-l-4 border-l-cyan-500 bg-gradient-to-br from-cyan-500/10 to-cyan-500/5 group">
          <div class="flex items-start justify-between">
            <div class="w-12 h-12 rounded-xl bg-cyan-500/20 flex items-center justify-center">
              <svg class="w-6 h-6 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 15a4 4 0 004 4h9a5 5 0 10-.1-9.999 5.002 5.002 0 10-9.78 2.096A4.001 4.001 0 003 15z" />
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 12v6m0 0l-3-3m3 3l3-3" />
              </svg>
            </div>
            <span class="text-xs text-dark-500 group-hover:text-cyan-400 transition-colors">查看详情 →</span>
          </div>
          <div class="mt-4">
            <div class="text-sm text-dark-400">今日爬取</div>
            <Show when={!isLoading()} fallback={<Skeleton width={60} height={28} />}>
              <div class={`text-2xl font-bold mt-1 ${(crawl()?.today_failed ?? 0) > (crawl()?.today_success ?? 0) ? 'text-red-400' : 'text-cyan-400'}`}>
                {fmtNum(crawl()?.today_success)}
              </div>
            </Show>
            <div class="text-xs text-dark-500 mt-1">
              失败 {fmtNum(crawl()?.today_failed)} · 成功率 {fmtPct(crawl()?.today_rate)}
              <Show when={(crawl()?.feeds_paused ?? 0) > 0}>
                <span class="text-red-400"> · {crawl()?.feeds_paused} 源暂停</span>
              </Show>
            </div>
          </div>
        </A>

        {/* PKB Card */}
        <A href="/knowledge/overview" class="card card-hover relative overflow-hidden border-l-4 border-l-blue-500 bg-gradient-to-br from-blue-500/10 to-blue-500/5 group">
          <div class="flex items-start justify-between">
            <div class="w-12 h-12 rounded-xl bg-blue-500/20 flex items-center justify-center">
              <svg class="w-6 h-6 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
              </svg>
            </div>
            <span class="text-xs text-dark-500 group-hover:text-blue-400 transition-colors">查看详情 →</span>
          </div>
          <div class="mt-4">
            <div class="text-sm text-dark-400">知识卡片</div>
            <Show when={!isLoading()} fallback={<Skeleton width={60} height={28} />}>
              <div class="text-2xl font-bold text-blue-400 mt-1">{fmtNum(pkb()?.cards)}</div>
            </Show>
            <div class="text-xs text-dark-500 mt-1">
              知识树 {pkb()?.trees ?? '--'} · 今日 +{pkb()?.cards_today ?? '--'}
            </div>
          </div>
        </A>

        {/* LLM Card */}
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
            <div class="text-sm text-dark-400">LLM 24h 请求</div>
            <Show when={!isLoading()} fallback={<Skeleton width={60} height={28} />}>
              <div class={`text-2xl font-bold ${brokenLLMChannels() > 0 ? 'text-red-400' : 'text-amber-400'} mt-1`}>
                {fmtNum(llm()?.requests_24h)}
              </div>
            </Show>
            <div class="text-xs text-dark-500 mt-1">
              健康渠道 {healthyLLMChannels()}/{totalLLMChannels()} · 错误 {fmtNum(llm()?.errors_24h)}
            </div>
          </div>
        </A>

        {/* Log Card */}
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
            <div class="text-sm text-dark-400">24h 错误日志</div>
            <Show when={!isLoading()} fallback={<Skeleton width={60} height={28} />}>
              <div class={`text-2xl font-bold ${logErrors() > 5 ? 'text-red-400' : 'text-emerald-400'} mt-1`}>
                {logErrors()}
              </div>
            </Show>
            <div class="text-xs text-dark-500 mt-1">警告 {logWarnings()} · Matrix 通知 {matrixStats()?.notifications_24h ?? '--'}</div>
          </div>
        </A>
      </div>

      {/* Crawl + PKB Detail */}
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        {/* Crawl Detail */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">内容采集</h2>
            <A href="/rss" class="text-sm text-cyan-400 hover:text-cyan-300">RSS 源管理 →</A>
          </div>
          <Show when={!isLoading()} fallback={
            <div class="grid grid-cols-3 gap-4">
              <For each={[1, 2, 3, 4, 5, 6]}>{() => <Skeleton height={72} class="rounded-xl" />}</For>
            </div>
          }>
            <div class="grid grid-cols-3 gap-4">
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">URL 总数</div>
                <div class="text-2xl font-bold text-white">{fmtNum(crawl()?.total_urls)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">爬取成功</div>
                <div class="text-2xl font-bold text-emerald-400">{fmtNum(crawl()?.success)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">爬取失败</div>
                <div class={`text-2xl font-bold ${(crawl()?.failed ?? 0) > 0 ? 'text-red-400' : 'text-dark-200'}`}>{fmtNum(crawl()?.failed)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">总成功率</div>
                <div class="text-2xl font-bold text-cyan-400">{fmtPct(crawl()?.success_rate)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">待处理</div>
                <div class="text-2xl font-bold text-amber-400">{fmtNum(crawl()?.pending)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">今日新增 URL</div>
                <div class="text-2xl font-bold text-white">{fmtNum(crawl()?.today_new)}</div>
              </div>
            </div>
            <div class="mt-4 flex items-center justify-between p-3 bg-dark-700/30 rounded-xl text-sm">
              <div class="flex items-center gap-2">
                <span class={`status-dot ${(crawl()?.feeds_paused ?? 0) > 0 ? 'status-dot-danger' : 'status-dot-success'}`} />
                <span class="text-dark-300">
                  RSS 源：活跃 <span class="text-emerald-400 font-semibold">{crawl()?.feeds_active ?? '--'}</span>
                  {' / '}暂停 <span class={`font-semibold ${(crawl()?.feeds_paused ?? 0) > 0 ? 'text-red-400' : 'text-dark-300'}`}>{crawl()?.feeds_paused ?? '--'}</span>
                  {' / '}共 {crawl()?.feeds_total ?? '--'}
                </span>
              </div>
              <span class="text-dark-500">最近爬取 {fmtRelTime(crawl()?.last_crawl_at)}</span>
            </div>
          </Show>
        </div>

        {/* PKB Detail */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">PKB 知识库</h2>
            <A href="/knowledge/overview" class="text-sm text-blue-400 hover:text-blue-300">查看详情 →</A>
          </div>
          <Show when={!isLoading()} fallback={
            <div class="grid grid-cols-3 gap-4">
              <For each={[1, 2, 3, 4, 5, 6]}>{() => <Skeleton height={72} class="rounded-xl" />}</For>
            </div>
          }>
            <div class="grid grid-cols-3 gap-4">
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">知识卡片</div>
                <div class="text-2xl font-bold text-blue-400">{fmtNum(pkb()?.cards)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">今日新卡片</div>
                <div class={`text-2xl font-bold ${(pkb()?.cards_today ?? 0) > 0 ? 'text-emerald-400' : 'text-dark-200'}`}>+{pkb()?.cards_today ?? '--'}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">知识树</div>
                <div class="text-2xl font-bold text-white">{pkb()?.trees ?? '--'}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">领域摘要</div>
                <div class="text-2xl font-bold text-white">{fmtNum(pkb()?.digests)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl col-span-2">
                <div class="text-sm text-dark-400 mb-1">知识文件（共 {fmtNum(knowledgeStats()?.total_files)}）</div>
                <div class="flex items-baseline gap-4">
                  <For each={byLayer()}>
                    {([layer, count]) => (
                      <span class="text-sm text-dark-300">
                        {layer} <span class="text-lg font-bold text-white">{fmtNum(count)}</span>
                      </span>
                    )}
                  </For>
                  <Show when={byLayer().length === 0}>
                    <span class="text-dark-500">--</span>
                  </Show>
                </div>
              </div>
            </div>
          </Show>
        </div>
      </div>

      {/* LLM Usage + Service Status */}
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        {/* LLM Usage */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">LLM 用量（24h）</h2>
            <A href="/llm" class="text-sm text-amber-400 hover:text-amber-300">查看详情 →</A>
          </div>
          <Show when={!isLoading()} fallback={
            <div class="grid grid-cols-3 gap-4">
              <For each={[1, 2, 3, 4, 5, 6]}>{() => <Skeleton height={72} class="rounded-xl" />}</For>
            </div>
          }>
            <div class="grid grid-cols-3 gap-4">
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">请求数</div>
                <div class="text-2xl font-bold text-amber-400">{fmtNum(llm()?.requests_24h)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">Tokens</div>
                <div class="text-2xl font-bold text-white">{fmtNum(llm()?.tokens_24h)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">费用</div>
                <div class="text-2xl font-bold text-white">{fmtCost(llm()?.cost_cents_24h)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">成功率</div>
                <div class={`text-2xl font-bold ${(llm()?.success_rate ?? 1) < 0.9 ? 'text-red-400' : 'text-emerald-400'}`}>{fmtPct(llm()?.success_rate)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">限流次数</div>
                <div class={`text-2xl font-bold ${(llm()?.rate_limits_24h ?? 0) > 0 ? 'text-amber-400' : 'text-dark-200'}`}>{fmtNum(llm()?.rate_limits_24h)}</div>
              </div>
              <div class="p-4 bg-dark-700/50 rounded-xl">
                <div class="text-sm text-dark-400 mb-1">健康渠道</div>
                <div class={`text-2xl font-bold ${brokenLLMChannels() > 0 ? 'text-red-400' : 'text-emerald-400'}`}>
                  {healthyLLMChannels()}/{totalLLMChannels()}
                </div>
              </div>
            </div>
          </Show>
        </div>

        {/* Service Status */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">服务状态</h2>
            <A href="/matrix" class="text-sm text-violet-400 hover:text-violet-300">
              Matrix 房间 {matrixStats()?.rooms ?? '--'} →
            </A>
          </div>
          <Show when={!isLoading()} fallback={
            <div class="space-y-3">
              <For each={[1, 2, 3]}>{() => <Skeleton height={56} class="rounded-xl" />}</For>
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
        </div>
      </div>

      {/* Activity Trend Chart */}
      <div class="card mb-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h2 class="text-lg font-semibold text-white">模块活跃度</h2>
            <p class="text-sm text-dark-400 mt-1">24h 操作量分布</p>
          </div>
          <A href="/logs/dashboard" class="text-sm text-emerald-400 hover:text-emerald-300">日志看板 →</A>
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
