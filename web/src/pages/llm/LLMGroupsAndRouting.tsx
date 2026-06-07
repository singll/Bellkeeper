import { Component, For, Show, createSignal, createResource, createMemo, onMount } from 'solid-js'
import { createStore, produce } from 'solid-js/store'
import type { LLMGroupStatus, LLMModelGroupConfig, LLMModelGroupMemberConfig, LLMChannelConfig, LLMConversationBinding, LLMModelRateLimit, LLMCodingStrategy } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import {
  formatPercent,
  getCircuitBadge,
  getCircuitLabel,
  getCircuitDot,
  getSuccessRateColor,
  deriveTier,
  getTierLabel,
  getTierBadge,
  parseJsonArray,
  formatCents,
  formatDateShort,
} from './shared'

const TIERS = ['free', 'standard', 'premium']
const DEFAULT_TASK_TYPES = ['coding', 'analysis', 'summary', 'translate', 'general']

interface StrategyOption {
  value: LLMCodingStrategy
  label: string
  desc: string
}

const STRATEGIES: StrategyOption[] = [
  { value: 'free_first', label: '成本优先', desc: '优先免费层 → 标准层 → 高级层，最大化节省成本。' },
  { value: 'quality_first', label: '质量优先', desc: '优先高级层 → 标准层 → 免费层，最大化输出质量。' },
  { value: 'complexity_aware', label: '复杂度自适应', desc: '按请求复杂度动态选层：简单→免费，中等→标准，复杂→高级。' },
]

const LLMGroupsAndRouting: Component = () => {
  const toast = useToast()

  const [groupsData, { refetch: refetchGroups }] = createResource(() => llmProxyApi.groupsStatus())
  const [groupConfigsData, { refetch: refetchGroupConfigs }] = createResource(() => llmProxyApi.listGroups())
  const [channelsData, { refetch: refetchChannels }] = createResource(() => llmProxyApi.listChannels())
  const [rateLimitsData, { refetch: refetchRateLimits }] = createResource(() => llmProxyApi.listRateLimits())

  const groups = () => groupsData()?.data || []
  const groupConfigs = () => groupConfigsData()?.data || []
  const channels = () => channelsData()?.data || []
  const enabledChannels = createMemo(() => channels().filter((c) => c.is_enabled))

  const [busyGroup, setBusyGroup] = createSignal<string | null>(null)
  const [strategy, setStrategy] = createSignal<LLMCodingStrategy>('free_first')
  const [savingStrategy, setSavingStrategy] = createSignal(false)

  const [conversations, setConversations] = createSignal<LLMConversationBinding[]>([])
  const [loadingConvs, setLoadingConvs] = createSignal(true)
  const [deletingConv, setDeletingConv] = createSignal<string | null>(null)

  const loadStrategy = async () => {
    try {
      const res = await llmProxyApi.getCodingStrategy()
      if (res.data?.strategy) setStrategy(res.data.strategy)
    } catch (err) {
      toast.error('加载编码策略失败: ' + (err as Error).message)
    }
  }

  const handleSetStrategy = async (s: LLMCodingStrategy) => {
    if (s === strategy() || savingStrategy()) return
    setSavingStrategy(true)
    try {
      await llmProxyApi.setCodingStrategy(s)
      setStrategy(s)
      toast.success(`编码策略已切换为「${STRATEGIES.find((x) => x.value === s)?.label || s}」`)
    } catch (err) {
      toast.error('切换失败: ' + (err as Error).message)
    } finally {
      setSavingStrategy(false)
    }
  }

  const loadConversations = async () => {
    setLoadingConvs(true)
    try {
      const res = await llmProxyApi.listConversations()
      setConversations(res.data || [])
    } catch (err) {
      toast.error('加载粘性绑定失败: ' + (err as Error).message)
    } finally {
      setLoadingConvs(false)
    }
  }

  const handleDeleteConv = async (convId: string) => {
    setDeletingConv(convId)
    try {
      await llmProxyApi.deleteConversation(convId)
      toast.success('已删除粘性绑定')
      await loadConversations()
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    } finally {
      setDeletingConv(null)
    }
  }

  const handleClearSticky = async (name: string) => {
    setBusyGroup(name)
    try {
      const result = await llmProxyApi.clearGroupSticky(name)
      toast.success(`已清理 ${result.data.cleared} 条粘性绑定`)
      await refetchGroups()
    } catch (err) {
      toast.error('清理失败: ' + (err as Error).message)
    } finally {
      setBusyGroup(null)
    }
  }

  onMount(() => {
    loadStrategy()
    loadConversations()
  })

  const refetchAll = () => Promise.all([refetchGroups(), refetchGroupConfigs(), refetchChannels(), loadStrategy(), loadConversations(), refetchRateLimits()])

  const usingDefaultTaskTypes = createMemo(
    () => !enabledChannels().some((c) => parseJsonArray(c.task_types).length > 0)
  )
  const taskTypes = createMemo(() => {
    const set = new Set<string>()
    enabledChannels().forEach((c) => parseJsonArray(c.task_types).forEach((t) => set.add(t)))
    const arr = Array.from(set).sort()
    return arr.length > 0 ? arr : DEFAULT_TASK_TYPES
  })

  const cellChannels = (taskType: string, tier: string): LLMChannelConfig[] =>
    enabledChannels().filter((c) => {
      const tt = parseJsonArray(c.task_types)
      const eligible = tt.length === 0 || tt.includes(taskType)
      return eligible && deriveTier(c.tier, c.is_free) === tier
    })

  const channelTierMap = createMemo(() => {
    const m: Record<string, string> = {}
    channels().forEach((c) => { m[c.name] = deriveTier(c.tier, c.is_free) })
    return m
  })

  const [showGroupModal, setShowGroupModal] = createSignal(false)
  const [editingGroup, setEditingGroup] = createSignal<LLMModelGroupConfig | null>(null)
  const [saving, setSaving] = createSignal(false)

  const [grpForm, setGrpForm] = createStore({
    name: '',
    description: '',
    strategy: 'priority-health',
    sticky_ttl_seconds: 600,
    members: [] as LLMModelGroupMemberConfig[],
  })

  const openGroupModal = (g?: LLMModelGroupConfig) => {
    if (g) {
      setEditingGroup(g)
      setGrpForm({ name: g.name, description: g.description, strategy: g.strategy, sticky_ttl_seconds: g.sticky_ttl_seconds, members: g.members.map((m) => ({ channel_name: m.channel_name, model: m.model, weight: m.weight })) })
    } else {
      setEditingGroup(null)
      setGrpForm({ name: '', description: '', strategy: 'priority-health', sticky_ttl_seconds: 600, members: [] })
    }
    setShowGroupModal(true)
  }

  const saveGroup = async () => {
    setSaving(true)
    try {
      const data = { name: grpForm.name, description: grpForm.description, strategy: grpForm.strategy, sticky_ttl_seconds: grpForm.sticky_ttl_seconds, members: [...grpForm.members] }
      const editing = editingGroup()
      if (editing) { await llmProxyApi.updateGroup(editing.id, data); toast.success('模型组已更新') }
      else { await llmProxyApi.createGroup(data); toast.success('模型组已创建') }
      setShowGroupModal(false)
      await refetchAll()
    } catch (err) { toast.error('保存失败: ' + (err as Error).message) }
    finally { setSaving(false) }
  }

  const deleteGroup = async (id: number) => {
    if (!confirm('确定删除此模型组？')) return
    try { await llmProxyApi.deleteGroup(id); toast.success('模型组已删除'); await refetchAll() }
    catch (err) { toast.error('删除失败: ' + (err as Error).message) }
  }

  const addGroupMember = () => setGrpForm('members', produce((m) => { m.push({ channel_name: '', model: '', weight: 1 }) }))
  const removeGroupMember = (index: number) => setGrpForm('members', produce((m) => { m.splice(index, 1) }))
  const updateGroupMember = (index: number, field: keyof LLMModelGroupMemberConfig, value: string | number) => {
    setGrpForm('members', index, field, value)
  }

  const loading = () => groupsData.loading || groupConfigsData.loading || channelsData.loading

  return (
    <div class="animate-fade-in">
      <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">模型组与路由</h1>
          <p class="text-sm text-dark-400 mt-1">模型组管理、任务路由矩阵、编码策略与会话粘性</p>
        </div>
        <div class="flex items-center gap-2">
          <button class="btn btn-primary btn-sm" onClick={() => openGroupModal()}>
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
            </svg>
            新增模型组
          </button>
          <button class="btn btn-secondary btn-sm" onClick={refetchAll} disabled={loading()}>
            <svg class={`w-4 h-4 ${loading() ? 'animate-spin' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            刷新
          </button>
        </div>
      </div>

      <div class="space-y-6">
        <div class="card">
          <h2 class="text-lg font-semibold text-white mb-1">编码任务路由策略</h2>
          <p class="text-sm text-dark-400 mb-4">控制 coding 类请求在免费 / 标准 / 高级三层渠道间的优先顺序。</p>
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <For each={STRATEGIES}>
              {(opt) => (
                <button
                  class={`text-left p-4 rounded-xl border transition-all ${
                    strategy() === opt.value
                      ? 'border-primary-500/60 bg-primary-500/10'
                      : 'border-dark-600/50 bg-dark-700/30 hover:bg-dark-700/50'
                  }`}
                  disabled={savingStrategy()}
                  onClick={() => handleSetStrategy(opt.value)}
                >
                  <div class="flex items-center justify-between mb-1">
                    <span class="font-medium text-white">{opt.label}</span>
                    <Show when={strategy() === opt.value}>
                      <span class="badge badge-primary">当前</span>
                    </Show>
                  </div>
                  <div class="text-xs text-dark-400 leading-relaxed">{opt.desc}</div>
                  <div class="text-xs text-dark-500 mt-2 font-mono">{opt.value}</div>
                </button>
              )}
            </For>
          </div>
          <Show when={strategy() === 'complexity_aware'}>
            <div class="mt-4 p-3 rounded-xl bg-dark-700/40 border border-dark-600/50 text-sm text-dark-300">
              <span class="text-dark-200 font-medium">复杂度分级：</span>
              简单任务 → 免费层，中等任务 → 标准层，复杂任务 → 高级层。
              <span class="text-dark-500"> （复杂度阈值在服务端配置，暂未通过 API 暴露调整。）</span>
            </div>
          </Show>
        </div>

        <div class="card">
          <div class="flex items-center justify-between mb-1">
            <h2 class="text-lg font-semibold text-white">任务路由矩阵</h2>
            <span class="text-sm text-dark-400">{enabledChannels().length} 个启用渠道</span>
          </div>
          <p class="text-sm text-dark-400 mb-4">
            每种任务类型在各能力层可路由到的渠道（由渠道的 task_types 与 tier 标签派生）。
          </p>
          <Show when={usingDefaultTaskTypes()}>
            <div class="mb-3 p-2.5 rounded-lg bg-amber-500/10 border border-amber-500/30 text-xs text-amber-300">
              当前没有渠道声明 task_types，默认所有渠道适用全部任务类型；下方为示例任务类型分组。
            </div>
          </Show>
          <Show
            when={enabledChannels().length > 0}
            fallback={
              <div class="empty-state">
                <p class="empty-state-title">暂无启用渠道</p>
                <p class="empty-state-description">请在渠道管理页面启用渠道后再查看路由矩阵。</p>
              </div>
            }
          >
            <div class="overflow-x-auto rounded-xl border border-dark-600/50">
              <table class="table">
                <thead>
                  <tr>
                    <th>任务类型</th>
                    <For each={TIERS}>{(tier) => <th>{getTierLabel(tier)}层</th>}</For>
                  </tr>
                </thead>
                <tbody>
                  <For each={taskTypes()}>
                    {(tt) => (
                      <tr>
                        <td class="font-medium text-white">{tt}</td>
                        <For each={TIERS}>
                          {(tier) => {
                            const cs = cellChannels(tt, tier)
                            return (
                              <td>
                                <Show when={cs.length > 0} fallback={<span class="text-dark-600">—</span>}>
                                  <div class="flex flex-wrap gap-1">
                                    <For each={cs}>
                                      {(c) => (
                                        <span class="inline-flex px-2 py-0.5 rounded-md bg-dark-700/60 text-xs text-dark-200">
                                          {c.name}
                                        </span>
                                      )}
                                    </For>
                                  </div>
                                </Show>
                              </td>
                            )
                          }}
                        </For>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>
        </div>

        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">模型组</h2>
            <span class="text-sm text-dark-400">共 {groups().length} 个模型组</span>
          </div>
          <Show
            when={groups().length > 0}
            fallback={
              <div class="empty-state">
                <p class="empty-state-title">暂无模型组</p>
                <p class="empty-state-description">点击「新增模型组」创建虚拟模型池。</p>
              </div>
            }
          >
            <div class="space-y-4">
              <For each={groups()}>
                {(group: LLMGroupStatus) => {
                  const cfg = () => groupConfigs().find((g) => g.name === group.name)
                  return (
                    <div class="card card-hover">
                      <div class="flex flex-col xl:flex-row xl:items-start justify-between gap-4 mb-4">
                        <div>
                          <div class="flex items-center gap-2 flex-wrap mb-2">
                            <h3 class="text-lg font-semibold text-white">{group.name}</h3>
                            <span class="badge badge-primary">{group.strategy}</span>
                            <span class="badge badge-gray">Sticky TTL {group.sticky_ttl_seconds}s</span>
                            <span class="badge badge-gray">绑定 {group.sticky_bindings}</span>
                          </div>
                          <p class="text-sm text-dark-400">{group.description || '未填写描述'}</p>
                        </div>
                        <div class="flex items-center gap-2 flex-shrink-0">
                          <button class="btn btn-secondary btn-sm" disabled={busyGroup() === group.name} onClick={() => handleClearSticky(group.name)}>
                            {busyGroup() === group.name ? '清理中...' : '清理粘性绑定'}
                          </button>
                          <Show when={cfg()}>
                            <button class="btn btn-ghost btn-sm" onClick={() => openGroupModal(cfg())}>编辑</button>
                            <button class="btn btn-ghost btn-sm text-red-400" onClick={() => deleteGroup(cfg()!.id)}>删除</button>
                          </Show>
                        </div>
                      </div>

                      <div class="overflow-x-auto rounded-xl border border-dark-600/50">
                        <table class="table">
                          <thead>
                            <tr>
                              <th>渠道</th><th>模型</th><th>权重</th><th>可用</th><th>状态</th><th>成功率</th>
                            </tr>
                          </thead>
                          <tbody>
                            <For each={group.members}>
                              {(member) => (
                                <tr>
                                  <td>
                                    <div class="flex items-center gap-2">
                                      <span class={`status-dot ${member.available ? getCircuitDot(member.health.state) : 'status-dot-gray'}`} />
                                      <span>{member.channel}</span>
                                    </div>
                                  </td>
                                  <td class="font-mono text-xs text-dark-300">{member.model}</td>
                                  <td>{member.weight}</td>
                                  <td>
                                    <span class={`badge ${member.available ? 'badge-success' : 'badge-danger'}`}>
                                      {member.available ? '可用' : '不可用'}
                                    </span>
                                  </td>
                                  <td>
                                    <span class={`badge ${getCircuitBadge(member.health.state)}`}>
                                      {getCircuitLabel(member.health.state)}
                                    </span>
                                  </td>
                                  <td>
                                    <span class={getSuccessRateColor(member.health.recent_success_rate)}>
                                      {formatPercent(member.health.recent_success_rate)}
                                    </span>
                                  </td>
                                </tr>
                              )}
                            </For>
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )
                }}
              </For>
            </div>
          </Show>
        </div>

        <div class="card">
          <h2 class="text-lg font-semibold text-white mb-4">自适应限流学习</h2>
          <p class="text-sm text-dark-400 mb-4">
            实时展示每渠道×模型的限流配置与学习状态。通过观察上游 429 响应，系统自动调整安全 RPM 并记录信心分。
          </p>
          <Show
            when={(rateLimitsData()?.data || []).length > 0}
            fallback={
              <div class="empty-state">
                <p class="empty-state-title">暂无限流数据</p>
                <p class="empty-state-description">启用渠道并发送请求后限流学习数据将出现。</p>
              </div>
            }
          >
            <div class="overflow-x-auto rounded-xl border border-dark-600/50">
              <table class="table text-sm">
                <thead>
                  <tr>
                    <th>渠道</th><th>模型</th><th>配置 RPM</th><th>学习 RPM（安全）</th><th>信心分</th><th>最后 429</th><th>锁定</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={(rateLimitsData()?.data || []) as LLMModelRateLimit[]}>
                    {(rl) => {
                      const chName = enabledChannels().find((c) => c.id === rl.channel_id)?.name || `#${rl.channel_id}`
                      return (
                        <tr>
                          <td class="font-medium text-white">{chName}</td>
                          <td class="font-mono text-xs text-dark-300 max-w-[140px] truncate">{rl.model}</td>
                          <td class="text-dark-300">{rl.configured_rpm}</td>
                          <td class="text-emerald-300 font-medium">{rl.learned_rpm_safe}</td>
                          <td>
                            <span class={`text-xs font-medium ${rl.confidence_score >= 0.8 ? 'text-emerald-400' : rl.confidence_score >= 0.5 ? 'text-amber-400' : 'text-red-400'}`}>
                              {(rl.confidence_score * 100).toFixed(0)}%
                            </span>
                          </td>
                          <td class="text-xs text-dark-400">{rl.last_429_at ? formatDateShort(rl.last_429_at) : '--'}</td>
                          <td><span class={`badge ${rl.locked ? 'badge-warning' : 'badge-gray'}`}>{rl.locked ? '锁定' : '学习中'}</span></td>
                        </tr>
                      )
                    }}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>
        </div>

        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">会话粘性绑定</h2>
            <span class="text-sm text-dark-400">{conversations().length} 条绑定</span>
          </div>
          <Show
            when={!loadingConvs()}
            fallback={
              <div class="py-8 flex items-center justify-center">
                <div class="loading-spinner" />
                <span class="ml-3 text-dark-400">加载粘性绑定...</span>
              </div>
            }
          >
            <Show
              when={conversations().length > 0}
              fallback={
                <div class="empty-state">
                  <p class="empty-state-title">暂无粘性绑定</p>
                  <p class="empty-state-description">带 X-Conversation-ID 或 cache_control 的请求会在此建立会话→渠道绑定。</p>
                </div>
              }
            >
              <div class="overflow-x-auto rounded-xl border border-dark-600/50">
                <table class="table">
                  <thead>
                    <tr>
                      <th>会话 ID</th><th>渠道</th><th>模型</th><th>任务</th><th>请求数</th><th>Token</th><th>成本</th><th>最后活跃</th><th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <For each={conversations()}>
                      {(conv) => (
                        <tr>
                          <td class="font-mono text-xs text-dark-300 max-w-[160px] truncate" title={conv.conversation_id}>{conv.conversation_id}</td>
                          <td class="text-sm font-medium text-white">{conv.channel_name}</td>
                          <td class="text-xs font-mono text-dark-300 max-w-[140px] truncate" title={conv.model}>{conv.model}</td>
                          <td class="text-xs text-dark-400">{conv.task_type || '--'}</td>
                          <td class="text-sm text-dark-300">{conv.request_count}</td>
                          <td class="text-sm text-dark-300">{conv.total_tokens.toLocaleString()}</td>
                          <td class="text-sm text-dark-300">{formatCents(conv.total_cost_cents)}</td>
                          <td class="text-xs text-dark-400 whitespace-nowrap">{formatDateShort(conv.last_seen_at)}</td>
                          <td>
                            <button
                              class="btn btn-secondary btn-sm"
                              disabled={deletingConv() === conv.conversation_id}
                              onClick={() => handleDeleteConv(conv.conversation_id)}
                            >
                              {deletingConv() === conv.conversation_id ? '删除中...' : '删除'}
                            </button>
                          </td>
                        </tr>
                      )}
                    </For>
                  </tbody>
                </table>
              </div>
            </Show>
          </Show>
        </div>
      </div>

      <Modal open={showGroupModal()} onClose={() => setShowGroupModal(false)} title={editingGroup() ? '编辑模型组' : '新增模型组'} size="lg">
        <div class="space-y-4">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div><label class="label">名称</label><input class="input" value={grpForm.name} onInput={(e) => setGrpForm('name', e.currentTarget.value)} placeholder="如 pool-chat-free" /></div>
            <div><label class="label">策略</label><select class="input" value={grpForm.strategy} onChange={(e) => setGrpForm('strategy', e.currentTarget.value)}><option value="priority-health">priority-health</option><option value="round-robin">round-robin</option></select></div>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div><label class="label">描述</label><input class="input" value={grpForm.description} onInput={(e) => setGrpForm('description', e.currentTarget.value)} placeholder="可选描述" /></div>
            <div><label class="label">Sticky TTL (秒)</label><input class="input" type="number" value={grpForm.sticky_ttl_seconds} onInput={(e) => setGrpForm('sticky_ttl_seconds', parseInt(e.currentTarget.value) || 0)} /></div>
          </div>
          <div>
            <div class="flex items-center justify-between mb-2">
              <label class="label mb-0">成员</label>
              <button class="btn btn-ghost btn-sm" onClick={addGroupMember}>
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" /></svg>
                添加成员
              </button>
            </div>
            <Show when={grpForm.members.length > 0} fallback={<p class="text-sm text-dark-400">暂无成员，点击上方按钮添加。</p>}>
              <div class="space-y-2">
                <For each={grpForm.members}>
                  {(member, index) => (
                    <div class="grid grid-cols-[1fr_1fr_80px_40px] gap-2 items-end">
                      <input class="input" value={member.channel_name} onInput={(e) => updateGroupMember(index(), 'channel_name', e.currentTarget.value)} placeholder="渠道名" />
                      <input class="input" value={member.model} onInput={(e) => updateGroupMember(index(), 'model', e.currentTarget.value)} placeholder="模型名" />
                      <input class="input" type="number" value={member.weight} onInput={(e) => updateGroupMember(index(), 'weight', parseInt(e.currentTarget.value) || 1)} placeholder="权重" />
                      <button class="btn btn-ghost btn-sm text-red-400" onClick={() => removeGroupMember(index())}>
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>
                      </button>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
          <div class="flex justify-end gap-3 pt-2">
            <button class="btn btn-secondary" onClick={() => setShowGroupModal(false)}>取消</button>
            <button class="btn btn-primary" disabled={saving()} onClick={saveGroup}>{saving() ? '保存中...' : '保存'}</button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default LLMGroupsAndRouting
