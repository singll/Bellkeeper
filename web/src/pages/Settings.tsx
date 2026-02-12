import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { settingsApi } from '@/api'
import type { Setting } from '@/types'

const Settings: Component = () => {
  const [category, setCategory] = createSignal('')
  const [editingKey, setEditingKey] = createSignal<string | null>(null)
  const [editValue, setEditValue] = createSignal('')

  const [settings, { refetch }] = createResource(
    () => category(),
    (cat) => settingsApi.list(cat)
  )

  const categories = [
    { value: '', label: '全部' },
    { value: 'api', label: 'API 配置' },
    { value: 'feature', label: '功能开关' },
    { value: 'ui', label: '界面设置' },
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
    try {
      await settingsApi.update(setting.key, {
        value: editValue(),
        value_type: setting.value_type,
        category: setting.category,
        description: setting.description,
        is_secret: setting.is_secret,
      })
      cancelEdit()
      refetch()
    } catch (err) {
      alert('保存失败: ' + (err as Error).message)
    }
  }

  return (
    <div>
      <h1 class="text-2xl font-bold mb-6">系统设置</h1>

      {/* Category Tabs */}
      <div class="flex gap-2 mb-6">
        <For each={categories}>
          {(cat) => (
            <button
              class="px-4 py-2 rounded-lg transition-colors"
              classList={{
                'bg-primary-600 text-white': category() === cat.value,
                'bg-dark-card text-dark-muted hover:text-dark-text': category() !== cat.value,
              }}
              onClick={() => setCategory(cat.value)}
            >
              {cat.label}
            </button>
          )}
        </For>
      </div>

      {/* Settings List */}
      <div class="card">
        <Show
          when={!settings.loading}
          fallback={<div class="text-dark-muted py-8 text-center">加载中...</div>}
        >
          <div class="divide-y divide-dark-border">
            <For each={settings()?.data}>
              {(setting) => (
                <div class="py-4 first:pt-0 last:pb-0">
                  <div class="flex items-start justify-between">
                    <div class="flex-1">
                      <div class="flex items-center gap-2">
                        <span class="font-medium font-mono text-sm">{setting.key}</span>
                        <span class="px-2 py-0.5 bg-dark-bg rounded text-xs text-dark-muted">
                          {setting.value_type}
                        </span>
                        {setting.is_secret && (
                          <span class="px-2 py-0.5 bg-yellow-500/20 text-yellow-400 rounded text-xs">
                            敏感
                          </span>
                        )}
                      </div>
                      <p class="text-sm text-dark-muted mt-1">{setting.description}</p>
                    </div>
                    <div class="ml-4">
                      <Show
                        when={editingKey() === setting.key}
                        fallback={
                          <div class="flex items-center gap-2">
                            <span class="text-sm font-mono bg-dark-bg px-3 py-1 rounded">
                              {setting.is_secret ? '******' : setting.value}
                            </span>
                            <button
                              class="text-primary-500 hover:text-primary-400 text-sm"
                              onClick={() => startEdit(setting)}
                            >
                              修改
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
                                class="input w-48"
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
                            class="text-green-500 hover:text-green-400 text-sm"
                            onClick={() => saveEdit(setting)}
                          >
                            保存
                          </button>
                          <button
                            class="text-dark-muted hover:text-dark-text text-sm"
                            onClick={cancelEdit}
                          >
                            取消
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
      </div>

      {/* Info Card */}
      <div class="card mt-6 bg-primary-500/10 border-primary-500/30">
        <h3 class="font-semibold text-primary-400 mb-2">配置说明</h3>
        <ul class="text-sm text-dark-muted space-y-1">
          <li>• 配置优先级: 数据库 {'>'} 环境变量 {'>'} 配置文件 {'>'} 默认值</li>
          <li>• 敏感配置 (如 API Key) 修改后不会显示原值</li>
          <li>• 部分配置修改后可能需要重启服务才能生效</li>
        </ul>
      </div>
    </div>
  )
}

export default Settings
