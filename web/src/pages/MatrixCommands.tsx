import { Component, createSignal, onMount, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixCommand, MatrixRoom } from '@/types'
import Modal from '@/components/Modal'
import { useToast } from '@/components/Toast'

const MatrixCommands: Component = () => {
  const { success: showSuccess, error: showError } = useToast()
  const [commands, setCommands] = createSignal<MatrixCommand[]>([])
  const [rooms, setRooms] = createSignal<MatrixRoom[]>([])
  const [loading, setLoading] = createSignal(true)
  const [error, setError] = createSignal<string | null>(null)
  const [showCreate, setShowCreate] = createSignal(false)
  const [testing, setTesting] = createSignal<MatrixCommand | null>(null)

  const [form, setForm] = createSignal({
    name: '',
    description: '',
    handler_type: 'n8n' as 'builtin' | 'n8n' | 'api',
    handler_config: {} as Record<string, unknown>,
    permission_scope: 'all',
    is_enabled: true,
  })

  const [testForm, setTestForm] = createSignal({
    room_id: '',
    user_id: '',
    args: '',
  })
  const [testResult, setTestResult] = createSignal<string | null>(null)

  const loadData = async () => {
    setLoading(true)
    try {
      const [commandsRes, roomsRes] = await Promise.all([
        matrixApi.listCommands(),
        matrixApi.listRooms({ page: 1, page_size: 100 }),
      ])
      setCommands(commandsRes.data || [])
      setRooms(roomsRes.data.data || [])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load data')
    } finally {
      setLoading(false)
    }
  }

  onMount(loadData)

  const handleCreate = async () => {
    try {
      await matrixApi.createCommand(form())
      showSuccess('命令创建成功')
      setShowCreate(false)
      setForm({ name: '', description: '', handler_type: 'n8n', handler_config: {}, permission_scope: 'all', is_enabled: true })
      loadData()
    } catch (e) {
      showError(e instanceof Error ? e.message : '创建失败')
    }
  }

  const handleToggle = async (cmd: MatrixCommand) => {
    try {
      await matrixApi.updateCommand(cmd.id, { ...cmd, is_enabled: !cmd.is_enabled })
      showSuccess('状态已更新')
      loadData()
    } catch (e) {
      showError(e instanceof Error ? e.message : '更新失败')
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除这个命令吗？')) return
    try {
      await matrixApi.deleteCommand(id)
      showSuccess('命令已删除')
      loadData()
    } catch (e) {
      showError(e instanceof Error ? e.message : '删除失败')
    }
  }

  const handleTest = async () => {
    const cmd = testing()
    if (!cmd) return
    try {
      const res = await matrixApi.testCommand(cmd.id, testForm())
      setTestResult(res.response || res.message)
    } catch (e) {
      setTestResult(`错误: ${e instanceof Error ? e.message : 'Unknown error'}`)
    }
  }

  return (
    <div class="p-6">
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">命令管理</h1>
        <button
          onClick={() => setShowCreate(true)}
          class="px-4 py-2 bg-primary text-white rounded hover:bg-primary/80"
        >
          注册命令
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
              <th class="px-4 py-3 text-left text-sm font-medium">命令</th>
              <th class="px-4 py-3 text-left text-sm font-medium">描述</th>
              <th class="px-4 py-3 text-left text-sm font-medium">类型</th>
              <th class="px-4 py-3 text-left text-sm font-medium">权限</th>
              <th class="px-4 py-3 text-left text-sm font-medium">启用</th>
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
            <For each={commands()} fallback={
              <tr>
                <td colspan="6" class="px-4 py-8 text-center text-muted-foreground">
                  暂无命令
                </td>
              </tr>
            }>
              {(cmd) => (
                <tr class="hover:bg-muted/50">
                  <td class="px-4 py-3 text-sm font-mono">{cmd.name}</td>
                  <td class="px-4 py-3 text-sm">{cmd.description || '-'}</td>
                  <td class="px-4 py-3 text-sm">
                    <span class={`px-2 py-0.5 text-xs rounded ${
                      cmd.handler_type === 'builtin'
                        ? 'bg-purple-500/20 text-purple-400'
                        : cmd.handler_type === 'n8n'
                          ? 'bg-blue-500/20 text-blue-400'
                          : 'bg-orange-500/20 text-orange-400'
                    }`}>
                      {cmd.handler_type}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm">{cmd.permission_scope}</td>
                  <td class="px-4 py-3">
                    <button
                      onClick={() => handleToggle(cmd)}
                      class={`w-12 h-6 rounded-full transition-colors ${
                        cmd.is_enabled ? 'bg-green-500' : 'bg-gray-600'
                      }`}
                    >
                      <div class={`w-4 h-4 rounded-full bg-white transition-transform ${
                        cmd.is_enabled ? 'translate-x-7' : 'translate-x-1'
                      }`} />
                    </button>
                  </td>
                  <td class="px-4 py-3 text-right">
                    <button
                      onClick={() => setTesting(cmd)}
                      class="text-blue-400 hover:underline mr-3"
                    >
                      测试
                    </button>
                    <button
                      onClick={() => handleDelete(cmd.id)}
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
        title="注册 Matrix 命令"
        onConfirm={handleCreate}
      >
        <div class="space-y-4">
          <div>
            <label class="block text-sm mb-1">命令名称</label>
            <input
              type="text"
              value={form().name}
              onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
              placeholder="!custom"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">描述</label>
            <input
              type="text"
              value={form().description}
              onInput={(e) => setForm({ ...form(), description: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">命令类型</label>
            <select
              value={form().handler_type}
              onChange={(e) => setForm({ ...form(), handler_type: e.currentTarget.value as any })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            >
              <option value="builtin">内置</option>
              <option value="n8n">n8n Webhook</option>
              <option value="api">API</option>
            </select>
          </div>
          <Show when={form().handler_type === 'n8n'}>
            <div>
              <label class="block text-sm mb-1">Webhook URL</label>
              <input
                type="text"
                value={(form().handler_config.webhook as string) || ''}
                onInput={(e) => setForm({
                  ...form(),
                  handler_config: { ...form().handler_config, webhook: e.currentTarget.value }
                })}
                class="w-full px-3 py-2 bg-background border border-border rounded"
              />
            </div>
          </Show>
          <div>
            <label class="block text-sm mb-1">权限范围</label>
            <select
              value={form().permission_scope}
              onChange={(e) => setForm({ ...form(), permission_scope: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            >
              <option value="all">所有人</option>
              <option value="admin">仅管理员</option>
            </select>
          </div>
          <label class="flex items-center gap-2">
            <input
              type="checkbox"
              checked={form().is_enabled}
              onChange={(e) => setForm({ ...form(), is_enabled: e.currentTarget.checked })}
            />
            <span class="text-sm">立即启用</span>
          </label>
        </div>
      </Modal>

      {/* Test Modal */}
      <Modal
        open={!!testing()}
        onClose={() => { setTesting(null); setTestResult(null) }}
        title={`测试命令: ${testing()?.name}`}
        onConfirm={handleTest}
        confirmText="执行测试"
      >
        <div class="space-y-4">
          <div>
            <label class="block text-sm mb-1">房间 ID</label>
            <select
              value={testForm().room_id}
              onChange={(e) => setTestForm({ ...testForm(), room_id: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            >
              <option value="">选择房间</option>
              <For each={rooms()}>
                {(room) => <option value={room.room_id}>{room.name}</option>}
              </For>
            </select>
          </div>
          <div>
            <label class="block text-sm mb-1">用户 ID</label>
            <input
              type="text"
              value={testForm().user_id}
              onInput={(e) => setTestForm({ ...testForm(), user_id: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
              placeholder="@user:matrix.example.com"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">参数</label>
            <input
              type="text"
              value={testForm().args}
              onInput={(e) => setTestForm({ ...testForm(), args: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            />
          </div>
          <Show when={testResult()}>
            <div class="bg-muted rounded p-4">
              <div class="text-sm font-medium mb-2">执行结果</div>
              <pre class="text-sm whitespace-pre-wrap">{testResult()}</pre>
            </div>
          </Show>
        </div>
      </Modal>
    </div>
  )
}

export default MatrixCommands
