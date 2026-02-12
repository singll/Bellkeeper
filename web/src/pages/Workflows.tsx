import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { workflowsApi } from '@/api'
import type { Workflow, WorkflowExecution } from '@/types'

const Workflows: Component = () => {
  const [selectedWorkflow, setSelectedWorkflow] = createSignal<string | null>(null)
  const [triggerName, setTriggerName] = createSignal('')
  const [triggerPayload, setTriggerPayload] = createSignal('{}')
  const [showTriggerModal, setShowTriggerModal] = createSignal(false)
  const [triggerResult, setTriggerResult] = createSignal<string | null>(null)

  const [workflows, { refetch: refetchWorkflows }] = createResource(
    () => workflowsApi.list()
  )

  const [executions, { refetch: refetchExecutions }] = createResource(
    () => selectedWorkflow(),
    (workflowId) => workflowsApi.executions(workflowId || undefined, 20),
    { initialValue: { data: [] } }
  )

  const handleToggleActive = async (workflow: Workflow) => {
    try {
      if (workflow.active) {
        await workflowsApi.deactivate(workflow.id)
      } else {
        await workflowsApi.activate(workflow.id)
      }
      refetchWorkflows()
    } catch (err) {
      alert('操作失败: ' + (err as Error).message)
    }
  }

  const handleTrigger = async () => {
    try {
      let payload = {}
      try {
        payload = JSON.parse(triggerPayload())
      } catch {
        alert('无效的 JSON 格式')
        return
      }

      const result = await workflowsApi.trigger(triggerName(), payload)
      setTriggerResult(JSON.stringify(result.data, null, 2))
      refetchExecutions()
    } catch (err) {
      setTriggerResult('触发失败: ' + (err as Error).message)
    }
  }

  const openTriggerModal = (name: string) => {
    setTriggerName(name)
    setTriggerPayload('{}')
    setTriggerResult(null)
    setShowTriggerModal(true)
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'success':
        return 'bg-green-500/20 text-green-400'
      case 'error':
        return 'bg-red-500/20 text-red-400'
      case 'waiting':
        return 'bg-yellow-500/20 text-yellow-400'
      default:
        return 'bg-dark-border text-dark-muted'
    }
  }

  const formatDate = (dateStr: string) => {
    if (!dateStr) return '--'
    return new Date(dateStr).toLocaleString()
  }

  return (
    <div>
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">工作流管理</h1>
        <button
          class="btn btn-primary"
          onClick={() => openTriggerModal('')}
        >
          手动触发
        </button>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Workflow List */}
        <div class="card">
          <h2 class="text-lg font-semibold mb-4">n8n 工作流</h2>
          <Show
            when={!workflows.loading}
            fallback={<div class="text-dark-muted py-4 text-center">加载中...</div>}
          >
            <Show
              when={workflows()?.data && workflows()!.data.length > 0}
              fallback={
                <div class="text-dark-muted py-4 text-center">
                  暂无工作流数据
                  <p class="text-sm mt-2">请配置 n8n API Key 以获取工作流列表</p>
                </div>
              }
            >
              <div class="space-y-3">
                <For each={workflows()?.data}>
                  {(workflow) => (
                    <div
                      class="p-4 bg-dark-bg rounded-lg cursor-pointer transition-colors hover:bg-dark-border"
                      classList={{
                        'ring-2 ring-primary-500': selectedWorkflow() === workflow.id,
                      }}
                      onClick={() => {
                        setSelectedWorkflow(workflow.id)
                        refetchExecutions()
                      }}
                    >
                      <div class="flex items-center justify-between">
                        <div class="flex-1">
                          <div class="font-medium">{workflow.name}</div>
                          <div class="text-sm text-dark-muted mt-1">
                            ID: {workflow.id}
                          </div>
                          <Show when={workflow.tags && workflow.tags.length > 0}>
                            <div class="flex gap-1 mt-2">
                              <For each={workflow.tags}>
                                {(tag) => (
                                  <span class="px-2 py-0.5 bg-dark-card rounded text-xs">
                                    {tag.name}
                                  </span>
                                )}
                              </For>
                            </div>
                          </Show>
                        </div>
                        <div class="flex items-center gap-3">
                          <button
                            class="text-sm text-primary-500 hover:text-primary-400"
                            onClick={(e) => {
                              e.stopPropagation()
                              openTriggerModal(workflow.name)
                            }}
                          >
                            触发
                          </button>
                          <button
                            class={`px-3 py-1 rounded text-sm transition-colors ${
                              workflow.active
                                ? 'bg-green-500/20 text-green-400 hover:bg-green-500/30'
                                : 'bg-dark-card text-dark-muted hover:bg-dark-border'
                            }`}
                            onClick={(e) => {
                              e.stopPropagation()
                              handleToggleActive(workflow)
                            }}
                          >
                            {workflow.active ? '已激活' : '已停用'}
                          </button>
                        </div>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </Show>
        </div>

        {/* Execution History */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold">执行历史</h2>
            <Show when={selectedWorkflow()}>
              <button
                class="text-sm text-primary-500 hover:text-primary-400"
                onClick={() => refetchExecutions()}
              >
                刷新
              </button>
            </Show>
          </div>
          <Show
            when={selectedWorkflow()}
            fallback={
              <div class="text-dark-muted py-4 text-center">
                选择一个工作流查看执行历史
              </div>
            }
          >
            <Show
              when={!executions.loading}
              fallback={<div class="text-dark-muted py-4 text-center">加载中...</div>}
            >
              <Show
                when={executions()?.data && executions()!.data.length > 0}
                fallback={
                  <div class="text-dark-muted py-4 text-center">
                    暂无执行记录
                    <p class="text-sm mt-2">请配置 n8n API Key 以获取执行历史</p>
                  </div>
                }
              >
                <div class="space-y-2 max-h-96 overflow-y-auto">
                  <For each={executions()?.data}>
                    {(execution) => (
                      <div class="p-3 bg-dark-bg rounded-lg">
                        <div class="flex items-center justify-between">
                          <div class="flex items-center gap-2">
                            <span
                              class={`px-2 py-0.5 rounded text-xs ${getStatusColor(
                                execution.status
                              )}`}
                            >
                              {execution.status}
                            </span>
                            <span class="text-sm font-mono">{execution.id}</span>
                          </div>
                          <Show when={execution.finished}>
                            <span class="text-xs text-dark-muted">已完成</span>
                          </Show>
                        </div>
                        <div class="text-sm text-dark-muted mt-1">
                          开始: {formatDate(execution.started_at)}
                          <Show when={execution.stopped_at}>
                            {' | '}结束: {formatDate(execution.stopped_at!)}
                          </Show>
                        </div>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            </Show>
          </Show>
        </div>
      </div>

      {/* Info Card */}
      <div class="card mt-6 bg-primary-500/10 border-primary-500/30">
        <h3 class="font-semibold text-primary-400 mb-2">配置说明</h3>
        <ul class="text-sm text-dark-muted space-y-1">
          <li>• 需要在设置中配置 <code class="px-1 bg-dark-bg rounded">n8n.api_key</code> 才能查看工作流详情</li>
          <li>• 触发工作流使用 Webhook 方式，需要工作流中配置对应的 Webhook 节点</li>
          <li>• 工作流名称对应 Webhook 路径，如 <code class="px-1 bg-dark-bg rounded">manual-ingest</code></li>
        </ul>
      </div>

      {/* Trigger Modal */}
      <Show when={showTriggerModal()}>
        <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div class="card w-full max-w-lg">
            <h3 class="text-lg font-semibold mb-4">触发工作流</h3>

            <div class="space-y-4">
              <div>
                <label class="block text-sm font-medium mb-1">工作流名称 (Webhook 路径)</label>
                <input
                  type="text"
                  class="input w-full"
                  value={triggerName()}
                  onInput={(e) => setTriggerName(e.currentTarget.value)}
                  placeholder="例如: manual-ingest"
                />
              </div>

              <div>
                <label class="block text-sm font-medium mb-1">Payload (JSON)</label>
                <textarea
                  class="input w-full font-mono text-sm"
                  rows={6}
                  value={triggerPayload()}
                  onInput={(e) => setTriggerPayload(e.currentTarget.value)}
                  placeholder='{"key": "value"}'
                />
              </div>

              <Show when={triggerResult()}>
                <div class="p-3 bg-dark-bg rounded-lg">
                  <div class="text-sm font-medium mb-1">结果</div>
                  <pre class="text-sm font-mono overflow-auto max-h-32 text-dark-muted">
                    {triggerResult()}
                  </pre>
                </div>
              </Show>
            </div>

            <div class="flex justify-end gap-3 mt-6">
              <button
                class="btn"
                onClick={() => setShowTriggerModal(false)}
              >
                关闭
              </button>
              <button
                class="btn btn-primary"
                onClick={handleTrigger}
              >
                触发
              </button>
            </div>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default Workflows
