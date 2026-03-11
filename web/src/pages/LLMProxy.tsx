import { A } from '@solidjs/router'
import { Component, For, Show, createMemo, createResource, createSignal } from 'solid-js'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import type { LLMChannelHealth, LLMChannelStatus, LLMGroupStatus } from '@/types'

type TabKey = 'overview' | 'channels' | 'groups'
type HealthFilter = 'all' | 'closed' | 'open' | 'half_open'
type BillingFilter = 'all' | 'free' | 'paid'

const formatDateTime = (value?: string) => {
  if (!value) return '--'
  return new Date(value).toLocaleString('zh-CN')
}

const formatPercent = (value: number) => `${Math.round(value * 100)}%`

const getCircuitBadge = (state: string) => {
  switch (state) {
    case 'closed':
      return 'badge-success'
    case 'half_open':
      return 'badge-warning'
    case 'open':
      return 'badge-danger'
    default:
      return 'badge-gray'
  }
}

const getCircuitLabel = (state: string) => {
  switch (state) {
    case 'closed':
      return '正常'
    case 'half_open':
      return '半开'
    case 'open':
      return '熔断'
    default:
      return state
  }
}

const getCircuitDot = (state: string) => {
  switch (state) {
    case 'closed':
      return 'status-dot-success'
    case 'half_open':
      return 'status-dot-warning'
    case 'open':
      return 'status-dot-danger'
    default:
      return 'status-dot-gray'
  }
}

const getSuccessRateColor = (rate: number) => {
  if (rate >= 0.9) return 'text-emerald-400'
  if (rate >= 0.7) return 'text-amber-400'
  return 'text-red-400'
}

const getProgressClass = (ratio: number) => {
  if (ratio >= 0.7) return 'bg-emerald-500'
  if (ratio >= 0.35) return 'bg-amber-500'
  return 'bg-red-500'
}

const calcRatio = (current: number, total: number) => {
  if (total <= 0) return 0
  return Math.min(current / total, 1)
}

const LLMProxy: Component = () => {
  const toast = useToast()
  const [activeTab, setActiveTab] = createSignal<TabKey>('overview')
  const [healthFilter, setHealthFilter] = createSignal<HealthFilter>('all')
  const [billingFilter, setBillingFilter] = createSignal<BillingFilter>('all')
  const [keyword, setKeyword] = createSignal('')
  const [busyChannel, setBusyChannel] = createSignal<string | null>(null)
  const [busyGroup, setBusyGroup] = createSignal<string | null>(null)

  const [channels, { refetch: refetchChannels }] = createResource(() => llmProxyApi.channelsStatus())
  const [groups, { refetch: refetchGroups }] = createResource(() => llmProxyApi.groupsStatus())

  const refreshAll = async () => {
    await Promise.all([refetchChannels(), refetchGroups()])
    toast.success('LLM Proxy 状态已刷新')
  }

  const channelList = createMemo(() => channels()?.data || [])
  const groupList = createMemo(() => groups()?.data || [])

  const totalChannels = createMemo(() => channelList().length)
  const healthyChannels = createMemo(() => channelList().filter((item) => item.health.state === 'closed').length)
  const circuitBrokenChannels = createMemo(() => channelList().filter((item) => item.health.state === 'open').length)
  const halfOpenChannels = createMemo(() => channelList().filter((item) => item.health.state === 'half_open').length)

  const overviewStatus = createMemo(() => {
    if (totalChannels() === 0) {
      return {
        label: '未配置',
        badge: 'badge-gray',
        description: '当前没有可用渠道数据',
      }
    }

    if (circuitBrokenChannels() > 0) {
      return {
        label: '部分降级',
        badge: 'badge-warning',
        description: `有 ${circuitBrokenChannels()} 个渠道处于熔断状态`,
      }
    }

    if (halfOpenChannels() > 0) {
      return {
        label: '恢复观察中',
        badge: 'badge-warning',
        description: `有 ${halfOpenChannels()} 个渠道处于半开探测状态`,
      }
    }

    return {
      label: '运行正常',
      badge: 'badge-success',
      description: '所有已启用渠道均可正常服务',
    }
  })

  const alerts = createMemo(() => {
    const items: string[] = []

    const openChannels = channelList().filter((item) => item.health.state === 'open')
    if (openChannels.length > 0) {
      items.push(`熔断渠道：${openChannels.map((item) => item.name).join('、')}`)
    }

    const unstableChannels = channelList().filter(
      (item) => item.health.state !== 'open' && item.health.recent_success_rate < 0.7
    )
    if (unstableChannels.length > 0) {
      items.push(`成功率偏低：${unstableChannels.map((item) => item.name).join('、')}`)
    }

    const stickyGroups = groupList().filter((group) => group.sticky_bindings > 0)
    if (stickyGroups.length > 0) {
      items.push(`存在粘性绑定：${stickyGroups.map((group) => `${group.name}(${group.sticky_bindings})`).join('、')}`)
    }

    return items
  })

  const filteredChannels = createMemo(() => {
    const q = keyword().trim().toLowerCase()

    return channelList().filter((channel) => {
      const matchesHealth = healthFilter() === 'all' || channel.health.state === healthFilter()
      const matchesBilling =
        billingFilter() === 'all' ||
        (billingFilter() === 'free' ? channel.is_free : !channel.is_free)
      const matchesKeyword =
        q === '' ||
        channel.name.toLowerCase().includes(q) ||
        channel.base_url.toLowerCase().includes(q) ||
        channel.models.some((model) => model.toLowerCase().includes(q))

      return matchesHealth && matchesBilling && matchesKeyword
    })
  })

  const handleResetCircuit = async (name: string) => {
    setBusyChannel(name)
    try {
      const result = await llmProxyApi.resetChannelCircuit(name)
      toast.success(result.message)
      await refetchChannels()
    } catch (err) {
      toast.error('重置失败: ' + (err as Error).message)
    } finally {
      setBusyChannel(null)
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

  const renderProgress = (current: number, total: number, label: string) => {
    const ratio = calcRatio(current, total)
    return (
      <div>
        <div class="flex items-center justify-between text-xs text-dark-400 mb-1.5">
          <span>{label}</span>
          <span>
            {current} / {total || '--'}
          </span>
        </div>
        <div class="h-2 rounded-full bg-dark-700/70 overflow-hidden">
          <div
            class={`h-full rounded-full transition-all ${getProgressClass(ratio)}`}
            style={{ width: `${Math.max(ratio * 100, ratio > 0 ? 6 : 0)}%` }}
          />
        </div>
      </div>
    )
  }

  const renderHealthMeta = (health: LLMChannelHealth) => (
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
      <div class="p-3 bg-dark-700/40 rounded-xl">
        <div class="text-dark-400 mb-1">最近成功率</div>
        <div class={`font-semibold ${getSuccessRateColor(health.recent_success_rate)}`}>
          {formatPercent(health.recent_success_rate)}
        </div>
      </div>
      <div class="p-3 bg-dark-700/40 rounded-xl">
        <div class="text-dark-400 mb-1">连续失败</div>
        <div class="font-semibold text-white">{health.consecutive_fails}</div>
      </div>
      <div class="p-3 bg-dark-700/40 rounded-xl">
        <div class="text-dark-400 mb-1">最近成功</div>
        <div class="font-medium text-dark-200">{formatDateTime(health.last_success_at)}</div>
      </div>
      <div class="p-3 bg-dark-700/40 rounded-xl">
        <div class="text-dark-400 mb-1">最近错误</div>
        <div class="font-medium text-dark-200">{formatDateTime(health.last_error_at)}</div>
      </div>
    </div>
  )

  return (
    <div class="animate-fade-in">
      <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">LLM Proxy</h1>
          <p class="text-sm text-dark-400 mt-1">查看渠道健康、模型组路由与熔断/粘性运行态</p>
        </div>
        <div class="flex items-center gap-3">
          <A href="/settings" class="btn btn-secondary">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            </svg>
            查看设置
          </A>
          <button class="btn btn-primary" onClick={refreshAll}>
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            刷新状态
          </button>
        </div>
      </div>

      <div class="card mb-6 bg-primary-500/10 border-primary-500/30">
        <div class="flex items-start gap-3">
          <svg class="w-5 h-5 text-primary-400 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <div class="text-sm">
            <p class="text-primary-300 font-medium">配置建议</p>
            <ul class="text-dark-400 mt-1 space-y-0.5">
              <li>渠道/模型组等结构化配置保留在 YAML 中更合适。</li>
              <li>API Key 等敏感信息应继续保留在环境变量，不建议迁入通用设置。</li>
              <li>设置页更适合承载少量可调参数，而不是完整代理池拓扑。</li>
            </ul>
          </div>
        </div>
      </div>

      <div class="tabs mb-6 w-fit">
        <button class={`tab ${activeTab() === 'overview' ? 'tab-active' : ''}`} onClick={() => setActiveTab('overview')}>
          总览
        </button>
        <button class={`tab ${activeTab() === 'channels' ? 'tab-active' : ''}`} onClick={() => setActiveTab('channels')}>
          渠道
        </button>
        <button class={`tab ${activeTab() === 'groups' ? 'tab-active' : ''}`} onClick={() => setActiveTab('groups')}>
          模型组
        </button>
      </div>

      <Show
        when={!channels.loading && !groups.loading}
        fallback={
          <div class="card py-12">
            <div class="flex items-center justify-center">
              <div class="loading-spinner" />
              <span class="ml-3 text-dark-400">加载 LLM Proxy 状态中...</span>
            </div>
          </div>
        }
      >
        <Show
          when={!channels.error && !groups.error}
          fallback={
            <div class="card py-12">
              <div class="empty-state py-4">
                <svg class="empty-state-icon w-12 h-12 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <p class="empty-state-title">LLM Proxy 数据加载失败</p>
                <p class="empty-state-description">
                  {((channels.error || groups.error) as Error)?.message || '请检查后端服务状态'}
                </p>
                <button class="btn btn-secondary btn-sm mt-3" onClick={refreshAll}>重试</button>
              </div>
            </div>
          }
        >
          <Show when={activeTab() === 'overview'}>
            <div class="space-y-6">
              <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
                <div class="stat-card">
                  <div class="stat-label">渠道总数</div>
                  <div class="stat-value">{totalChannels()}</div>
                  <div class="stat-trend text-dark-400">已启用上游渠道</div>
                </div>
                <div class="stat-card">
                  <div class="stat-label">健康渠道</div>
                  <div class="stat-value text-emerald-400">{healthyChannels()}</div>
                  <div class="stat-trend stat-trend-up">状态 closed</div>
                </div>
                <div class="stat-card">
                  <div class="stat-label">熔断渠道</div>
                  <div class="stat-value text-red-400">{circuitBrokenChannels()}</div>
                  <div class="stat-trend stat-trend-down">状态 open</div>
                </div>
                <div class="stat-card">
                  <div class="stat-label">模型组</div>
                  <div class="stat-value text-primary-300">{groupList().length}</div>
                  <div class="stat-trend text-dark-400">虚拟模型池</div>
                </div>
              </div>

              <div class="grid grid-cols-1 xl:grid-cols-[1.3fr_1fr] gap-6">
                <div class="card">
                  <div class="flex items-center justify-between mb-4">
                    <h2 class="text-lg font-semibold text-white">全局健康摘要</h2>
                    <span class={`badge ${overviewStatus().badge}`}>{overviewStatus().label}</span>
                  </div>
                  <p class="text-dark-300 mb-4">{overviewStatus().description}</p>
                  <Show
                    when={alerts().length > 0}
                    fallback={<div class="text-sm text-emerald-300">当前没有需要处理的告警项。</div>}
                  >
                    <div class="space-y-2">
                      <For each={alerts()}>
                        {(alert) => (
                          <div class="flex items-start gap-2 p-3 rounded-xl bg-dark-700/50 border border-dark-600/50 text-sm text-dark-200">
                            <span class="status-dot status-dot-warning mt-1" />
                            <span>{alert}</span>
                          </div>
                        )}
                      </For>
                    </div>
                  </Show>
                </div>

                <div class="card">
                  <h2 class="text-lg font-semibold text-white mb-4">资源概览</h2>
                  <div class="space-y-4">
                    {renderProgress(healthyChannels(), totalChannels(), '健康渠道占比')}
                    {renderProgress(circuitBrokenChannels(), totalChannels(), '熔断渠道占比')}
                    {renderProgress(
                      groupList().reduce((sum, item) => sum + item.sticky_bindings, 0),
                      Math.max(groupList().length * 5, 1),
                      '粘性绑定热度'
                    )}
                  </div>
                </div>
              </div>

              <div>
                <div class="flex items-center justify-between mb-4">
                  <h2 class="text-lg font-semibold text-white">模型组摘要</h2>
                  <span class="text-sm text-dark-400">共 {groupList().length} 个模型组</span>
                </div>
                <Show
                  when={groupList().length > 0}
                  fallback={
                    <div class="card empty-state">
                      <p class="empty-state-title">暂无模型组</p>
                      <p class="empty-state-description">请先在配置中定义 llm_proxy.model_groups。</p>
                    </div>
                  }
                >
                  <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
                    <For each={groupList()}>
                      {(group) => (
                        <div class="card card-hover">
                          <div class="flex items-start justify-between gap-3 mb-4">
                            <div>
                              <div class="text-lg font-semibold text-white">{group.name}</div>
                              <div class="text-sm text-dark-400 mt-1">{group.description || '未填写描述'}</div>
                            </div>
                            <button
                              class="btn btn-ghost btn-sm"
                              disabled={busyGroup() === group.name}
                              onClick={() => handleClearSticky(group.name)}
                            >
                              {busyGroup() === group.name ? '清理中...' : '清理粘性'}
                            </button>
                          </div>
                          <div class="grid grid-cols-2 gap-3 mb-4 text-sm">
                            <div class="p-3 bg-dark-700/40 rounded-xl">
                              <div class="text-dark-400 mb-1">策略</div>
                              <div class="font-medium text-white">{group.strategy}</div>
                            </div>
                            <div class="p-3 bg-dark-700/40 rounded-xl">
                              <div class="text-dark-400 mb-1">粘性绑定</div>
                              <div class="font-medium text-white">{group.sticky_bindings}</div>
                            </div>
                            <div class="p-3 bg-dark-700/40 rounded-xl">
                              <div class="text-dark-400 mb-1">成员数</div>
                              <div class="font-medium text-white">{group.members.length}</div>
                            </div>
                            <div class="p-3 bg-dark-700/40 rounded-xl">
                              <div class="text-dark-400 mb-1">Sticky TTL</div>
                              <div class="font-medium text-white">{group.sticky_ttl_seconds}s</div>
                            </div>
                          </div>
                          <div class="flex items-center gap-2 flex-wrap">
                            <For each={group.members}>
                              {(member) => (
                                <div class="inline-flex items-center gap-2 px-2.5 py-1.5 rounded-lg bg-dark-700/50 text-xs text-dark-200">
                                  <span class={`status-dot ${member.available ? getCircuitDot(member.health.state) : 'status-dot-gray'}`} />
                                  <span>{member.channel}</span>
                                </div>
                              )}
                            </For>
                          </div>
                        </div>
                      )}
                    </For>
                  </div>
                </Show>
              </div>
            </div>
          </Show>

          <Show when={activeTab() === 'channels'}>
            <div class="space-y-6">
              <div class="card">
                <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label class="label">状态筛选</label>
                    <select class="input" value={healthFilter()} onChange={(e) => setHealthFilter(e.currentTarget.value as HealthFilter)}>
                      <option value="all">全部状态</option>
                      <option value="closed">正常</option>
                      <option value="half_open">半开</option>
                      <option value="open">熔断</option>
                    </select>
                  </div>
                  <div>
                    <label class="label">计费类型</label>
                    <select class="input" value={billingFilter()} onChange={(e) => setBillingFilter(e.currentTarget.value as BillingFilter)}>
                      <option value="all">全部渠道</option>
                      <option value="free">免费</option>
                      <option value="paid">付费</option>
                    </select>
                  </div>
                  <div>
                    <label class="label">搜索</label>
                    <input
                      class="input"
                      type="text"
                      value={keyword()}
                      onInput={(e) => setKeyword(e.currentTarget.value)}
                      placeholder="渠道名 / 模型名 / URL"
                    />
                  </div>
                </div>
              </div>

              <Show
                when={filteredChannels().length > 0}
                fallback={
                  <div class="card empty-state">
                    <p class="empty-state-title">没有匹配的渠道</p>
                    <p class="empty-state-description">请调整筛选条件后重试。</p>
                  </div>
                }
              >
                <div class="space-y-4">
                  <For each={filteredChannels()}>
                    {(channel: LLMChannelStatus) => {
                      const tokenRatio = calcRatio(channel.available_tokens, channel.max_tokens)
                      const dailyRatio = calcRatio(channel.daily_used, channel.daily_limit)

                      return (
                        <div class="card card-hover">
                          <div class="flex flex-col xl:flex-row xl:items-start justify-between gap-4 mb-4">
                            <div>
                              <div class="flex flex-wrap items-center gap-2 mb-2">
                                <h2 class="text-lg font-semibold text-white">{channel.name}</h2>
                                <span class={`badge ${channel.is_free ? 'badge-primary' : 'badge-gray'}`}>
                                  {channel.is_free ? '免费' : '付费'}
                                </span>
                                <span class={`badge ${getCircuitBadge(channel.health.state)}`}>
                                  {getCircuitLabel(channel.health.state)}
                                </span>
                                <span class="badge badge-gray">优先级 {channel.priority}</span>
                              </div>
                              <div class="text-sm text-dark-400 break-all">{channel.base_url}</div>
                              <div class="flex flex-wrap gap-2 mt-3">
                                <For each={channel.models}>
                                  {(model) => <span class="badge badge-gray">{model}</span>}
                                </For>
                              </div>
                            </div>

                            <button
                              class="btn btn-secondary btn-sm"
                              disabled={busyChannel() === channel.name}
                              onClick={() => handleResetCircuit(channel.name)}
                            >
                              {busyChannel() === channel.name ? '重置中...' : '重置熔断器'}
                            </button>
                          </div>

                          <div class="grid grid-cols-1 xl:grid-cols-[1.2fr_1fr] gap-6">
                            <div class="space-y-4">
                              {renderProgress(channel.available_tokens, channel.max_tokens, `令牌桶 (${Math.round(tokenRatio * 100)}%)`)}
                              {renderProgress(channel.daily_used, channel.daily_limit, `日额度 (${Math.round(dailyRatio * 100)}%)`)}
                              <div class="grid grid-cols-2 gap-3 text-sm">
                                <div class="p-3 bg-dark-700/40 rounded-xl">
                                  <div class="text-dark-400 mb-1">RPM / RPD</div>
                                  <div class="font-medium text-white">{channel.rpm_limit} / {channel.rpd_limit}</div>
                                </div>
                                <div class="p-3 bg-dark-700/40 rounded-xl">
                                  <div class="text-dark-400 mb-1">补充速率</div>
                                  <div class="font-medium text-white">{channel.refill_rate_per_s}/s</div>
                                </div>
                              </div>
                            </div>

                            <div class="space-y-3">
                              {renderHealthMeta(channel.health)}
                              <div class="p-3 bg-dark-700/40 rounded-xl text-sm">
                                <div class="text-dark-400 mb-1">最近错误类型</div>
                                <div class="font-medium text-dark-200">{channel.health.last_error_type || '--'}</div>
                              </div>
                              <Show when={channel.health.circuit_open_until}>
                                <div class="p-3 bg-red-500/10 border border-red-500/20 rounded-xl text-sm">
                                  <div class="text-red-300 font-medium mb-1">熔断恢复时间</div>
                                  <div class="text-red-200">{formatDateTime(channel.health.circuit_open_until)}</div>
                                </div>
                              </Show>
                            </div>
                          </div>
                        </div>
                      )
                    }}
                  </For>
                </div>
              </Show>
            </div>
          </Show>

          <Show when={activeTab() === 'groups'}>
            <div class="space-y-4">
              <Show
                when={groupList().length > 0}
                fallback={
                  <div class="card empty-state">
                    <p class="empty-state-title">暂无模型组</p>
                    <p class="empty-state-description">当前配置尚未定义任何虚拟模型组。</p>
                  </div>
                }
              >
                <For each={groupList()}>
                  {(group: LLMGroupStatus) => (
                    <div class="card card-hover">
                      <div class="flex flex-col xl:flex-row xl:items-start justify-between gap-4 mb-4">
                        <div>
                          <div class="flex items-center gap-2 flex-wrap mb-2">
                            <h2 class="text-lg font-semibold text-white">{group.name}</h2>
                            <span class="badge badge-primary">{group.strategy}</span>
                            <span class="badge badge-gray">Sticky TTL {group.sticky_ttl_seconds}s</span>
                            <span class="badge badge-gray">绑定 {group.sticky_bindings}</span>
                          </div>
                          <p class="text-sm text-dark-400">{group.description || '未填写描述'}</p>
                        </div>
                        <button
                          class="btn btn-secondary btn-sm"
                          disabled={busyGroup() === group.name}
                          onClick={() => handleClearSticky(group.name)}
                        >
                          {busyGroup() === group.name ? '清理中...' : '清理粘性绑定'}
                        </button>
                      </div>

                      <div class="overflow-x-auto rounded-xl border border-dark-600/50">
                        <table class="table">
                          <thead>
                            <tr>
                              <th>渠道</th>
                              <th>模型</th>
                              <th>权重</th>
                              <th>可用</th>
                              <th>状态</th>
                              <th>成功率</th>
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
                  )}
                </For>
              </Show>
            </div>
          </Show>
        </Show>
      </Show>
    </div>
  )
}

export default LLMProxy
