import { Component, For, Show, createSignal } from 'solid-js'
import type { LLMChannelConfig, LLMModelGroupConfig, LLMModelGroupMemberConfig } from '@/types'
import { useToast } from '@/components/Toast'
import { llmProxyApi } from '@/api'
import Modal from '@/components/Modal'

interface LLMProxyConfigProps {
  channelConfigs: LLMChannelConfig[] | undefined
  groupConfigs: LLMModelGroupConfig[] | undefined
  onRefreshConfigs: () => Promise<void>
  onRefreshAll: () => Promise<void>
}

const LLMProxyConfig: Component<LLMProxyConfigProps> = (props) => {
  const toast = useToast()

  // Modal state
  const [showChannelModal, setShowChannelModal] = createSignal(false)
  const [showGroupModal, setShowGroupModal] = createSignal(false)
  const [editingChannel, setEditingChannel] = createSignal<LLMChannelConfig | null>(null)
  const [editingGroup, setEditingGroup] = createSignal<LLMModelGroupConfig | null>(null)
  const [saving, setSaving] = createSignal(false)

  // Channel form
  const [chForm, setChForm] = createSignal({
    name: '',
    base_url: '',
    api_key_env: '',
    provider_type: 'openai' as string,
    rpm: 500,
    rpd: 50000,
    priority: 1,
    is_free: false,
    is_enabled: true,
    models: '',
  })

  // Group form
  const [grpForm, setGrpForm] = createSignal({
    name: '',
    description: '',
    strategy: 'priority-health',
    sticky_ttl_seconds: 600,
    members: [] as LLMModelGroupMemberConfig[],
  })

  // Channel CRUD
  const openChannelModal = (ch?: LLMChannelConfig) => {
    if (ch) {
      setEditingChannel(ch)
      setChForm({
        name: ch.name,
        base_url: ch.base_url,
        api_key_env: ch.api_key_env,
        provider_type: ch.provider_type || 'openai',
        rpm: ch.rpm,
        rpd: ch.rpd,
        priority: ch.priority,
        is_free: ch.is_free,
        is_enabled: ch.is_enabled,
        models: ch.models,
      })
    } else {
      setEditingChannel(null)
      setChForm({
        name: '',
        base_url: '',
        api_key_env: '',
        provider_type: 'openai',
        rpm: 500,
        rpd: 50000,
        priority: 1,
        is_free: false,
        is_enabled: true,
        models: '[]',
      })
    }
    setShowChannelModal(true)
  }

  const saveChannel = async () => {
    setSaving(true)
    try {
      const form = chForm()
      const data = { ...form }
      const editing = editingChannel()
      if (editing) {
        await llmProxyApi.updateChannel(editing.id, data)
        toast.success('渠道已更新')
      } else {
        await llmProxyApi.createChannel(data)
        toast.success('渠道已创建')
      }
      setShowChannelModal(false)
      await Promise.all([props.onRefreshConfigs(), props.onRefreshAll()])
    } catch (err) {
      toast.error('保存失败: ' + (err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const deleteChannel = async (id: number) => {
    if (!confirm('确定删除此渠道？运行态将在重载后更新。')) return
    try {
      await llmProxyApi.deleteChannel(id)
      toast.success('渠道已删除')
      await Promise.all([props.onRefreshConfigs(), props.onRefreshAll()])
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    }
  }

  // Group CRUD
  const openGroupModal = (g?: LLMModelGroupConfig) => {
    if (g) {
      setEditingGroup(g)
      setGrpForm({
        name: g.name,
        description: g.description,
        strategy: g.strategy,
        sticky_ttl_seconds: g.sticky_ttl_seconds,
        members: g.members.map((m) => ({ channel_name: m.channel_name, model: m.model, weight: m.weight })),
      })
    } else {
      setEditingGroup(null)
      setGrpForm({
        name: '',
        description: '',
        strategy: 'priority-health',
        sticky_ttl_seconds: 600,
        members: [],
      })
    }
    setShowGroupModal(true)
  }

  const saveGroup = async () => {
    setSaving(true)
    try {
      const form = grpForm()
      const data = {
        name: form.name,
        description: form.description,
        strategy: form.strategy,
        sticky_ttl_seconds: form.sticky_ttl_seconds,
        members: form.members,
      }
      const editing = editingGroup()
      if (editing) {
        await llmProxyApi.updateGroup(editing.id, data)
        toast.success('模型组已更新')
      } else {
        await llmProxyApi.createGroup(data)
        toast.success('模型组已创建')
      }
      setShowGroupModal(false)
      await Promise.all([props.onRefreshConfigs(), props.onRefreshAll()])
    } catch (err) {
      toast.error('保存失败: ' + (err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const deleteGroup = async (id: number) => {
    if (!confirm('确定删除此模型组？')) return
    try {
      await llmProxyApi.deleteGroup(id)
      toast.success('模型组已删除')
      await Promise.all([props.onRefreshConfigs(), props.onRefreshAll()])
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    }
  }

  const addGroupMember = () => {
    setGrpForm((prev) => ({ ...prev, members: [...prev.members, { channel_name: '', model: '', weight: 1 }] }))
  }

  const removeGroupMember = (index: number) => {
    setGrpForm((prev) => ({ ...prev, members: prev.members.filter((_, i) => i !== index) }))
  }

  const updateGroupMember = (index: number, field: keyof LLMModelGroupMemberConfig, value: string | number) => {
    setGrpForm((prev) => ({
      ...prev,
      members: prev.members.map((m, i) => (i === index ? { ...m, [field]: value } : m)),
    }))
  }

  const baseUrlPlaceholder = () => {
    const pt = chForm().provider_type
    if (pt === 'anthropic') return '如 https://api.anthropic.com (不含 /v1)'
    return '如 https://api.siliconflow.cn (不含 /v1)'
  }

  return (
    <div class="space-y-6">
      {/* Channel Config */}
      <div class="card">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-lg font-semibold text-white">渠道管理</h2>
          <button class="btn btn-primary btn-sm" onClick={() => openChannelModal()}>
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
            </svg>
            新增渠道
          </button>
        </div>
        <Show
          when={props.channelConfigs && props.channelConfigs.length > 0}
          fallback={
            <div class="empty-state py-8">
              <p class="empty-state-title">暂无渠道配置</p>
              <p class="empty-state-description">点击「新增渠道」添加第一个 LLM 渠道</p>
            </div>
          }
        >
          <div class="overflow-x-auto rounded-xl border border-dark-600/50">
            <table class="table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>Base URL</th>
                  <th>协议</th>
                  <th>API Key 变量</th>
                  <th>RPM / RPD</th>
                  <th>优先级</th>
                  <th>类型</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <For each={props.channelConfigs}>
                  {(ch) => (
                    <tr>
                      <td class="font-medium text-white">{ch.name}</td>
                      <td class="text-xs text-dark-300 max-w-[200px] truncate">{ch.base_url}</td>
                      <td>
                        <span class={`badge ${ch.provider_type === 'anthropic' ? 'badge-warning' : 'badge-gray'}`}>
                          {ch.provider_type === 'anthropic' ? 'Anthropic' : 'OpenAI'}
                        </span>
                      </td>
                      <td class="font-mono text-xs text-dark-300">{ch.api_key_env || '--'}</td>
                      <td>{ch.rpm} / {ch.rpd}</td>
                      <td>{ch.priority}</td>
                      <td>
                        <span class={`badge ${ch.is_free ? 'badge-primary' : 'badge-gray'}`}>
                          {ch.is_free ? '免费' : '付费'}
                        </span>
                      </td>
                      <td>
                        <span class={`badge ${ch.is_enabled ? 'badge-success' : 'badge-danger'}`}>
                          {ch.is_enabled ? '启用' : '禁用'}
                        </span>
                      </td>
                      <td>
                        <div class="flex items-center gap-2">
                          <button class="btn btn-ghost btn-sm" onClick={() => openChannelModal(ch)}>编辑</button>
                          <button class="btn btn-ghost btn-sm text-red-400" onClick={() => deleteChannel(ch.id)}>删除</button>
                        </div>
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Show>
      </div>

      {/* Group Config */}
      <div class="card">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-lg font-semibold text-white">模型组管理</h2>
          <button class="btn btn-primary btn-sm" onClick={() => openGroupModal()}>
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
            </svg>
            新增模型组
          </button>
        </div>
        <Show
          when={props.groupConfigs && props.groupConfigs.length > 0}
          fallback={
            <div class="empty-state py-8">
              <p class="empty-state-title">暂无模型组配置</p>
              <p class="empty-state-description">点击「新增模型组」创建虚拟模型池</p>
            </div>
          }
        >
          <div class="overflow-x-auto rounded-xl border border-dark-600/50">
            <table class="table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>描述</th>
                  <th>策略</th>
                  <th>Sticky TTL</th>
                  <th>成员数</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <For each={props.groupConfigs}>
                  {(g) => (
                    <tr>
                      <td class="font-medium text-white">{g.name}</td>
                      <td class="text-sm text-dark-300 max-w-[200px] truncate">{g.description || '--'}</td>
                      <td><span class="badge badge-primary">{g.strategy}</span></td>
                      <td>{g.sticky_ttl_seconds}s</td>
                      <td>{g.members?.length || 0}</td>
                      <td>
                        <div class="flex items-center gap-2">
                          <button class="btn btn-ghost btn-sm" onClick={() => openGroupModal(g)}>编辑</button>
                          <button class="btn btn-ghost btn-sm text-red-400" onClick={() => deleteGroup(g.id)}>删除</button>
                        </div>
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Show>
      </div>

      {/* Channel Modal */}
      <Modal open={showChannelModal()} onClose={() => setShowChannelModal(false)} title={editingChannel() ? '编辑渠道' : '新增渠道'} size="lg">
        <div class="space-y-4">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">名称</label>
              <input class="input" value={chForm().name} onInput={(e) => setChForm((p) => ({ ...p, name: e.currentTarget.value }))} placeholder="如 siliconflow" />
            </div>
            <div>
              <label class="label">供应商类型</label>
              <select class="input" value={chForm().provider_type} onChange={(e) => setChForm((p) => ({ ...p, provider_type: e.currentTarget.value }))}>
                <option value="openai">OpenAI 兼容</option>
                <option value="anthropic">Anthropic (Claude)</option>
              </select>
            </div>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">Base URL</label>
              <input class="input" value={chForm().base_url} onInput={(e) => setChForm((p) => ({ ...p, base_url: e.currentTarget.value }))} placeholder={baseUrlPlaceholder()} />
            </div>
            <div>
              <label class="label">API Key 环境变量</label>
              <input class="input" value={chForm().api_key_env} onInput={(e) => setChForm((p) => ({ ...p, api_key_env: e.currentTarget.value }))} placeholder={chForm().provider_type === 'anthropic' ? '如 LLM_ANTHROPIC_API_KEY' : '如 LLM_SILICONFLOW_API_KEY'} />
            </div>
          </div>
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <div>
              <label class="label">RPM</label>
              <input class="input" type="number" value={chForm().rpm} onInput={(e) => setChForm((p) => ({ ...p, rpm: parseInt(e.currentTarget.value) || 0 }))} />
            </div>
            <div>
              <label class="label">RPD</label>
              <input class="input" type="number" value={chForm().rpd} onInput={(e) => setChForm((p) => ({ ...p, rpd: parseInt(e.currentTarget.value) || 0 }))} />
            </div>
            <div>
              <label class="label">优先级</label>
              <input class="input" type="number" value={chForm().priority} onInput={(e) => setChForm((p) => ({ ...p, priority: parseInt(e.currentTarget.value) || 1 }))} />
            </div>
            <div class="flex items-end gap-4">
              <label class="flex items-center gap-2 cursor-pointer">
                <input type="checkbox" checked={chForm().is_free} onChange={(e) => setChForm((p) => ({ ...p, is_free: e.currentTarget.checked }))} class="w-4 h-4 rounded" />
                <span class="text-sm text-dark-200">免费</span>
              </label>
              <label class="flex items-center gap-2 cursor-pointer">
                <input type="checkbox" checked={chForm().is_enabled} onChange={(e) => setChForm((p) => ({ ...p, is_enabled: e.currentTarget.checked }))} class="w-4 h-4 rounded" />
                <span class="text-sm text-dark-200">启用</span>
              </label>
            </div>
          </div>
          <div>
            <label class="label">模型列表 (JSON 数组)</label>
            <textarea class="input min-h-[80px]" value={chForm().models} onInput={(e) => setChForm((p) => ({ ...p, models: e.currentTarget.value }))} placeholder='["Qwen/Qwen3-8B", "Qwen/Qwen2.5-7B-Instruct"]' />
          </div>
          <div class="flex justify-end gap-3 pt-2">
            <button class="btn btn-secondary" onClick={() => setShowChannelModal(false)}>取消</button>
            <button class="btn btn-primary" disabled={saving()} onClick={saveChannel}>
              {saving() ? '保存中...' : '保存'}
            </button>
          </div>
        </div>
      </Modal>

      {/* Group Modal */}
      <Modal open={showGroupModal()} onClose={() => setShowGroupModal(false)} title={editingGroup() ? '编辑模型组' : '新增模型组'} size="lg">
        <div class="space-y-4">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">名称</label>
              <input class="input" value={grpForm().name} onInput={(e) => setGrpForm((p) => ({ ...p, name: e.currentTarget.value }))} placeholder="如 pool-chat-free" />
            </div>
            <div>
              <label class="label">策略</label>
              <select class="input" value={grpForm().strategy} onChange={(e) => setGrpForm((p) => ({ ...p, strategy: e.currentTarget.value }))}>
                <option value="priority-health">priority-health</option>
                <option value="round-robin">round-robin</option>
              </select>
            </div>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">描述</label>
              <input class="input" value={grpForm().description} onInput={(e) => setGrpForm((p) => ({ ...p, description: e.currentTarget.value }))} placeholder="可选描述" />
            </div>
            <div>
              <label class="label">Sticky TTL (秒)</label>
              <input class="input" type="number" value={grpForm().sticky_ttl_seconds} onInput={(e) => setGrpForm((p) => ({ ...p, sticky_ttl_seconds: parseInt(e.currentTarget.value) || 0 }))} />
            </div>
          </div>
          <div>
            <div class="flex items-center justify-between mb-2">
              <label class="label mb-0">成员</label>
              <button class="btn btn-ghost btn-sm" onClick={addGroupMember}>
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
                </svg>
                添加成员
              </button>
            </div>
            <Show when={grpForm().members.length > 0} fallback={<p class="text-sm text-dark-400">暂无成员，点击上方按钮添加。</p>}>
              <div class="space-y-2">
                <For each={grpForm().members}>
                  {(member, index) => (
                    <div class="grid grid-cols-[1fr_1fr_80px_40px] gap-2 items-end">
                      <input class="input" value={member.channel_name} onInput={(e) => updateGroupMember(index(), 'channel_name', e.currentTarget.value)} placeholder="渠道名" />
                      <input class="input" value={member.model} onInput={(e) => updateGroupMember(index(), 'model', e.currentTarget.value)} placeholder="模型名" />
                      <input class="input" type="number" value={member.weight} onInput={(e) => updateGroupMember(index(), 'weight', parseInt(e.currentTarget.value) || 1)} placeholder="权重" />
                      <button class="btn btn-ghost btn-sm text-red-400" onClick={() => removeGroupMember(index())}>
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                        </svg>
                      </button>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
          <div class="flex justify-end gap-3 pt-2">
            <button class="btn btn-secondary" onClick={() => setShowGroupModal(false)}>取消</button>
            <button class="btn btn-primary" disabled={saving()} onClick={saveGroup}>
              {saving() ? '保存中...' : '保存'}
            </button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default LLMProxyConfig
