import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { workflowsApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { Workflow, WorkflowDefinition, WorkflowExecution } from '@/types'

const Workflows: Component = () => {
  const toast = useToast()
  const [selectedWorkflow, setSelectedWorkflow] = createSignal<string | null>(null)
  const [triggerName, setTriggerName] = createSignal('')
  const [triggerPayload, setTriggerPayload] = createSignal('{}')
  const [showTriggerModal, setShowTriggerModal] = createSignal(false)
  const [triggerResult, setTriggerResult] = createSignal<string | null>(null)
  const [triggering, setTriggering] = createSignal(false)
  const [pushingDefinition, setPushingDefinition] = createSignal<string | null>(null)
  const [pushAllRunning, setPushAllRunning] = createSignal(false)
  const [showDefinitionModal, setShowDefinitionModal] = createSignal(false)
  const [definitionJSON, setDefinitionJSON] = createSignal<string | null>(null)
  const [definitionModalTitle, setDefinitionModalTitle] = createSignal('')

  const [definitions, { refetch: refetchDefinitions }] = createResource(
    () => workflowsApi.definitions()
  )

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
        toast.success(`工作流"${workflow.name}"已停用`)
      } else {
        await workflowsApi.activate(workflow.id)
        toast.success(`工作流"${workflow.name}"已激活`)
      }
      refetchWorkflows()
    } catch (err) {
      toast.error('操作失败: ' + (err as Error).message)
    }
  }

  const handlePushDefinition = async (definition: WorkflowDefinition) => {
    setPushingDefinition(definition.key)
    try {
      const result = await workflowsApi.pushDefinition(definition.key)
      toast.success(`工作流"${definition.name}"已${result.data.action === 'created' ? '创建' : '更新'}到 n8n`)
      refetchDefinitions()
      refetchWorkflows()
    } catch (err) {
      toast.error('推送失败: ' + (err as Error).message)
    } finally {
      setPushingDefinition(null)
    }
  }

  const handlePushAllDefinitions = async () => {
    setPushAllRunning(true)
    try {
      const result = await workflowsApi.pushAllDefinitions()
      const failed = result.data.filter((item) => item.error).length
      toast.success(`批量推送完成，成功 ${result.data.length - failed}，失败 ${failed}`)
      refetchDefinitions()
      refetchWorkflows()
    } catch (err) {
      toast.error('批量推送失败: ' + (err as Error).message)
    } finally {
      setPushAllRunning(false)
    }
  }

  const openDefinitionModal = async (definition: WorkflowDefinition) => {
    setDefinitionModalTitle(definition.name)
    setDefinitionJSON(null)
    setShowDefinitionModal(true)
    try {
      const detail = await workflowsApi.definition(definition.key)
      setDefinitionJSON(detail.data.raw_json)
    } catch (err) {
      setDefinitionJSON('加载失败: ' + (err as Error).message)
    }
  }

  const handleTrigger = async () => {
    if (!triggerName()) {
      toast.error('请输入工作流名称')
      return
    }

    setTriggering(true)
    try {
      let payload = {}
      try {
        payload = JSON.parse(triggerPayload())
      } catch {
        toast.error('无效的 JSON 格式')
        setTriggering(false)
        return
      }

      const result = await workflowsApi.trigger(triggerName(), payload)
      setTriggerResult(JSON.stringify(result.data, null, 2))
      toast.success('工作流触发成功')
      refetchExecutions()
    } catch (err) {
      setTriggerResult('触发失败: ' + (err as Error).message)
      toast.error('触发失败: ' + (err as Error).message)
    } finally {
      setTriggering(false)
    }
  }

  const openTriggerModal = (name: string = '') => {
    setTriggerName(name)
    setTriggerPayload('{}')
    setTriggerResult(null)
    setShowTriggerModal(true)
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'success':
        return 'badge-success'
      case 'error':
        return 'badge-danger'
      case 'waiting':
        return 'badge-warning'
      default:
        return 'badge-gray'
    }
  }

  const getDriftBadge = (status: string) => {
    switch (status) {
      case 'present':
        return 'badge-success'
      case 'missing_remote':
        return 'badge-warning'
      case 'invalid':
        return 'badge-danger'
      case 'runtime_unknown':
        return 'badge-gray'
      default:
        return 'badge-gray'
    }
  }

  const getDriftText = (status: string) => {
    switch (status) {
      case 'present':
        return '已匹配'
      case 'missing_remote':
        return '远端缺失'
      case 'invalid':
        return '无效'
      case 'runtime_unknown':
        return '运行态未知'
      default:
        return '未比对'
    }
  }

  const formatDate = (dateStr: string) => {
    if (!dateStr) return '--'
    return new Date(dateStr).toLocaleString('zh-CN')
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">工作流管理</h1>
          <p class="text-sm text-dark-400 mt-1">管理和监控 n8n 自动化工作流</p>
        </div>
        <button class="btn btn-primary" onClick={() => openTriggerModal('')}>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          手动触发
        </button>
      </div>

      {/* Repository Definitions */}
      <div class="card mb-6">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
          <div>
            <h2 class="text-lg font-semibold text-white">仓库工作流定义</h2>
            <Show when={definitions()?.data.workflow_dir}>
              <div class="text-xs text-dark-500 font-mono mt-1 break-all">{definitions()?.data.workflow_dir}</div>
            </Show>
          </div>
          <div class="flex items-center gap-2">
            <button class="btn btn-ghost btn-sm" onClick={() => refetchDefinitions()}>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </button>
            <button
              class="btn btn-secondary btn-sm"
              disabled={pushAllRunning() || !definitions()?.data.definitions?.length}
              onClick={handlePushAllDefinitions}
            >
              {pushAllRunning() ? (
                <>
                  <div class="loading-spinner" />
                  推送中...
                </>
              ) : '批量推送'}
            </button>
          </div>
        </div>

        <Show
          when={!definitions.loading}
          fallback={
            <div class="flex items-center justify-center py-8">
              <div class="loading-spinner" />
              <span class="ml-3 text-dark-400">加载中...</span>
            </div>
          }
        >
          <Show
            when={!definitions.error}
            fallback={
              <div class="empty-state py-8">
                <svg class="empty-state-icon w-12 h-12 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <p class="empty-state-title">加载失败</p>
                <p class="empty-state-description">{(definitions.error as Error)?.message}</p>
              </div>
            }
          >
            <Show when={definitions()?.data.local_error}>
              <div class="mb-3 p-3 rounded-lg border border-red-500/30 bg-red-500/10 text-sm text-red-300">
                {definitions()?.data.local_error}
              </div>
            </Show>
            <Show when={definitions()?.data.runtime_error}>
              <div class="mb-3 p-3 rounded-lg border border-amber-500/30 bg-amber-500/10 text-sm text-amber-300">
                {definitions()?.data.runtime_error}
              </div>
            </Show>

            <Show
              when={definitions()?.data.definitions && definitions()!.data.definitions.length > 0}
              fallback={
                <div class="empty-state py-8">
                  <svg class="empty-state-icon w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                  </svg>
                  <p class="empty-state-title">暂无仓库定义</p>
                </div>
              }
            >
              <div class="grid grid-cols-1 xl:grid-cols-2 gap-3">
                <For each={definitions()?.data.definitions}>
                  {(definition) => (
                    <div class="p-4 bg-dark-700/50 rounded-xl border border-dark-600/50">
                      <div class="flex items-start justify-between gap-3">
                        <div class="min-w-0">
                          <div class="font-medium text-white truncate">{definition.name}</div>
                          <div class="text-xs text-dark-500 mt-1 font-mono truncate">{definition.file_name}</div>
                          <div class="flex flex-wrap gap-1 mt-2">
                            <span class={`badge ${getDriftBadge(definition.drift_status)}`}>
                              {getDriftText(definition.drift_status)}
                            </span>
                            <span class="badge badge-gray">{definition.node_count} nodes</span>
                            <Show when={definition.runtime}>
                              <span class={definition.runtime!.active ? 'badge badge-success' : 'badge badge-gray'}>
                                {definition.runtime!.active ? 'Active' : 'Inactive'}
                              </span>
                            </Show>
                          </div>
                          <Show when={definition.trigger_types && definition.trigger_types.length > 0}>
                            <div class="flex flex-wrap gap-1 mt-2">
                              <For each={definition.trigger_types}>
                                {(triggerType) => <span class="badge badge-gray">{triggerType}</span>}
                              </For>
                            </div>
                          </Show>
                          <Show when={definition.error}>
                            <div class="text-xs text-red-300 mt-2 break-words">{definition.error}</div>
                          </Show>
                        </div>
                        <div class="flex items-center gap-2 shrink-0">
                          <button class="btn btn-ghost btn-sm" onClick={() => openDefinitionModal(definition)}>
                            JSON
                          </button>
                          <button
                            class="btn btn-secondary btn-sm"
                            disabled={!definition.valid || pushingDefinition() === definition.key}
                            onClick={() => handlePushDefinition(definition)}
                          >
                            {pushingDefinition() === definition.key ? '推送中...' : '推送'}
                          </button>
                        </div>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </Show>

            <Show when={definitions()?.data.remote_only && definitions()!.data.remote_only!.length > 0}>
              <div class="mt-4 pt-4 border-t border-dark-700">
                <div class="text-sm font-medium text-dark-300 mb-2">仅存在于 n8n</div>
                <div class="flex flex-wrap gap-2">
                  <For each={definitions()?.data.remote_only}>
                    {(workflow) => (
                      <button
                        class="badge badge-warning"
                        onClick={() => {
                          setSelectedWorkflow(workflow.id)
                          refetchExecutions()
                        }}
                      >
                        {workflow.name}
                      </button>
                    )}
                  </For>
                </div>
              </div>
            </Show>
          </Show>
        </Show>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Workflow List */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">n8n 工作流</h2>
            <button class="btn btn-ghost btn-sm" onClick={() => refetchWorkflows()}>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </button>
          </div>
          <Show
            when={!workflows.loading}
            fallback={
              <div class="flex items-center justify-center py-8">
                <div class="loading-spinner" />
                <span class="ml-3 text-dark-400">加载中...</span>
              </div>
            }
          >
            <Show
              when={!workflows.error}
              fallback={
                <div class="empty-state py-8">
                  <svg class="empty-state-icon w-12 h-12 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <p class="empty-state-title">加载失败</p>
                  <p class="empty-state-description">{(workflows.error as Error)?.message || '请检查 n8n API Key 配置'}</p>
                  <button class="btn btn-secondary btn-sm mt-3" onClick={() => refetchWorkflows()}>重试</button>
                </div>
              }
            >
            <Show
              when={workflows()?.data && workflows()!.data.length > 0}
              fallback={
                <div class="empty-state py-8">
                  <svg class="empty-state-icon w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                  </svg>
                  <p class="empty-state-title">暂无工作流数据</p>
                  <p class="empty-state-description">请配置 n8n API Key 以获取工作流列表</p>
                </div>
              }
            >
              <div class="space-y-3">
                <For each={workflows()?.data}>
                  {(workflow) => (
                    <div
                      class="p-4 bg-dark-700/50 rounded-xl border border-dark-600/50 cursor-pointer transition-all hover:border-dark-600/50"
                      classList={{
                        'ring-2 ring-primary-500 border-primary-500/50': selectedWorkflow() === workflow.id,
                      }}
                      onClick={() => {
                        setSelectedWorkflow(workflow.id)
                        refetchExecutions()
                      }}
                    >
                      <div class="flex items-center justify-between">
                        <div class="flex-1 min-w-0">
                          <div class="font-medium text-white truncate">{workflow.name}</div>
                          <div class="text-xs text-dark-500 mt-1 font-mono">
                            ID: {workflow.id}
                          </div>
                          <Show when={workflow.tags && workflow.tags.length > 0}>
                            <div class="flex flex-wrap gap-1 mt-2">
                              <For each={workflow.tags}>
                                {(tag) => (
                                  <span class="badge badge-gray">{tag.name}</span>
                                )}
                              </For>
                            </div>
                          </Show>
                        </div>
                        <div class="flex items-center gap-2 ml-3">
                          <button
                            class="btn btn-ghost btn-sm"
                            onClick={(e) => {
                              e.stopPropagation()
                              openTriggerModal(workflow.name)
                            }}
                          >
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                            </svg>
                          </button>
                          <button
                            class={`px-3 py-1 rounded-lg text-sm font-medium transition-colors ${
                              workflow.active
                                ? 'bg-emerald-500/20 text-emerald-400 hover:bg-emerald-500/30'
                                : 'bg-dark-700 text-dark-400 hover:bg-dark-600'
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
          </Show>
        </div>

        {/* Execution History */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">执行历史</h2>
            <Show when={selectedWorkflow()}>
              <button class="btn btn-ghost btn-sm" onClick={() => refetchExecutions()}>
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
              </button>
            </Show>
          </div>
          <Show
            when={selectedWorkflow()}
            fallback={
              <div class="empty-state py-8">
                <svg class="empty-state-icon w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <p class="empty-state-title">选择一个工作流</p>
                <p class="empty-state-description">点击左侧工作流查看执行历史</p>
              </div>
            }
          >
            <Show
              when={!executions.loading}
              fallback={
                <div class="flex items-center justify-center py-8">
                  <div class="loading-spinner" />
                  <span class="ml-3 text-dark-400">加载中...</span>
                </div>
              }
            >
              <Show
                when={executions()?.data && executions()!.data.length > 0}
                fallback={
                  <div class="empty-state py-8">
                    <svg class="empty-state-icon w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                    <p class="empty-state-title">暂无执行记录</p>
                    <p class="empty-state-description">请配置 n8n API Key 以获取执行历史</p>
                  </div>
                }
              >
                <div class="space-y-2 max-h-96 overflow-y-auto">
                  <For each={executions()?.data}>
                    {(execution) => (
                      <div class="p-3 bg-dark-700/50 rounded-xl border border-dark-600/50">
                        <div class="flex items-center justify-between">
                          <div class="flex items-center gap-2">
                            <span class={`badge ${getStatusBadge(execution.status)}`}>
                              {execution.status}
                            </span>
                            <span class="text-sm font-mono text-dark-400">{execution.id.slice(0, 8)}...</span>
                          </div>
                          <Show when={execution.finished}>
                            <span class="text-xs text-dark-500">已完成</span>
                          </Show>
                        </div>
                        <div class="text-xs text-dark-500 mt-2">
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

      {/* Definition Modal */}
      <Modal
        open={showDefinitionModal()}
        onClose={() => setShowDefinitionModal(false)}
        title={definitionModalTitle()}
        footer={
          <button type="button" class="btn btn-secondary" onClick={() => setShowDefinitionModal(false)}>
            关闭
          </button>
        }
      >
        <div class="p-4 bg-dark-700/50 rounded-xl border border-dark-600/50">
          <pre class="text-sm font-mono text-dark-300 overflow-auto max-h-[60vh] whitespace-pre-wrap break-words">
            {definitionJSON() || '加载中...'}
          </pre>
        </div>
      </Modal>

      {/* Trigger Modal */}
      <Modal
        open={showTriggerModal()}
        onClose={() => setShowTriggerModal(false)}
        title="触发工作流"
        footer={
          <>
            <button type="button" class="btn btn-secondary" onClick={() => setShowTriggerModal(false)}>
              关闭
            </button>
            <button
              type="button"
              class="btn btn-primary"
              disabled={triggering()}
              onClick={handleTrigger}
            >
              {triggering() ? (
                <>
                  <div class="loading-spinner" />
                  触发中...
                </>
              ) : '触发'}
            </button>
          </>
        }
      >
        <div class="space-y-4">
          <div>
            <label class="label">工作流名称 (Webhook 路径) *</label>
            <input
              type="text"
              class="input font-mono"
              value={triggerName()}
              onInput={(e) => setTriggerName(e.currentTarget.value)}
              placeholder="例如: manual-ingest"
            />
          </div>

          <div>
            <label class="label">Payload (JSON)</label>
            <textarea
              class="input font-mono text-sm resize-none"
              rows={6}
              value={triggerPayload()}
              onInput={(e) => setTriggerPayload(e.currentTarget.value)}
              placeholder='{"key": "value"}'
            />
          </div>

          <Show when={triggerResult()}>
            <div class="p-4 bg-dark-700/50 rounded-xl border border-dark-600/50">
              <div class="text-sm font-medium text-dark-300 mb-2">结果</div>
              <pre class="text-sm font-mono text-dark-400 overflow-auto max-h-32">
                {triggerResult()}
              </pre>
            </div>
          </Show>
        </div>
      </Modal>
    </div>
  )
}

export default Workflows
