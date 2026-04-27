import { Component, For, Show, createResource } from 'solid-js'
import { logCenterApi } from '@/api'
import { formatDateTime, formatDuration, parseTaskStatusBadge, parseResultBadge } from './shared'
import type { ParseTask } from './shared'

const LogParseTasks: Component = () => {
  const [parseTasks, { refetch }] = createResource(
    () => true,
    () => logCenterApi.listParseTasks().then(r => r.data || [])
  )

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">解析任务</h1>
          <p class="text-sm text-dark-400 mt-1">查看 RAGFlow 文档解析任务状态</p>
        </div>
        <button class="btn btn-secondary btn-sm" onClick={() => refetch()}>刷新</button>
      </div>

      <div class="space-y-4">
        <div class="flex items-center gap-3">
          <span class="text-xs text-dark-500">显示内存中的解析任务（重启后清空）</span>
        </div>

        <Show when={!parseTasks.loading} fallback={<div class="text-center text-dark-400 py-8">加载中...</div>}>
          <For each={parseTasks() || []} fallback={<div class="text-center text-dark-500 py-8">暂无解析任务</div>}>
            {(task: ParseTask) => (
              <div class="card p-4 space-y-3">
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <span class="font-mono text-sm text-dark-200">{task.id}</span>
                    <span class={`badge badge-sm ${parseTaskStatusBadge(task.status)}`}>
                      {task.status === 'running' ? '运行中' : task.status === 'recovering' ? '恢复中' : '已完成'}
                    </span>
                    <Show when={task.result_status}>
                      <span class={`badge badge-sm ${parseResultBadge(task.result_status)}`}>
                        {task.result_status === 'success' ? '全部成功' : task.result_status === 'partial_failed' ? '部分失败' : task.result_status === 'failed' ? '全部失败' : task.result_status}
                      </span>
                    </Show>
                  </div>
                  <span class="text-xs text-dark-500">{formatDateTime(task.started_at)}</span>
                </div>

                <div>
                  <div class="flex justify-between text-xs text-dark-400 mb-1">
                    <span>进度: {task.completed + task.failed} / {task.total}</span>
                    <span>成功 {task.completed} | 失败 {task.failed} | 待处理 {task.pending}</span>
                  </div>
                  <div class="w-full bg-dark-700 rounded-full h-2.5">
                    <div class="flex h-2.5 rounded-full overflow-hidden">
                      <div
                        class="bg-emerald-500 transition-all duration-300"
                        style={{ width: `${task.total > 0 ? (task.completed / task.total) * 100 : 0}%` }}
                      />
                      <div
                        class="bg-red-500 transition-all duration-300"
                        style={{ width: `${task.total > 0 ? (task.failed / task.total) * 100 : 0}%` }}
                      />
                    </div>
                  </div>
                </div>

                <Show when={task.current_stage && task.status !== 'completed'}>
                  <div class="flex items-center gap-2 text-xs text-dark-400">
                    <div class="status-dot status-dot-primary animate-pulse" />
                    <span>阶段: {task.current_stage}</span>
                    <Show when={task.current_dataset_id}>
                      <span>| 当前 dataset: {task.current_dataset_id}</span>
                    </Show>
                    <Show when={task.current_batch_index}>
                      <span>| 批次 #{task.current_batch_index}</span>
                    </Show>
                  </div>
                </Show>

                <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
                  <div class="bg-dark-800 rounded-lg p-2 text-center">
                    <div class="text-dark-500">运行中</div>
                    <div class="text-lg font-semibold text-sky-400">{task.running_count}</div>
                  </div>
                  <div class="bg-dark-800 rounded-lg p-2 text-center">
                    <div class="text-dark-500">恢复中</div>
                    <div class="text-lg font-semibold text-amber-400">{task.recovering_count}</div>
                  </div>
                  <div class="bg-dark-800 rounded-lg p-2 text-center">
                    <div class="text-dark-500">已完成</div>
                    <div class="text-lg font-semibold text-emerald-400">{task.succeeded_count}</div>
                  </div>
                  <div class="bg-dark-800 rounded-lg p-2 text-center">
                    <div class="text-dark-500">最终失败</div>
                    <div class="text-lg font-semibold text-red-400">{task.final_failed_count}</div>
                  </div>
                </div>

                <Show when={task.failed_docs && task.failed_docs.length > 0}>
                  <details class="text-xs">
                    <summary class="text-red-400 cursor-pointer hover:text-red-300">
                      失败文档 ({task.failed_docs!.length})
                    </summary>
                    <div class="mt-2 space-y-1 max-h-40 overflow-y-auto">
                      <For each={task.failed_docs!}>
                        {(doc) => (
                          <div class="bg-dark-800 rounded p-2 font-mono text-dark-400">
                            <span class="text-dark-500">{doc.document_id.slice(0, 12)}...</span>
                            <span class="text-red-400 ml-2">{doc.error}</span>
                            <span class="text-dark-600 ml-2">({doc.retries} retries)</span>
                          </div>
                        )}
                      </For>
                    </div>
                  </details>
                </Show>

                <Show when={task.log && task.log.length > 0}>
                  <details class="text-xs">
                    <summary class="text-dark-400 cursor-pointer hover:text-dark-300">
                      任务日志 ({task.log!.length} 条)
                    </summary>
                    <div class="mt-2 bg-dark-900 rounded-lg p-3 max-h-60 overflow-y-auto font-mono text-dark-400 space-y-0.5">
                      <For each={task.log!}>
                        {(line) => <div class="whitespace-pre-wrap break-all">{line}</div>}
                      </For>
                    </div>
                  </details>
                </Show>

                <Show when={task.completed_at}>
                  <div class="text-xs text-dark-500">
                    完成于 {formatDateTime(task.completed_at)} |
                    耗时 {formatDuration(new Date(task.completed_at!).getTime() - new Date(task.started_at).getTime())}
                  </div>
                </Show>
              </div>
            )}
          </For>
        </Show>
      </div>
    </div>
  )
}

export default LogParseTasks
