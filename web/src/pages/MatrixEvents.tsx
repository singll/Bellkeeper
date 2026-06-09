import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixEvent } from '@/types'

const MatrixEvents: Component = () => {
  const [page, setPage] = createSignal(1)
  const [typeFilter, setTypeFilter] = createSignal('')
  const [roomFilter, setRoomFilter] = createSignal('')
  const [selected, setSelected] = createSignal<MatrixEvent | null>(null)

  const [events, { refetch }] = createResource(
    () => ({ page: page(), type: typeFilter(), room_id: roomFilter() }),
    ({ page, type, room_id }) => matrixApi.listEvents({ page, page_size: 20, event_type: type, room_id })
  )

  const formatTime = (time: string) => {
    const d = new Date(time)
    return d.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  const getTypeBadge = (type: string) => {
    switch (type) {
      case 'command':
        return <span class="badge badge-blue">命令</span>
      case 'message':
        return <span class="badge badge-green">消息</span>
      case 'member':
        return <span class="badge badge-purple">成员</span>
      default:
        return <span class="badge badge-gray">{type}</span>
    }
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'processed':
        return <span class="badge badge-green">已处理</span>
      case 'failed':
        return <span class="badge badge-red">失败</span>
      case 'pending':
        return <span class="badge badge-yellow">待处理</span>
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
          <h1 class="text-2xl font-bold text-white">事件日志</h1>
          <p class="text-sm text-dark-400 mt-1">查看 Matrix 事件记录</p>
        </div>
      </div>

      {/* Search */}
      <div class="card mb-6">
        <div class="flex items-center gap-3 flex-wrap">
          <div class="relative">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-dark-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <select
              class="input pl-10"
              value={typeFilter()}
              onChange={(e) => {
                setTypeFilter(e.currentTarget.value)
                setPage(1)
              }}
            >
              <option value="">全部类型</option>
              <option value="command">命令</option>
              <option value="message">消息</option>
              <option value="member">成员</option>
            </select>
          </div>
          <div class="relative flex-1 min-w-[200px]">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-dark-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              type="text"
              class="input pl-10"
              placeholder="房间 ID..."
              value={roomFilter()}
              onInput={(e) => {
                setRoomFilter(e.currentTarget.value)
                setPage(1)
              }}
            />
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
                <th>类型</th>
                <th>房间</th>
                <th>用户</th>
                <th>状态</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!events.loading}
                fallback={
                  <tr>
                    <td colspan="6" class="text-center py-12">
                      <div class="loading-spinner mx-auto" />
                      <p class="mt-3 text-dark-400">加载中...</p>
                    </td>
                  </tr>
                }
              >
                <Show
                  when={events()?.data && events()!.data.length > 0}
                  fallback={
                    <tr>
                      <td colspan="6">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                          </svg>
                          <p class="empty-state-title">暂无事件记录</p>
                          <p class="empty-state-description">Matrix 事件将在此显示</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={events()?.data ?? []}>
                    {(event) => (
                      <tr class="group">
                        <td>
                          <span class="text-dark-400 text-sm">{formatTime(event.created_at)}</span>
                        </td>
                        <td>{getTypeBadge(event.event_type)}</td>
                        <td>
                          <span class="font-mono text-sm text-dark-400 truncate max-w-[150px] block" title={event.room_id}>
                            {event.room_id}
                          </span>
                        </td>
                        <td>
                          <span class="text-dark-300">{event.sender}</span>
                        </td>
                        <td>{getStatusBadge(event.processing_status)}</td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={() => setSelected(event)}
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
            共 <span class="text-dark-200 font-medium">{events()?.data?.length || 0}</span> 条记录
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
              <h3 class="text-lg font-semibold text-white">事件详情</h3>
              <button class="btn btn-ghost btn-sm p-1" onClick={() => setSelected(null)}>
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div class="p-6 space-y-4">
              <div class="grid grid-cols-2 gap-4">
                <div class="p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">事件 ID</div>
                  <div class="font-mono text-sm text-dark-200 truncate" title={selected()!.event_id}>
                    {selected()!.event_id}
                  </div>
                </div>
                <div class="p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">类型</div>
                  <div class="mt-1">{getTypeBadge(selected()!.event_type)}</div>
                </div>
                <div class="col-span-2 p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">房间</div>
                  <div class="font-mono text-sm text-dark-200">{selected()!.room_id}</div>
                </div>
                <div class="col-span-2 p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">发送者</div>
                  <div class="text-sm text-white">{selected()!.sender}</div>
                </div>
                <div class="p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">状态</div>
                  <div class="mt-1">{getStatusBadge(selected()!.processing_status)}</div>
                </div>
                <div class="p-3 bg-dark-900 rounded-lg">
                  <div class="text-xs text-dark-500 mb-1">创建时间</div>
                  <div class="text-sm text-white">{formatTime(selected()!.created_at)}</div>
                </div>
              </div>
            </div>
            <div class="flex justify-end gap-3 px-6 py-4 border-t border-dark-700">
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

export default MatrixEvents
