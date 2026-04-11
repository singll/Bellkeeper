import { Component, For, Show, createMemo, createSignal } from 'solid-js'
import type { LLMChannelStatus } from '@/types'
import type { HealthFilter, BillingFilter } from '@/types'
import { useToast } from '@/components/Toast'
import { llmProxyApi } from '@/api'
import {
  formatDateTime,
  formatPercent,
  getCircuitBadge,
  getCircuitLabel,
  getCircuitDot,
  getSuccessRateColor,
  calcRatio,
} from './shared'

interface LLMProxyChannelsProps {
  channels: LLMChannelStatus[]
  onRefreshChannels: () => Promise<void>
}

type LocalHealthFilter = 'all' | 'closed' | 'open' | 'half_open'
type LocalBillingFilter = 'all' | 'free' | 'paid'

const LLMProxyChannels: Component<LLMProxyChannelsProps> = (props) => {
  const toast = useToast()
  const [healthFilter, setHealthFilter] = createSignal<LocalHealthFilter>('all')
  const [billingFilter, setBillingFilter] = createSignal<LocalBillingFilter>('all')
  const [keyword, setKeyword] = createSignal('')
  const [busyChannel, setBusyChannel] = createSignal<string | null>(null)

  const filteredChannels = createMemo(() => {
    const q = keyword().trim().toLowerCase()

    return props.channels.filter((channel) => {
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
      await props.onRefreshChannels()
    } catch (err) {
      toast.error('重置失败: ' + (err as Error).message)
    } finally {
      setBusyChannel(null)
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
            class={`h-full rounded-full transition-all ${ratio >= 0.7 ? 'bg-emerald-500' : ratio >= 0.35 ? 'bg-amber-500' : 'bg-red-500'}`}
            style={{ width: `${Math.max(ratio * 100, ratio > 0 ? 6 : 0)}%` }}
          />
        </div>
      </div>
    )
  }

  const renderHealthMeta = (health: LLMChannelStatus['health']) => (
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
    <div class="space-y-6">
      {/* Filters */}
      <div class="card">
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label class="label">状态筛选</label>
            <select class="input" value={healthFilter()} onChange={(e) => setHealthFilter(e.currentTarget.value as LocalHealthFilter)}>
              <option value="all">全部状态</option>
              <option value="closed">正常</option>
              <option value="half_open">半开</option>
              <option value="open">熔断</option>
            </select>
          </div>
          <div>
            <label class="label">计费类型</label>
            <select class="input" value={billingFilter()} onChange={(e) => setBillingFilter(e.currentTarget.value as LocalBillingFilter)}>
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

      {/* Channel List */}
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
  )
}

export default LLMProxyChannels
