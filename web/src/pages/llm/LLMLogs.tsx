import { Component, For, Show, createSignal, createResource, onMount } from 'solid-js'
import type { LLMProxyLog, LLMChannelConfig } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import { formatDateTime } from './shared'

const LLMLogs: Component = () => {
  const toast = useToast()
  const [logs, setLogs] = createSignal<LLMProxyLog[]>([])
  const [loading, setLoading] = createSignal(true)
  const [filterChannel, setFilterChannel] = createSignal('')
  const [limit, setLimit] = createSignal(50)

  const [channelConfigsData] = createResource(() => llmProxyApi.listChannels())
  const channelConfigs = () => channelConfigsData()?.data || []

  const fetchLogs = async () => {
    setLoading(true)
    try {
      const ch = filterChannel() || undefined
      const res = await llmProxyApi.logs(ch, limit())
      setLogs(res.data || [])
    } catch (err) {
      toast.error('加载日志失败: ' + (err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  onMount(() => { fetchLogs() })

  const handleRefresh = async () => {
    await fetchLogs()
    toast.success('日志已刷新')
  }

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
      {/* Header */}
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">调用日志</h1>
        <p class="text-sm text-dark-400 mt-1">LLM Proxy 请求调用记录</p>
      </div>

      <div class="space-y-4">
        {/* Filters */}
        <div class="card">
          <div class="flex flex-wrap items-center gap-3">
            <div class="flex items-center gap-2">
              <label class="text-sm text-dark-400">渠道</label>
              <select class="input w-auto" value={filterChannel()} onChange={(e) => { setFilterChannel(e.currentTarget.value) }}>
                <option value="">全部</option>
                <For each={channelConfigs()}>
                  {(ch) => <option value={ch.name}>{ch.name}</option>}
                </For>
              </select>
            </div>
            <div class="flex items-center gap-2">
              <label class="text-sm text-dark-400">条数</label>
              <select class="input w-auto" value={limit()} onChange={(e) => { setLimit(parseInt(e.currentTarget.value)) }}>
                <option value="20">20</option>
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

        {/* Logs Table */}
        <Show
          when={!loading()}
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
    </div>
  )
}

export default LLMLogs
