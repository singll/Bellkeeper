import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import { useToast } from '@/components/Toast'
import type { MatrixCommand } from '@/types'

const MatrixCommands: Component = () => {
  const toast = useToast()
  const [testing, setTesting] = createSignal<MatrixCommand | null>(null)
  const [testResult, setTestResult] = createSignal<string | null>(null)

  const [commands] = createResource(() => matrixApi.listCommands())

  const handleToggle = async (cmd: MatrixCommand) => {
    try {
      await matrixApi.updateCommand(cmd.name as unknown as number, { is_active: !cmd.is_active })
      toast.success('状态已更新')
      commands.refetch()
    } catch (err) {
      toast.error('更新失败: ' + (err as Error).message)
    }
  }

  const handleTest = async () => {
    const cmd = testing()
    if (!cmd) return
    try {
      setTestResult('正在执行...')
      const res = await matrixApi.testCommand(cmd.name as unknown as number, {
        room_id: '',
        user_id: '',
        args: '',
      })
      setTestResult(res.response || res.message || '执行完成')
    } catch (err) {
      setTestResult('错误: ' + (err as Error).message)
    }
  }

  const getHandlerBadgeClass = (type: string) => {
    switch (type) {
      case 'builtin':
        return 'badge-purple'
      case 'n8n':
        return 'badge-blue'
      case 'api':
        return 'badge-orange'
      default:
        return 'badge-gray'
    }
  }

  const getHandlerLabel = (type: string) => {
    switch (type) {
      case 'builtin':
        return '内置'
      case 'n8n':
        return 'n8n'
      case 'api':
        return 'API'
      default:
        return type
    }
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">命令管理</h1>
          <p class="text-sm text-dark-400 mt-1">管理 Matrix 命令路由</p>
        </div>
      </div>

      {/* Table */}
      <div class="card overflow-hidden p-0">
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>命令</th>
                <th>描述</th>
                <th>类型</th>
                <th>权限</th>
                <th>状态</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!commands.loading}
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
                  when={commands()?.data && commands()!.data.length > 0}
                  fallback={
                    <tr>
                      <td colspan="6">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
                          </svg>
                          <p class="empty-state-title">暂无命令</p>
                          <p class="empty-state-description">命令在系统初始化时自动注册</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={commands()?.data ?? []}>
                    {(cmd) => (
                      <tr class="group">
                        <td>
                          <div class="flex items-center gap-2">
                            <svg class="w-4 h-4 text-dark-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
                            </svg>
                            <span class="font-mono font-medium text-white">{cmd.name}</span>
                          </div>
                        </td>
                        <td>
                          <span class="text-dark-400 text-sm">{cmd.description || '-'}</span>
                        </td>
                        <td>
                          <span class={`badge ${getHandlerBadgeClass(cmd.handler_type)}`}>
                            {getHandlerLabel(cmd.handler_type)}
                          </span>
                        </td>
                        <td>
                          <span class="badge badge-gray">{cmd.permission_level}</span>
                        </td>
                        <td>
                          <button
                            onClick={() => handleToggle(cmd)}
                            class="relative inline-flex items-center w-12 h-6 rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 focus:ring-offset-dark-900"
                            classList={{
                              'bg-emerald-500': cmd.is_active,
                              'bg-dark-600': !cmd.is_active,
                            }}
                          >
                            <span
                              class="inline-block w-4 h-4 transform bg-white rounded-full transition-transform shadow-sm"
                              classList={{
                                'translate-x-7': cmd.is_active,
                                'translate-x-1': !cmd.is_active,
                              }}
                            />
                          </button>
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={() => {
                                setTesting(cmd)
                                setTestResult(null)
                              }}
                            >
                              测试
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

      {/* Test Modal */}
      <Show when={testing()}>
        <div class="fixed inset-0 z-50 flex items-center justify-center">
          <div class="fixed inset-0 bg-black/60" onClick={() => setTesting(null)} />
          <div class="relative bg-dark-800 rounded-xl border border-dark-700 shadow-2xl w-full max-w-lg mx-4">
            <div class="flex items-center justify-between px-6 py-4 border-b border-dark-700">
              <h3 class="text-lg font-semibold text-white">测试命令: {testing()?.name}</h3>
              <button class="btn btn-ghost btn-sm p-1" onClick={() => setTesting(null)}>
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div class="p-6 space-y-4">
              <div class="p-4 bg-dark-900 rounded-lg">
                <div class="text-sm text-dark-400 mb-1">命令类型</div>
                <div class="text-white">{getHandlerLabel(testing()!.handler_type)}</div>
              </div>
              <Show when={testResult()}>
                <div class="p-4 bg-dark-900 rounded-lg">
                  <div class="text-sm text-dark-400 mb-1">执行结果</div>
                  <pre class="text-sm text-dark-200 whitespace-pre-wrap">{testResult()}</pre>
                </div>
              </Show>
            </div>
            <div class="flex justify-end gap-3 px-6 py-4 border-t border-dark-700">
              <button class="btn btn-secondary" onClick={() => setTesting(null)}>
                关闭
              </button>
              <button class="btn btn-primary" onClick={handleTest}>
                执行测试
              </button>
            </div>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default MatrixCommands
