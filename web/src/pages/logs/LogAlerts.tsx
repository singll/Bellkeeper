import { Component, For, Show, createResource, createSignal } from 'solid-js'
import { logCenterApi } from '@/api'
import { moduleLabel, levelLabel } from './shared'
import type { LogAlertRule } from './shared'

const LogAlerts: Component = () => {
  const [alerts, { refetch }] = createResource(
    () => true,
    () => logCenterApi.listAlerts()
  )
  const [showCreate, setShowCreate] = createSignal(false)
  const [editingId, setEditingId] = createSignal<number | null>(null)
  const [alertResult, setAlertResult] = createSignal<string | null>(null)

  const alertForm = { name: '', module: 'rss_fetch', level: 'error', threshold: 5, window_minutes: 30, notify_channel: 'daily' }
  const [form, setForm] = createSignal({ ...alertForm })

  const handleCreate = async () => {
    const f = form()
    if (!f.name) return
    await logCenterApi.createAlert({
      name: f.name,
      condition: { module: f.module, level: f.level, threshold: f.threshold, window_minutes: f.window_minutes },
      notify_channel: f.notify_channel,
    })
    setShowCreate(false)
    setForm(alertForm)
    refetch()
  }

  const toggleActive = async (rule: LogAlertRule) => {
    await logCenterApi.updateAlert(rule.id, { is_active: !rule.is_active })
    refetch()
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确认删除该告警规则？')) return
    await logCenterApi.deleteAlert(id)
    refetch()
  }

  const conditionLabel = (cond: LogAlertRule['condition']) => (
    <span class="text-xs text-dark-300">
      {moduleLabel(cond.module)} / {levelLabel(cond.level)} / {cond.threshold}次 / {cond.window_minutes}分钟
    </span>
  )

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">告警规则</h1>
          <p class="text-sm text-dark-400 mt-1">设置日志告警规则，当满足条件时触发通知</p>
        </div>
        <button class="btn btn-primary btn-sm" onClick={() => setShowCreate(true)}>
          创建规则
        </button>
      </div>

      <div class="space-y-4">
        <Show when={showCreate()}>
          <div class="card p-4 space-y-3">
            <h3 class="font-semibold text-dark-200">创建告警规则</h3>
            <div class="grid grid-cols-2 gap-3">
              <input class="input" placeholder="规则名称" value={form().name}
                onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })} />
              <select class="input" value={form().module}
                onChange={(e) => setForm({ ...form(), module: e.currentTarget.value })}>
                <option value="rss_fetch">RSS采集</option>
                <option value="ragflow_parse">智能解析</option>
                <option value="classify">文章分类</option>
                <option value="llm_proxy">LLM Proxy</option>
                <option value="file_ingest">文件入库</option>
                <option value="crawler">爬虫</option>
                <option value="matrix_notify">Matrix通知</option>
              </select>
              <select class="input" value={form().level}
                onChange={(e) => setForm({ ...form(), level: e.currentTarget.value })}>
                <option value="error">错误</option>
                <option value="warn">警告</option>
              </select>
              <div class="flex gap-2">
                <input class="input flex-1" type="number" placeholder="阈值" value={form().threshold}
                  onInput={(e) => setForm({ ...form(), threshold: parseInt(e.currentTarget.value) || 0 })} />
                <input class="input flex-1" type="number" placeholder="窗口(分)" value={form().window_minutes}
                  onInput={(e) => setForm({ ...form(), window_minutes: parseInt(e.currentTarget.value) || 0 })} />
              </div>
              <input class="input" placeholder="通知渠道 (如 daily)" value={form().notify_channel}
                onInput={(e) => setForm({ ...form(), notify_channel: e.currentTarget.value })} />
            </div>
            <div class="flex gap-2">
              <button class="btn btn-primary btn-sm" onClick={handleCreate}>创建</button>
              <button class="btn btn-secondary btn-sm" onClick={() => { setShowCreate(false); setForm(alertForm) }}>取消</button>
            </div>
          </div>
        </Show>

        <Show when={alertResult()}>
          <div class="card p-3 bg-sky-500/10 border border-sky-500/30">
            <pre class="text-xs text-sky-300 font-mono whitespace-pre-wrap">{alertResult()}</pre>
          </div>
        </Show>

        <Show when={!alerts.loading} fallback={<div class="text-center text-dark-400 py-8">加载中...</div>}>
          <div class="space-y-2">
            <For each={alerts()?.data || []} fallback={<div class="text-center text-dark-500 py-8">暂无告警规则</div>}>
              {(rule) => (
                <div class="card p-4 flex items-center gap-4">
                  {/* Toggle */}
                  <button
                    class={`w-10 h-5 rounded-full relative transition-colors ${rule.is_active ? 'bg-emerald-500' : 'bg-dark-600'}`}
                    onClick={() => toggleActive(rule)}
                  >
                    <div class={`w-4 h-4 rounded-full bg-white absolute top-0.5 transition-all ${rule.is_active ? 'left-5' : 'left-0.5'}`} />
                  </button>

                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <span class="font-medium text-dark-200">{rule.name}</span>
                      {!rule.is_active && <span class="text-xs text-dark-500">(已停用)</span>}
                    </div>
                    <div class="mt-1">{conditionLabel(rule.condition)}</div>
                    <div class="text-xs text-dark-500 mt-0.5">通知: {rule.notify_channel}</div>
                  </div>

                  <div class="flex gap-1">
                    <button class="text-red-400 hover:text-red-300 text-xs" onClick={() => handleDelete(rule.id)}>删除</button>
                  </div>
                </div>
              )}
            </For>
          </div>
        </Show>
      </div>
    </div>
  )
}

export default LogAlerts
