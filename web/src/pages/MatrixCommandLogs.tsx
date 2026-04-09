import { Component, createResource, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixCommandLog } from '@/types'

const MatrixCommandLogs: Component = () => {
  const [logs] = createResource(() => matrixApi.listCommandLogs())

  const formatTime = (time: string) => {
    const d = new Date(time)
    return d.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'success':
        return <span class="badge badge-green">成功</span>
      case 'failed':
        return <span class="badge badge-red">失败</span>
      default:
        return <span class="badge badge-gray">{status}</span>
    }
  }

  const formatDuration = (ms?: number) => {
    if (ms === undefined) return '-'
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(1)}s`
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">命令日志</h1>
          <p class="text-sm text-dark-400 mt-1">查看命令执行记录</p>
        </div>
      </div>

      {/* Feature not available notice */}
      <div class="card p-8">
        <div class="empty-state">
          <svg class="empty-state-icon text-dark-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <p class="empty-state-title">功能暂不可用</p>
          <p class="empty-state-description">命令日志功能的后端接口尚未实现</p>
        </div>
      </div>

      {/* Table - shows placeholder when data arrives */}
      <div class="card overflow-hidden p-0 mt-6">
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>时间</th>
                <th>命令</th>
                <th>用户</th>
                <th>房间</th>
                <th>状态</th>
                <th>耗时</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!logs.loading && logs()?.data?.data?.length > 0}
                fallback={
                  <tr>
                    <td colspan="6" class="text-center py-12">
                      <Show when={logs.loading}>
                        <div class="loading-spinner mx-auto" />
                        <p class="mt-3 text-dark-400">加载中...</p>
                      </Show>
                      <Show when={!logs.loading}>
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                          </svg>
                          <p class="empty-state-title">暂无命令日志</p>
                          <p class="empty-state-description">命令执行后将在此显示记录</p>
                        </div>
                      </Show>
                    </td>
                  </tr>
                }
              >
                <For each={logs()?.data?.data ?? []}>
                  {(log) => (
                    <tr>
                      <td>
                        <span class="text-dark-400 text-sm">{formatTime(log.created_at)}</span>
                      </td>
                      <td>
                        <span class="font-mono text-white">{log.command_name}</span>
                      </td>
                      <td>
                        <span class="text-dark-300">{log.sender}</span>
                      </td>
                      <td>
                        <span class="font-mono text-sm text-dark-400 truncate max-w-[150px] block" title={log.room_id}>
                          {log.room_id}
                        </span>
                      </td>
                      <td>{getStatusBadge(log.execution_status || 'success')}</td>
                      <td>
                        <span class="text-dark-400">{formatDuration(log.execution_time_ms)}</span>
                      </td>
                    </tr>
                  )}
                </For>
              </Show>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

export default MatrixCommandLogs
