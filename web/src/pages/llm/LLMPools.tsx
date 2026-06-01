import { Component, For, Show, createSignal, createResource, createMemo, onMount } from 'solid-js'
import type { LLMChannelConfig, LLMConversationBinding, LLMModelRateLimit, LLMCodingStrategy, LLMGroupStatus } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import {
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

const LLMPools: Component = () => {
  const toast = useToast()

  const [channelsData, { refetch: refetchChannels }] = createResource(() => llmProxyApi.listChannels())
  const [groupsData, { refetch: refetchGroups }] = createResource(() => llmProxyApi.groupsStatus())

  const channels = () => channelsData()?.data || []
  const groups = () => groupsData()?.data || []
  const enabledChannels = createMemo(() => channels().filter((c) => c.is_enabled))

  // Coding strategy (loaded once, mutated locally on switch)
  const [strategy, setStrategy] = createSignal<LLMCodingStrategy>('free_first')
  const [savingStrategy, setSavingStrategy] = createSignal(false)

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

  // Sticky conversation bindings
  const [conversations, setConversations] = createSignal<LLMConversationBinding[]>([])
  const [loadingConvs, setLoadingConvs] = createSignal(true)
  const [deletingConv, setDeletingConv] = createSignal<string | null>(null)

  // Adaptive rate-limit learning (Tier 3)
  const [rateLimitsData, { refetch: refetchRateLimits }] = createResource(() => llmProxyApi.listRateLimits())

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

  onMount(() => {
    loadStrategy()
    loadConversations()
  })

  // Routing matrix derivation (from channel task_types + tier tags)
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
    channels().forEach((c) => {
      m[c.name] = deriveTier(c.tier, c.is_free)
    })
    return m
  })

  const handleRefresh = async () => {
    await Promise.all([refetchChannels(), refetchGroups(), loadStrategy(), loadConversations(), refetchRateLimits()])
    toast.success('池子路由数据已刷新')
  }

  const loading = () => channelsData.loading || groupsData.loading

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">池子路由</h1>
          <p class="text-sm text-dark-400 mt-1">任务分层路由、编码策略与会话粘性绑定</p>
        </div>
        <button class="btn btn-primary" onClick={handleRefresh} disabled={loading()}>
          <svg class={`w-4 h-4 ${loading() ? 'animate-spin' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          刷新
        </button>
      </div>

      <div class="space-y-6">
        {/* Coding strategy switcher */}
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

        {/* Task routing matrix */}
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
                <p class="empty-state-description">请在配置页面启用渠道后再查看路由矩阵。</p>
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

        {/* Model group routing */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">模型组路由</h2>
            <span class="text-sm text-dark-400">共 {groups().length} 个模型组</span>
          </div>
          <Show
            when={groups().length > 0}
            fallback={
              <div class="empty-state">
                <p class="empty-state-title">暂无模型组</p>
                <p class="empty-state-description">请在配置页面创建模型组。</p>
              </div>
            }
          >
            <div class="space-y-3">
              <For each={groups()}>
                {(group: LLMGroupStatus) => (
                  <div class="p-4 rounded-xl bg-dark-700/30 border border-dark-600/50">
                    <div class="flex items-center gap-2 flex-wrap mb-3">
                      <span class="font-medium text-white">{group.name}</span>
                      <span class="badge badge-primary">{group.strategy}</span>
                      <span class="badge badge-gray">{group.members.length} 成员</span>
                    </div>
                    <div class="flex flex-wrap gap-2">
                      <For each={group.members}>
                        {(m) => {
                          const tier = channelTierMap()[m.channel] || 'standard'
                          return (
                            <span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-dark-800/60 text-xs">
                              <span class={`badge ${getTierBadge(tier)}`}>{getTierLabel(tier)}</span>
                              <span class="text-dark-200">{m.channel}</span>
                              <span class="text-dark-500 font-mono">{m.model}</span>
                            </span>
                          )
                        }}
                      </For>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </div>

        {/* Adaptive rate-limit learning state (Tier 3) */}
        <div class="card">
          <h2 class="text-lg font-semibold text-white mb-4">自适应限流学习</h2>
          <p class="text-sm text-dark-400 mb-4">
            实时展示每渠道×模型的限流配置与学习状态（Tier 3）。通过观察上游 429 响应，系统自动调整安全 RPM 并记录信心分。
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

        {/* Sticky conversation bindings */}
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
    </div>
  )
}

export default LLMPools
