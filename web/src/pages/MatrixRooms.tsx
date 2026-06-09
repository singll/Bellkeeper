import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { MatrixRoom } from '@/types'

const MatrixRooms: Component = () => {
  const toast = useToast()
  const [showModal, setShowModal] = createSignal(false)
  const [submitting, setSubmitting] = createSignal(false)
  const [roomId, setRoomId] = createSignal('')

  const [rooms, { refetch }] = createResource(() => matrixApi.listRooms({ page: 1, page_size: 100 }))

  const [form, setForm] = createSignal({
    room_name: '',
    room_type: 'notification',
  })

  const openCreateModal = () => {
    setForm({ room_name: '', room_type: 'notification' })
    setRoomId('')
    setShowModal(true)
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      const roomID = roomId() || `!${Date.now()}:matrix.singll.net`
      await matrixApi.createRoom({ ...form(), room_id: roomID })
      toast.success('房间创建成功')
      setShowModal(false)
      refetch()
    } catch (err) {
      toast.error('创建失败: ' + (err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (room: MatrixRoom) => {
    if (!confirm(`确定要删除房间 "${room.room_name || room.room_id}" 吗？此操作不可撤销。`)) return
    try {
      await matrixApi.deleteRoom(room.room_id)
      toast.success('房间已删除')
      refetch()
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    }
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">房间管理</h1>
          <p class="text-sm text-dark-400 mt-1">管理 Matrix 平台房间</p>
        </div>
        <button class="btn btn-primary" onClick={openCreateModal}>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          注册房间
        </button>
      </div>

      {/* Table */}
      <div class="card overflow-hidden p-0">
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>房间 ID</th>
                <th>名称</th>
                <th>类型</th>
                <th>状态</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!rooms.loading}
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
                  when={rooms()?.data && rooms()!.data.length > 0}
                  fallback={
                    <tr>
                      <td colspan="5">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
                          </svg>
                          <p class="empty-state-title">暂无注册房间</p>
                          <p class="empty-state-description">点击"注册房间"添加第一个 Matrix 房间</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={rooms()?.data ?? []}>
                    {(room) => (
                      <tr class="group">
                        <td>
                          <span class="font-mono text-sm text-dark-400 truncate max-w-[200px] block" title={room.room_id}>
                            {room.room_id}
                          </span>
                        </td>
                        <td>
                          <div class="flex items-center gap-2">
                            <svg class="w-4 h-4 text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
                            </svg>
                            <span class="font-medium text-white">{room.room_name || '-'}</span>
                          </div>
                        </td>
                        <td>
                          <span class="badge badge-gray">{room.room_type}</span>
                        </td>
                        <td>
                          <div class="flex items-center gap-2">
                            <span class={`status-dot ${room.is_active ? 'status-dot-success' : 'status-dot-gray'}`} />
                            <span class={room.is_active ? 'text-emerald-400' : 'text-dark-500'}>
                              {room.is_active ? '活跃' : '禁用'}
                            </span>
                          </div>
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              class="btn btn-ghost btn-sm text-red-400 hover:text-red-300 hover:bg-red-500/10"
                              onClick={() => handleDelete(room)}
                            >
                              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                              </svg>
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

      {/* Modal */}
      <Modal
        open={showModal()}
        onClose={() => setShowModal(false)}
        title="注册 Matrix 房间"
        size="md"
        footer={
          <>
            <button type="button" class="btn btn-secondary" onClick={() => setShowModal(false)}>
              取消
            </button>
            <button
              type="submit"
              form="room-form"
              class="btn btn-primary"
              disabled={submitting()}
            >
              {submitting() ? (
                <>
                  <div class="loading-spinner" />
                  处理中...
                </>
              ) : '创建'}
            </button>
          </>
        }
      >
        <form id="room-form" onSubmit={handleSubmit} class="space-y-4">
          <div>
            <label class="label">房间 ID *</label>
            <input
              type="text"
              class="input font-mono"
              required
              placeholder="!xxx:matrix.singll.net"
              value={roomId()}
              onInput={(e) => setRoomId(e.currentTarget.value)}
            />
          </div>
          <div>
            <label class="label">房间名称</label>
            <input
              type="text"
              class="input"
              placeholder="如：通知房间"
              value={form().room_name}
              onInput={(e) => setForm({ ...form(), room_name: e.currentTarget.value })}
            />
          </div>
          <div>
            <label class="label">房间类型 *</label>
            <select
              class="input"
              value={form().room_type}
              onChange={(e) => setForm({ ...form(), room_type: e.currentTarget.value })}
            >
              <option value="notification">通知</option>
              <option value="command">命令</option>
              <option value="general">通用</option>
            </select>
          </div>
        </form>
      </Modal>
    </div>
  )
}

export default MatrixRooms
