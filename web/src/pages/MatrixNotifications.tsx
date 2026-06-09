import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import { useToast } from '@/components/Toast'
import type { MatrixNotification } from '@/types'

const MatrixNotifications: Component = () => {
  const toast = useToast()
  const [page, setPage] = createSignal(1)
  const [channelFilter, setChannelFilter] = createSignal('')
  const [selected, setSelected] = createSignal<MatrixNotification | null>(null)

  const [notifications, { refetch }] = createResource(
    () => ({ page: page(), channel: channelFilter() }),
    ({ page, channel }) => matrixApi.listNotifications({ page, page_size: 20, channel })
  )

  const [channels] = createResource(() => matrixApi.listChannels())

  const handleRetry = async (id: number) => {
    try {
      await matrixApi.retryNotification(id)
      toast.success('重试成功')
      refetch()
    } catch (err) {
      toast.error('重试失败: ' + (err as Error).message)
    }
  }

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
      case 'sent':
        return <span class="badge badge-green">成功</span>
      case 'failed':
        return <span class="badge badge-red">失败</span>
      case 'pending':
        return <span class="badge badge-yellow">待发送</span>
      default:
        return <span class="badge badge-gray">{status}</span>
    }
  }

  const totalPages = () => 1

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">通知管理</h1>
          <p class="text-sm text-dark-400 mt-1">查看通知发送记录</p>
        </div>
      </div>

      {/* Search */}
      <div class="card mb-6">
        <div class="flex items-center gap-3">
          <div class="relative flex-1">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-dark-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <select
              class="input pl-10"
              value={channelFilter()}
              onChange={(e) => {
                setChannelFilter(e.currentTarget.value)
                setPage(1)
              }}
            >
              <option value="">全部频道</option>
              <For each={channels()?.data ?? []}>
                {(ch) => <option value={ch.channel_name}>{ch.channel_name}</option>}
              </For>
            </select>
          </div>
          <button class="btn btn-secondary" onClick={() => refetch()}>
            刷新
          </button>
        </div>
      </div>

      {/* Table */}
      <div class="card overflow-hidden p-0">
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>时间</th>
                <th>频道</th>
                <th>状态</th>
                <th>重试次数</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!notifications.loading}
                fallback={
                  <tr>
                    <td colspan="5" class="text-center py-12">
                      <div class="loading-spinner mx-auto" />
                      <p class="mt-3 text-dark-400">加载中...</p>
                    </td>
                  </tr>
                }
              >
                <Show
                  when={notifications()?.data && notifications()!.data.length > 0}
                  fallback={
                    <tr>
                      <td colspan="5">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
                          </svg>
                          <p class="empty-state-title">暂无通知记录</p>
                          <p class="empty-state-description">发送通知后将在此显示记录</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={notifications()?.data ?? []}>
                    {(notif) => (
                      <tr class="group">
                        <td>
                          <span class="text-dark-400 text-sm">{formatTime(notif.created_at)}</span>
                        </td>
                        <td>
                          <div class="flex items-center gap-2">
                            <svg class="w-4 h-4 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0" />
                            </svg>
                            <span class="font-medium text-white">{notif.channel_name}</span>
                          </div>
                        </td>
                        <td>{getStatusBadge(notif.status)}</td>
                        <td>
                          <span class="text-dark-400">{notif.retry_count}</span>
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <Show when={notif.status === 'failed'}>
                              <button
                                class="btn btn-ghost btn-sm text-blue-400 hover:text-blue-300"
                                onClick={() => handleRetry(notif.id)}
                              >
                                重试
                              </button>
                            </Show>
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={() => setSelected(notif)}
                            >
                              详情
                            </button>
                          </div>
                        </td>
                      </tr>
                    )}
                  </For>
                </Show>
              </Show>
            </tbody>
          </table>
        </div>
      </div>

      {/* Pagination */}
      <Show when={totalPages() > 1}>
        <div class="flex items-center justify-between mt-4">
          <div class="text-sm text-dark-400">
            共 <span class="text-dark-200 font-medium">{notifications()?.data?.length || 0}</span> 条记录
          </div>
          <div class="flex gap-2">
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() === 1}
              onClick={() => setPage((p) => p - 1)}
            >
              上一页
            </button>
            <span class="btn btn-ghost btn-sm cursor-default">
              {page()} / {totalPages()}
            </span>
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() >= totalPages()}
              onClick={() => setPage((p) => p + 1)}
            >
              下一页
            </button>
          </div>
        </div>
      </Show>

      {/* Detail Modal */}
      <Show when={selected()}>
        <div class="fixed inset-0 z-50 flex items-center justify-center">
          <div class="fixed inset-0 bg-black/60" onClick={() => setSelected(null)} />
          <div class="relative bg-dark-800 rounded-xl border border-dark-700 shadow-2xl w-full max-w-lg mx-4">
            <div class="flex items-center justify-between px-6 py-4 border-b border-dark-700">
              <h3 class="text-lg font-semibold text-white">通知详情</h3>
              <button class="btn btn-ghost btn-sm p-1" onClick={() => setSelected(null)}>
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div class="p-6 space-y-4">
              <div class="grid grid-cols-2 gap-4">
                <div class="p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">ID</div>
                  <div class="font-mono text-sm text-dark-200">{selected()!.id}</div>
                </div>
                <div class="p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">频道</div>
                  <div class="text-sm text-white">{selected()!.channel_name}</div>
                </div>
                <div class="p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">状态</div>
                  <div class="mt-1">{getStatusBadge(selected()!.status)}</div>
                </div>
                <div class="p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">重试次数</div>
                  <div class="text-sm text-white">{selected()!.retry_count}</div>
                </div>
                <div class="col-span-2 p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">创建时间</div>
                  <div class="text-sm text-white">{formatTime(selected()!.created_at)}</div>
                </div>
              </div>
              <Show when={selected()!.message_content}>
                <div class="p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">消息内容</div>
                  <pre class="text-sm text-dark-200 whitespace-pre-wrap">{selected()!.message_content}</pre>
                </div>
              </Show>
              <Show when={selected()!.error_message}>
                <div class="p-3 bg-red-500/10 border border-red-500/30 rounded-lg">
                  <div class="text-xs text-red-400 mb-1">错误信息</div>
                  <pre class="text-sm text-red-300 whitespace-pre-wrap">{selected()!.error_message}</pre>
                </div>
              </Show>
            </div>
            <div class="flex justify-end gap-3 px-6 py-4 border-t border-dark-700">
              <Show when={selected()!.status === 'failed'}>
                <button
                  class="btn btn-primary"
                  onClick={() => {
                    handleRetry(selected()!.id)
                    setSelected(null)
                  }}
                >
                  重试发送
                </button>
              </Show>
              <button class="btn btn-secondary" onClick={() => setSelected(null)}>
                关闭
              </button>
            </div>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default MatrixNotifications
