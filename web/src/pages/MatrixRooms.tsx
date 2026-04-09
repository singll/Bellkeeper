import { Component, createSignal, onMount, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixRoom } from '@/types'
import Modal from '@/components/Modal'
import { useToast } from '@/components/Toast'

const MatrixRooms: Component = () => {
  const { success: showSuccess, error: showError } = useToast()
  const [rooms, setRooms] = createSignal<MatrixRoom[]>([])
  const [loading, setLoading] = createSignal(true)
  const [error, setError] = createSignal<string | null>(null)
  const [showCreate, setShowCreate] = createSignal(false)
  const [editing, setEditing] = createSignal<MatrixRoom | null>(null)

  const [form, setForm] = createSignal({
    name: '',
    alias: '',
    is_public: true,
    is_encrypted: false,
    topic: '',
  })

  const loadRooms = async () => {
    setLoading(true)
    try {
      const res = await matrixApi.listRooms({ page: 1, page_size: 100 })
      setRooms(res.data.data || [])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load rooms')
    } finally {
      setLoading(false)
    }
  }

  onMount(loadRooms)

  const handleCreate = async () => {
    try {
      await matrixApi.createRoom(form())
      showSuccess('房间创建成功')
      setShowCreate(false)
      setForm({ name: '', alias: '', is_public: true, is_encrypted: false, topic: '' })
      loadRooms()
    } catch (e) {
      showError(e instanceof Error ? e.message : '创建失败')
    }
  }

  const handleUpdate = async () => {
    const room = editing()
    if (!room) return
    try {
      await matrixApi.updateRoom(room.id, form())
      showSuccess('房间更新成功')
      setEditing(null)
      loadRooms()
    } catch (e) {
      showError(e instanceof Error ? e.message : '更新失败')
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除这个房间吗？')) return
    try {
      await matrixApi.deleteRoom(id)
      showSuccess('房间已删除')
      loadRooms()
    } catch (e) {
      showError(e instanceof Error ? e.message : '删除失败')
    }
  }

  const openEdit = (room: MatrixRoom) => {
    setForm({
      name: room.name,
      alias: room.alias || '',
      is_public: room.is_public,
      is_encrypted: room.is_encrypted,
      topic: room.topic || '',
    })
    setEditing(room)
  }

  return (
    <div class="p-6">
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">房间管理</h1>
        <button
          onClick={() => setShowCreate(true)}
          class="px-4 py-2 bg-primary text-white rounded hover:bg-primary/80"
        >
          创建房间
        </button>
      </div>

      <Show when={error()}>
        <div class="bg-red-500/10 border border-red-500 text-red-500 rounded p-4 mb-4">
          {error()}
        </div>
      </Show>

      <div class="bg-card rounded-lg border border-border overflow-hidden">
        <table class="w-full">
          <thead class="bg-muted">
            <tr>
              <th class="px-4 py-3 text-left text-sm font-medium">房间 ID</th>
              <th class="px-4 py-3 text-left text-sm font-medium">名称</th>
              <th class="px-4 py-3 text-left text-sm font-medium">类型</th>
              <th class="px-4 py-3 text-left text-sm font-medium">成员数</th>
              <th class="px-4 py-3 text-left text-sm font-medium">创建时间</th>
              <th class="px-4 py-3 text-right text-sm font-medium">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border">
            <Show when={loading()}>
              <tr>
                <td colspan="6" class="px-4 py-8 text-center text-muted-foreground">
                  加载中...
                </td>
              </tr>
            </Show>
            <For each={rooms()} fallback={
              <tr>
                <td colspan="6" class="px-4 py-8 text-center text-muted-foreground">
                  暂无房间
                </td>
              </tr>
            }>
              {(room) => (
                <tr class="hover:bg-muted/50">
                  <td class="px-4 py-3 text-sm font-mono">{room.room_id.slice(0, 20)}...</td>
                  <td class="px-4 py-3 text-sm">{room.name}</td>
                  <td class="px-4 py-3 text-sm">
                    <span class={`px-2 py-0.5 text-xs rounded ${
                      room.is_public ? 'bg-green-500/20 text-green-400' : 'bg-yellow-500/20 text-yellow-400'
                    }`}>
                      {room.is_public ? '公开' : '私有'}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm">{room.member_count ?? '-'}</td>
                  <td class="px-4 py-3 text-sm text-muted-foreground">
                    {new Date(room.created_at).toLocaleDateString('zh-CN')}
                  </td>
                  <td class="px-4 py-3 text-right">
                    <button
                      onClick={() => openEdit(room)}
                      class="text-primary hover:underline mr-3"
                    >
                      编辑
                    </button>
                    <button
                      onClick={() => handleDelete(room.id)}
                      class="text-red-500 hover:underline"
                    >
                      删除
                    </button>
                  </td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
      </div>

      {/* Create Modal */}
      <Modal
        open={showCreate()}
        onClose={() => setShowCreate(false)}
        title="创建 Matrix 房间"
        onConfirm={handleCreate}
      >
        <div class="space-y-4">
          <div>
            <label class="block text-sm mb-1">房间名称</label>
            <input
              type="text"
              value={form().name}
              onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
              placeholder="通知房间"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">房间别名</label>
            <input
              type="text"
              value={form().alias}
              onInput={(e) => setForm({ ...form(), alias: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
              placeholder="optional-alias"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">房间主题</label>
            <input
              type="text"
              value={form().topic}
              onInput={(e) => setForm({ ...form(), topic: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            />
          </div>
          <div class="flex items-center gap-4">
            <label class="flex items-center gap-2">
              <input
                type="checkbox"
                checked={form().is_public}
                onChange={(e) => setForm({ ...form(), is_public: e.currentTarget.checked })}
              />
              <span class="text-sm">公开房间</span>
            </label>
            <label class="flex items-center gap-2">
              <input
                type="checkbox"
                checked={form().is_encrypted}
                onChange={(e) => setForm({ ...form(), is_encrypted: e.currentTarget.checked })}
              />
              <span class="text-sm">启用端到端加密</span>
            </label>
          </div>
        </div>
      </Modal>

      {/* Edit Modal */}
      <Modal
        open={!!editing()}
        onClose={() => setEditing(null)}
        title={`编辑房间: ${editing()?.name}`}
        onConfirm={handleUpdate}
      >
        <div class="space-y-4">
          <div>
            <label class="block text-sm mb-1">房间名称</label>
            <input
              type="text"
              value={form().name}
              onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">房间主题</label>
            <input
              type="text"
              value={form().topic}
              onInput={(e) => setForm({ ...form(), topic: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            />
          </div>
          <div class="flex items-center gap-4">
            <label class="flex items-center gap-2">
              <input
                type="checkbox"
                checked={form().is_public}
                onChange={(e) => setForm({ ...form(), is_public: e.currentTarget.checked })}
              />
              <span class="text-sm">公开房间</span>
            </label>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default MatrixRooms
