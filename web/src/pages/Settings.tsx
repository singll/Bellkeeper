import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { settingsApi } from '@/api'
import { useToast } from '@/components/Toast'
import type { Setting } from '@/types'

const Settings: Component = () => {
  const toast = useToast()
  const [category, setCategory] = createSignal('')
  const [editingKey, setEditingKey] = createSignal<string | null>(null)
  const [editValue, setEditValue] = createSignal('')
  const [saving, setSaving] = createSignal(false)

  const [settings, { refetch }] = createResource(
    () => category(),
    (cat) => settingsApi.list(cat)
  )

  const categories = [
    { value: '', label: '全部', icon: 'M4 6h16M4 10h16M4 14h16M4 18h16' },
    { value: 'api', label: 'API 配置', icon: 'M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4' },
    { value: 'feature', label: '功能开关', icon: 'M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4' },
    { value: 'ui', label: '界面设置', icon: 'M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z' },
  ]

  const startEdit = (setting: Setting) => {
    setEditingKey(setting.key)
    setEditValue(setting.is_secret ? '' : setting.value)
  }

  const cancelEdit = () => {
    setEditingKey(null)
    setEditValue('')
  }

  const saveEdit = async (setting: Setting) => {
    setSaving(true)
    try {
      await settingsApi.update(setting.key, {
        value: editValue(),
        value_type: setting.value_type,
        category: setting.category,
        description: setting.description,
        is_secret: setting.is_secret,
      })
      toast.success('设置已保存')
      cancelEdit()
      refetch()
    } catch (err) {
      toast.error('保存失败: ' + (err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">系统设置</h1>
        <p class="text-sm text-dark-400 mt-1">配置系统运行参数和功能开关</p>
      </div>

      {/* Category Tabs */}
      <div class="card mb-6 p-2">
        <div class="flex flex-wrap gap-2">
          <For each={categories}>
            {(cat) => (
              <button
                class="flex items-center gap-2 px-4 py-2 rounded-xl transition-all"
                classList={{
                  'bg-primary-600 text-white shadow-lg shadow-primary-500/25': category() === cat.value,
                  'text-dark-400 hover:text-white hover:bg-dark-700/50': category() !== cat.value,
                }}
                onClick={() => setCategory(cat.value)}
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={cat.icon} />
                </svg>
                {cat.label}
              </button>
            )}
          </For>
        </div>
      </div>

      {/* Settings List */}
      <div class="card">
        <Show
          when={!settings.loading}
          fallback={
            <div class="flex items-center justify-center py-12">
              <div class="loading-spinner" />
              <span class="ml-3 text-dark-400">加载中...</span>
            </div>
          }
        >
          <Show
            when={settings()?.data && settings()!.data.length > 0}
            fallback={
              <div class="empty-state py-12">
                <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                </svg>
                <p class="empty-state-title">暂无设置项</p>
              </div>
            }
          >
            <div class="divide-y divide-dark-700/50">
              <For each={settings()?.data}>
                {(setting) => (
                  <div class="py-4 first:pt-0 last:pb-0">
                    <div class="flex items-start justify-between gap-4">
                      <div class="flex-1 min-w-0">
                        <div class="flex items-center gap-2 flex-wrap">
                          <code class="font-mono text-sm text-white bg-dark-700/50 px-2 py-1 rounded">
                            {setting.key}
                          </code>
                          <span class="badge badge-gray">{setting.value_type}</span>
                          <Show when={setting.is_secret}>
                            <span class="badge badge-warning">敏感</span>
                          </Show>
                        </div>
                        <p class="text-sm text-dark-400 mt-2">{setting.description || '无描述'}</p>
                      </div>
                      <div class="flex-shrink-0">
                        <Show
                          when={editingKey() === setting.key}
                          fallback={
                            <div class="flex items-center gap-3">
                              <div class="text-right">
                                <code class="font-mono text-sm text-dark-300 bg-dark-700/50 px-3 py-1.5 rounded-lg block max-w-[200px] truncate">
                                  {setting.is_secret ? '••••••••' : setting.value || '(空)'}
                                </code>
                              </div>
                              <button
                                class="btn btn-ghost btn-sm"
                                onClick={() => startEdit(setting)}
                              >
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                                </svg>
                              </button>
                            </div>
                          }
                        >
                          <div class="flex items-center gap-2">
                            <Show
                              when={setting.value_type === 'bool'}
                              fallback={
                                <input
                                  type={setting.is_secret ? 'password' : 'text'}
                                  class="input w-48 font-mono text-sm"
                                  value={editValue()}
                                  onInput={(e) => setEditValue(e.currentTarget.value)}
                                  placeholder={setting.is_secret ? '输入新值' : ''}
                                />
                              }
                            >
                              <select
                                class="input w-32"
                                value={editValue()}
                                onChange={(e) => setEditValue(e.currentTarget.value)}
                              >
                                <option value="true">启用</option>
                                <option value="false">禁用</option>
                              </select>
                            </Show>
                            <button
                              class="btn btn-ghost btn-sm text-emerald-400 hover:text-emerald-300 hover:bg-emerald-500/10"
                              onClick={() => saveEdit(setting)}
                              disabled={saving()}
                            >
                              {saving() ? (
                                <div class="loading-spinner w-4 h-4" />
                              ) : (
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                                </svg>
                              )}
                            </button>
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={cancelEdit}
                            >
                              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                              </svg>
                            </button>
                          </div>
                        </Show>
                      </div>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </Show>
      </div>

      {/* Info Card */}
      <div class="card mt-6 bg-primary-500/10 border-primary-500/30">
        <div class="flex items-start gap-3">
          <svg class="w-5 h-5 text-primary-400 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <div class="text-sm">
            <p class="text-primary-300 font-medium">配置说明</p>
            <ul class="text-dark-400 mt-1 space-y-0.5">
              <li>配置优先级: 数据库 &gt; 环境变量 &gt; 配置文件 &gt; 默认值</li>
              <li>敏感配置 (如 API Key) 修改后不会显示原值</li>
              <li>部分配置修改后可能需要重启服务才能生效</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  )
}

export default Settings
