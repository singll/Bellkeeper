import { Component, For, Show, createSignal, createResource, createMemo, onMount } from 'solid-js'
import type { LLMProxyLog, LLMAlertEvent } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import { formatDateTime, getSeverityBadge, getSeverityLabel } from './shared'

type Tab = 'logs' | 'alerts'

const LLMLogsAndAlerts: Component = () => {
  const toast = useToast()
  const [tab, setTab] = createSignal<Tab>('logs')

  const [logs, setLogs] = createSignal<LLMProxyLog[]>([])
  const [loadingLogs, setLoadingLogs] = createSignal(true)
  const [filterChannel, setFilterChannel] = createSignal('')
  const [logLimit, setLogLimit] = createSignal(50)

  const [alerts, setAlerts] = createSignal<LLMAlertEvent[]>([])
  const [loadingAlerts, setLoadingAlerts] = createSignal(true)
  const [severity, setSeverity] = createSignal('')
  const [alertType, setAlertType] = createSignal('')
  const [alertChannel, setAlertChannel] = createSignal('')
  const [hours, setHours] = createSignal(24)
  const [alertLimit, setAlertLimit] = createSignal(100)

  const [channelConfigsData] = createResource(() => llmProxyApi.listChannels())
  const channelConfigs = () => channelConfigsData()?.data || []

  const fetchLogs = async () => {
    setLoadingLogs(true)
    try {
      const ch = filterChannel() || undefined
      const res = await llmProxyApi.logs(ch, logLimit())
      setLogs(res.data || [])
    } catch (err) {
      toast.error('加载日志失败: ' + (err as Error).message)
    } finally {
      setLoadingLogs(false)
    }
  }

  const fetchAlerts = async () => {
    setLoadingAlerts(true)
    try {
      const res = await llmProxyApi.listAlerts({
        hours: hours(),
        limit: alertLimit(),
        severity: severity() || undefined,
        type: alertType() || undefined,
      })
      setAlerts(res.data || [])
    } catch (err) {
      toast.error('加载告警失败: ' + (err as Error).message)
    } finally {
      setLoadingAlerts(false)
    }
  }

  onMount(() => {
    fetchLogs()
    fetchAlerts()
  })

  const filteredAlerts = createMemo(() => {
    const ch = alertChannel()
    return ch ? alerts().filter((a) => a.channel_name === ch) : alerts()
  })

  const countBy = (sev: string) => filteredAlerts().filter((a) => a.severity === sev).length

  const statusCodeBadge = (code: number) => {
    if (code >= 200 && code < 300) return 'badge-success'
    if (code === 429) return 'badge-warning'
    if (code >= 500) return 'badge-danger'
    if (code >= 400) return 'badge-warning'
    return 'badge-gray'
  }

  const durationClass = (ms: number) => {
    if (ms === 0) return ''
    if (ms < 3000) return 'text-emerald-400'
    if (ms < 10000) return 'text-amber-400'
    return 'text-red-400'
  }

  return (
    <div class="animate-fade-in">
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">日志与告警</h1>
        <p class="text-sm text-dark-400 mt-1">调用日志与告警事件</p>
      </div>

      <div class="flex gap-2 mb-4">
        <button
          class={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${tab() === 'logs' ? 'bg-primary-500/20 text-primary-300 border border-primary-500/40' : 'bg-dark-700/30 text-dark-400 border border-dark-600/50 hover:bg-dark-700/50'}`}
          onClick={() => setTab('logs')}
        >
          调用日志
        </button>
        <button
          class={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${tab() === 'alerts' ? 'bg-primary-500/20 text-primary-300 border border-primary-500/40' : 'bg-dark-700/30 text-dark-400 border border-dark-600/50 hover:bg-dark-700/50'}`}
          onClick={() => setTab('alerts')}
        >
          告警事件
        </button>
      </div>

      <Show when={tab() === 'logs'}>
        <div class="space-y-4">
          <div class="card">
            <div class="flex flex-wrap items-center gap-3">
              <div class="flex items-center gap-2">
                <label class="text-sm text-dark-400">渠道</label>
                <select class="input w-auto" value={filterChannel()} onChange={(e) => setFilterChannel(e.currentTarget.value)}>
                  <option value="">全部</option>
                  <For each={channelConfigs()}>
                    {(ch) => <option value={ch.name}>{ch.name}</option>}
                  </For>
                </select>
              </div>
              <div class="flex items-center gap-2">
                <label class="text-sm text-dark-400">条数</label>
                <select class="input w-auto" value={logLimit()} onChange={(e) => setLogLimit(parseInt(e.currentTarget.value))}>
                  <option value="20">20</option>
                  <option value="50">50</option>
                  <option value="100">100</option>
                  <option value="200">200</option>
                </select>
              </div>
              <button class="btn btn-primary btn-sm" onClick={fetchLogs}>
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
                查询
              </button>
            </div>
          </div>

          <Show
            when={!loadingLogs()}
            fallback={
              <div class="card py-12">
                <div class="flex items-center justify-center">
                  <div class="loading-spinner" />
                  <span class="ml-3 text-dark-400">加载调用日志...</span>
                </div>
              </div>
            }
          >
            <Show
              when={logs().length > 0}
              fallback={
                <div class="card empty-state">
                  <p class="empty-state-title">暂无调用日志</p>
                  <p class="empty-state-description">当有请求通过 LLM Proxy 时，日志将显示在此处</p>
                </div>
              }
            >
              <div class="overflow-x-auto rounded-xl border border-dark-600/50">
                <table class="table">
                  <thead>
                    <tr>
                      <th>时间</th><th>渠道</th><th>模型</th><th>状态码</th><th>耗时</th><th>Prompt</th><th>Completion</th><th>调用者</th><th>错误</th>
                    </tr>
                  </thead>
                  <tbody>
                    <For each={logs()}>
                      {(log) => (
                        <tr>
                          <td class="text-xs text-dark-300 whitespace-nowrap">{formatDateTime(log.created_at)}</td>
                          <td class="font-medium text-white text-sm">{log.channel_name}</td>
                          <td class="text-sm text-dark-200 max-w-[150px] truncate">{log.model}</td>
                          <td><span class={`badge ${statusCodeBadge(log.status_code)}`}>{log.status_code || '--'}</span></td>
                          <td class={`text-sm ${durationClass(log.duration_ms)}`}>{log.duration_ms > 0 ? `${log.duration_ms}ms` : '--'}</td>
                          <td class="text-sm text-dark-300">{log.prompt_tokens > 0 ? log.prompt_tokens : '--'}</td>
                          <td class="text-sm text-dark-300">{log.comp_tokens > 0 ? log.comp_tokens : '--'}</td>
                          <td class="text-xs text-dark-400">{log.caller_id || '--'}</td>
                          <td class="text-xs text-red-400 max-w-[200px] truncate" title={log.error_message}>{log.error_message || '--'}</td>
                        </tr>
                      )}
                    </For>
                  </tbody>
                </table>
              </div>
            </Show>
          </Show>
        </div>
      </Show>

      <Show when={tab() === 'alerts'}>
        <div class="space-y-4">
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div class="stat-card"><div class="stat-label">严重</div><div class="stat-value text-red-400">{countBy('critical')}</div></div>
            <div class="stat-card"><div class="stat-label">错误</div><div class="stat-value text-red-300">{countBy('error')}</div></div>
            <div class="stat-card"><div class="stat-label">警告</div><div class="stat-value text-amber-400">{countBy('warning')}</div></div>
            <div class="stat-card"><div class="stat-label">信息</div><div class="stat-value text-primary-300">{countBy('info')}</div></div>
          </div>

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
                <select class="input w-auto" value={alertChannel()} onChange={(e) => setAlertChannel(e.currentTarget.value)}>
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
                <select class="input w-auto" value={alertLimit()} onChange={(e) => setAlertLimit(parseInt(e.currentTarget.value))}>
                  <option value="50">50</option>
                  <option value="100">100</option>
                  <option value="200">200</option>
                </select>
              </div>
              <button class="btn btn-primary btn-sm" onClick={fetchAlerts}>
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
                查询
              </button>
            </div>
          </div>

          <Show
            when={!loadingAlerts()}
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
      </Show>
    </div>
  )
}

export default LLMLogsAndAlerts
