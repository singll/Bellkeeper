import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { MatrixChannel, MatrixRoom } from '@/types'

const MatrixChannels: Component = () => {
  const toast = useToast()
  const [editing, setEditing] = createSignal<MatrixChannel | null>(null)
  const [submitting, setSubmitting] = createSignal(false)

  const [channels] = createResource(() => matrixApi.listChannels())
  const [rooms] = createResource(() => matrixApi.listRooms({ page: 1, page_size: 100 }))

  const [form, setForm] = createSignal({
    is_active: true,
    priority: 0,
    room_id: '',
  })

  const openEditModal = (channel: MatrixChannel) => {
    setForm({
      is_active: channel.is_active,
      priority: channel.priority,
      room_id: channel.room_id,
    })
    setEditing(channel)
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    const channel = editing()
    if (!channel) return

    setSubmitting(true)
    try {
      await matrixApi.updateChannel(channel.channel_name, form())
      toast.success('频道更新成功')
      setEditing(null)
    } catch (err) {
      toast.error('更新失败: ' + (err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  const getRoomName = (roomId: string) => {
    const room = rooms()?.data?.find((r) => r.room_id === roomId)
    return room?.room_name || roomId.slice(0, 15) + '...'
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">频道管理</h1>
          <p class="text-sm text-dark-400 mt-1">管理 Matrix 通知频道</p>
        </div>
      </div>

      {/* Grid */}
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <Show
          when={!channels.loading}
          fallback={
            <For each={[1, 2, 3]}>
              {() => (
                <div class="card p-6">
                  <div class="loading-skeleton h-5 w-24 mb-3" />
                  <div class="loading-skeleton h-4 w-32 mb-2" />
                  <div class="loading-skeleton h-4 w-40" />
                </div>
              )}
            </For>
          }
        >
          <Show
            when={channels()?.data && channels()!.data.length > 0}
            fallback={
              <div class="col-span-full">
                <div class="card p-12">
                  <div class="empty-state">
                    <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0" />
                    </svg>
                    <p class="empty-state-title">暂无频道</p>
                    <p class="empty-state-description">频道在系统初始化时自动创建</p>
                  </div>
                </div>
              </div>
            }
          >
            <For each={channels()?.data ?? []}>
              {(channel) => (
                <div class="card p-6 group">
                  <div class="flex items-start justify-between mb-4">
                    <div class="flex items-center gap-3">
                      <div class="w-10 h-10 rounded-lg bg-primary-500/20 flex items-center justify-center">
                        <svg class="w-5 h-5 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0" />
                        </svg>
                      </div>
                      <div>
                        <h3 class="font-semibold text-white">{channel.channel_name}</h3>
                        <div class="flex items-center gap-2 mt-1">
                          <span class={`status-dot ${channel.is_active ? 'status-dot-success' : 'status-dot-gray'}`} />
                          <span class={`text-xs ${channel.is_active ? 'text-emerald-400' : 'text-dark-500'}`}>
                            {channel.is_active ? '启用' : '禁用'}
                          </span>
                        </div>
                      </div>
                    </div>
                    <button
                      class="btn btn-ghost btn-sm opacity-0 group-hover:opacity-100 transition-opacity"
                      onClick={() => openEditModal(channel)}
                    >
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                      </svg>
                    </button>
                  </div>
                  <div class="space-y-2 text-sm">
                    <div class="flex items-center justify-between">
                      <span class="text-dark-400">绑定房间</span>
                      <span class="font-mono text-dark-300 text-xs" title={channel.room_id}>
                        {getRoomName(channel.room_id)}
                      </span>
                    </div>
                    <div class="flex items-center justify-between">
                      <span class="text-dark-400">优先级</span>
                      <span class="text-dark-300">{channel.priority}</span>
                    </div>
                  </div>
                </div>
              )}
            </For>
          </Show>
        </Show>
      </div>

      {/* Edit Modal */}
      <Modal
        open={!!editing()}
        onClose={() => setEditing(null)}
        title={`编辑频道: ${editing()?.channel_name}`}
        size="md"
        footer={
          <>
            <button type="button" class="btn btn-secondary" onClick={() => setEditing(null)}>
              取消
            </button>
            <button
              type="submit"
              form="channel-form"
              class="btn btn-primary"
              disabled={submitting()}
            >
              {submitting() ? (
                <>
                  <div class="loading-spinner" />
                  处理中...
                </>
              ) : '保存'}
            </button>
          </>
        }
      >
        <form id="channel-form" onSubmit={handleSubmit} class="space-y-4">
          <div>
            <label class="label">绑定房间</label>
            <select
              class="input"
              value={form().room_id}
              onChange={(e) => setForm({ ...form(), room_id: e.currentTarget.value })}
            >
              <option value="">选择房间</option>
              <For each={rooms()?.data ?? []}>
                {(room) => (
                  <option value={room.room_id}>{room.room_name || room.room_id}</option>
                )}
              </For>
            </select>
          </div>
          <div>
            <label class="label">优先级</label>
            <input
              type="number"
              class="input"
              value={form().priority}
              onInput={(e) => setForm({ ...form(), priority: parseInt(e.currentTarget.value) || 0 })}
            />
          </div>
          <div class="flex items-center gap-3 pt-2">
            <label class="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                class="sr-only peer"
                checked={form().is_active}
                onChange={(e) => setForm({ ...form(), is_active: e.currentTarget.checked })}
              />
              <div class="w-11 h-6 bg-dark-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-primary-500 rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
              <span class="ms-3 text-sm font-medium text-dark-300">启用频道</span>
            </label>
          </div>
        </form>
      </Modal>
    </div>
  )
}

export default MatrixChannels
