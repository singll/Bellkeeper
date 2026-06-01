import { Component, For, Show, createSignal, createResource, createMemo, onMount } from 'solid-js'
import type { LLMAlertEvent } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import { formatDateTime, getSeverityBadge, getSeverityLabel } from './shared'

const LLMAlerts: Component = () => {
  const toast = useToast()
  const [alerts, setAlerts] = createSignal<LLMAlertEvent[]>([])
  const [loading, setLoading] = createSignal(true)
  const [severity, setSeverity] = createSignal('')
  const [alertType, setAlertType] = createSignal('')
  const [channel, setChannel] = createSignal('')
  const [hours, setHours] = createSignal(24)
  const [limit, setLimit] = createSignal(100)

  // Channel options reused from config; backend /llm/alerts has no channel param,
  // so channel filtering is done client-side below.
  const [channelConfigsData] = createResource(() => llmProxyApi.listChannels())
  const channelConfigs = () => channelConfigsData()?.data || []

  const fetchAlerts = async () => {
    setLoading(true)
    try {
      const res = await llmProxyApi.listAlerts({
        hours: hours(),
        limit: limit(),
        severity: severity() || undefined,
        type: alertType() || undefined,
      })
      setAlerts(res.data || [])
    } catch (err) {
      toast.error('加载告警失败: ' + (err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  onMount(() => { fetchAlerts() })

  const handleRefresh = async () => {
    await fetchAlerts()
    toast.success('告警已刷新')
  }

  // Client-side channel filter (no server param available)
  const filteredAlerts = createMemo(() => {
    const ch = channel()
    return ch ? alerts().filter((a) => a.channel_name === ch) : alerts()
  })

  const countBy = (sev: string) => alerts().filter((a) => a.severity === sev).length

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">告警</h1>
        <p class="text-sm text-dark-400 mt-1">LLM Proxy 聚合告警事件（熔断 / 配额 / 余额 / 会话）</p>
      </div>

      <div class="space-y-4">
        {/* Severity summary */}
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <div class="stat-card"><div class="stat-label">严重</div><div class="stat-value text-red-400">{countBy('critical')}</div></div>
          <div class="stat-card"><div class="stat-label">错误</div><div class="stat-value text-red-300">{countBy('error')}</div></div>
          <div class="stat-card"><div class="stat-label">警告</div><div class="stat-value text-amber-400">{countBy('warning')}</div></div>
          <div class="stat-card"><div class="stat-label">信息</div><div class="stat-value text-primary-300">{countBy('info')}</div></div>
        </div>

        {/* Filters */}
        <div class="card">
          <div class="flex flex-wrap items-center gap-3">
            <div class="flex items-center gap-2">
              <label class="text-sm text-dark-400">级别</label>
              <select class="input w-auto" value={severity()} onChange={(e) => setSeverity(e.currentTarget.value)}>
                <option value="">全部</option>
                <option value="info">信息</option>
                <option value="warning">警告</option>
                <option value="error">错误</option>
                <option value="critical">严重</option>
              </select>
            </div>
            <div class="flex items-center gap-2">
              <label class="text-sm text-dark-400">类型</label>
              <input class="input w-auto" placeholder="如 circuit_open" value={alertType()} onInput={(e) => setAlertType(e.currentTarget.value)} />
            </div>
            <div class="flex items-center gap-2">
              <label class="text-sm text-dark-400">渠道</label>
              <select class="input w-auto" value={channel()} onChange={(e) => setChannel(e.currentTarget.value)}>
                <option value="">全部</option>
                <For each={channelConfigs()}>{(c) => <option value={c.name}>{c.name}</option>}</For>
              </select>
            </div>
            <div class="flex items-center gap-2">
              <label class="text-sm text-dark-400">范围</label>
              <select class="input w-auto" value={hours()} onChange={(e) => setHours(parseInt(e.currentTarget.value))}>
                <option value="24">24 小时</option>
                <option value="72">3 天</option>
                <option value="168">7 天</option>
              </select>
            </div>
            <div class="flex items-center gap-2">
              <label class="text-sm text-dark-400">条数</label>
              <select class="input w-auto" value={limit()} onChange={(e) => setLimit(parseInt(e.currentTarget.value))}>
                <option value="50">50</option>
                <option value="100">100</option>
                <option value="200">200</option>
              </select>
            </div>
            <button class="btn btn-primary btn-sm" onClick={handleRefresh}>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              查询
            </button>
          </div>
        </div>

        {/* Alerts table */}
        <Show
          when={!loading()}
          fallback={
            <div class="card py-12">
              <div class="flex items-center justify-center">
                <div class="loading-spinner" />
                <span class="ml-3 text-dark-400">加载告警事件...</span>
              </div>
            </div>
          }
        >
          <Show
            when={filteredAlerts().length > 0}
            fallback={
              <div class="card empty-state">
                <p class="empty-state-title">暂无告警事件</p>
                <p class="empty-state-description">所选时间范围与过滤条件下没有告警记录。</p>
              </div>
            }
          >
            <div class="overflow-x-auto rounded-xl border border-dark-600/50">
              <table class="table">
                <thead>
                  <tr>
                    <th>时间</th><th>级别</th><th>类型</th><th>渠道</th><th>消息</th><th>推送状态</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={filteredAlerts()}>
                    {(a) => (
                      <tr>
                        <td class="text-xs text-dark-300 whitespace-nowrap">{formatDateTime(a.created_at)}</td>
                        <td><span class={`badge ${getSeverityBadge(a.severity)}`}>{getSeverityLabel(a.severity)}</span></td>
                        <td class="text-xs font-mono text-dark-300">{a.alert_type}</td>
                        <td class="text-sm text-dark-200">{a.channel_name || '--'}</td>
                        <td class="text-sm text-dark-300 max-w-[360px]">{a.message}</td>
                        <td>
                          <span class={`badge ${a.flushed_at ? 'badge-success' : 'badge-gray'}`}>
                            {a.flushed_at ? '已推送' : '待推送'}
                          </span>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>
        </Show>
      </div>
    </div>
  )
}

export default LLMAlerts
