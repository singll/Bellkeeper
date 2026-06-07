import { Component, For, Show, createMemo, createSignal, createResource } from 'solid-js'
import type { LLMChannelStatus, LLMChannelConfig, LLMChannelCredentialView } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import {
  formatDateTime,
  formatPercent,
  getCircuitBadge,
  getCircuitLabel,
  getSuccessRateColor,
  calcRatio,
  deriveTier,
  getTierLabel,
  getTierBadge,
  parseJsonArray,
} from './shared'

type HealthFilter = 'all' | 'closed' | 'open' | 'half_open'
type BillingFilter = 'all' | 'free' | 'paid'

const LLMChannels: Component = () => {
  const toast = useToast()
  const [healthFilter, setHealthFilter] = createSignal<HealthFilter>('all')
  const [billingFilter, setBillingFilter] = createSignal<BillingFilter>('all')
  const [keyword, setKeyword] = createSignal('')
  const [busyChannel, setBusyChannel] = createSignal<string | null>(null)

  const [channelsData, { refetch: refetchStatus }] = createResource(() => llmProxyApi.channelsStatus())
  const [channelConfigsData, { refetch: refetchConfigs }] = createResource(() => llmProxyApi.listChannels())

  const channels = () => channelsData()?.data || []
  const channelConfigs = () => channelConfigsData()?.data || []

  const configMap = createMemo(() => {
    const m: Record<string, LLMChannelConfig> = {}
    channelConfigs().forEach((c) => { m[c.name] = c })
    return m
  })

  const refetchAll = () => Promise.all([refetchStatus(), refetchConfigs()])

  const filteredChannels = createMemo(() => {
    const q = keyword().trim().toLowerCase()
    return channels().filter((channel) => {
      const matchesHealth = healthFilter() === 'all' || channel.health.state === healthFilter()
      const matchesBilling = billingFilter() === 'all' || (billingFilter() === 'free' ? channel.is_free : !channel.is_free)
      const matchesKeyword = q === '' || channel.name.toLowerCase().includes(q) || channel.base_url.toLowerCase().includes(q) || channel.models.some((model) => model.toLowerCase().includes(q))
      return matchesHealth && matchesBilling && matchesKeyword
    })
  })

  const handleResetCircuit = async (name: string) => {
    setBusyChannel(name)
    try {
      const result = await llmProxyApi.resetChannelCircuit(name)
      toast.success(result.message)
      await refetchAll()
    } catch (err) {
      toast.error('重置失败: ' + (err as Error).message)
    } finally {
      setBusyChannel(null)
    }
  }

  const [showChannelModal, setShowChannelModal] = createSignal(false)
  const [editingChannel, setEditingChannel] = createSignal<LLMChannelConfig | null>(null)
  const [saving, setSaving] = createSignal(false)

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
    tier: '',
    task_types: '',
    balance_provider_type: '',
    balance_config_json: '',
    model_rpm_overrides: '',
  })

  const openChannelModal = (ch?: LLMChannelConfig) => {
    if (ch) {
      setEditingChannel(ch)
      setChForm({
        name: ch.name, base_url: ch.base_url, api_key_env: ch.api_key_env,
        provider_type: ch.provider_type || 'openai', rpm: ch.rpm, rpd: ch.rpd,
        priority: ch.priority, is_free: ch.is_free, is_enabled: ch.is_enabled, models: ch.models,
        tier: ch.tier || '', task_types: ch.task_types || '',
        balance_provider_type: ch.balance_provider_type || '',
        balance_config_json: ch.balance_config_json || '',
        model_rpm_overrides: ch.model_rpm_overrides || '',
      })
    } else {
      setEditingChannel(null)
      setChForm({ name: '', base_url: '', api_key_env: '', provider_type: 'openai', rpm: 500, rpd: 50000, priority: 1, is_free: false, is_enabled: true, models: '[]', tier: '', task_types: '', balance_provider_type: '', balance_config_json: '', model_rpm_overrides: '' })
    }
    setShowChannelModal(true)
  }

  const saveChannel = async () => {
    setSaving(true)
    try {
      const form = chForm()
      const data: Record<string, unknown> = { ...form }
      if (editingChannel()) {
        await llmProxyApi.updateChannel(editingChannel()!.id, data)
        toast.success('渠道已更新')
      } else {
        await llmProxyApi.createChannel(data)
        toast.success('渠道已创建')
      }
      setShowChannelModal(false)
      await refetchAll()
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
      await refetchAll()
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    }
  }

  const [showCredModal, setShowCredModal] = createSignal(false)
  const [credChannelId, setCredChannelId] = createSignal<number | null>(null)
  const [credChannelName, setCredChannelName] = createSignal('')
  const [credentialsData, { refetch: refetchCredentials }] = createResource(
    credChannelId,
    (channelId) => llmProxyApi.listChannelCredentials(channelId)
  )
  const credentials = () => credentialsData()?.data || []

  const [credForm, setCredForm] = createSignal({
    purpose: 'api',
    source: 'env' as string,
    env_var_name: '',
    provider_type: '',
    label: '',
    credential: '',
    priority: 0,
  })
  const [editingCred, setEditingCred] = createSignal<LLMChannelCredentialView | null>(null)

  const openCredentialModal = (chId: number, chName: string) => {
    setCredChannelId(chId)
    setCredChannelName(chName)
    setCredForm({ purpose: 'api', source: 'env', env_var_name: '', provider_type: '', label: '', credential: '', priority: 0 })
    setEditingCred(null)
    setShowCredModal(true)
  }

  const openEditCredential = (cred: LLMChannelCredentialView) => {
    setEditingCred(cred)
    setCredForm({
      purpose: cred.purpose,
      source: cred.source,
      env_var_name: cred.env_var_name,
      provider_type: cred.provider_type,
      label: cred.label,
      credential: '',
      priority: cred.priority,
    })
  }

  const cancelEditCredential = () => {
    setEditingCred(null)
    setCredForm({ purpose: 'api', source: 'env', env_var_name: '', provider_type: '', label: '', credential: '', priority: 0 })
  }

  const saveCredential = async () => {
    setSaving(true)
    try {
      const form = credForm()
      if (form.source === 'env' && !form.env_var_name) {
        toast.error('环境变量名不能为空')
        setSaving(false)
        return
      }
      if (form.source === 'direct' && !form.credential && !editingCred()) {
        toast.error('凭证值不能为空')
        setSaving(false)
        return
      }
      if (editingCred()) {
        const data: Record<string, unknown> = {
          purpose: form.purpose,
          source: form.source,
          label: form.label || undefined,
          priority: form.priority || 0,
          status: 'active',
        }
        if (form.source === 'env') data.env_var_name = form.env_var_name
        if (form.provider_type) data.provider_type = form.provider_type
        if (form.source === 'direct' && form.credential) data.credential = form.credential
        await llmProxyApi.updateChannelCredential(editingCred()!.id, data as any)
        toast.success('凭证已更新')
        setEditingCred(null)
        setCredForm({ purpose: 'api', source: 'env', env_var_name: '', provider_type: '', label: '', credential: '', priority: 0 })
      } else {
        const chId = credChannelId()
        if (!chId) return
        await llmProxyApi.createChannelCredential(chId, {
          purpose: form.purpose,
          source: form.source,
          env_var_name: form.source === 'env' ? form.env_var_name : undefined,
          provider_type: form.provider_type || undefined,
          label: form.label || undefined,
          credential: form.source === 'direct' ? form.credential : undefined,
          priority: form.priority || undefined,
        })
        toast.success('凭证已添加')
        setCredForm({ purpose: 'api', source: 'env', env_var_name: '', provider_type: '', label: '', credential: '', priority: 0 })
      }
      await refetchCredentials()
    } catch (err) {
      toast.error('保存失败: ' + (err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const deleteCredential = async (credId: number) => {
    if (!confirm('确定删除此凭证？')) return
    try {
      await llmProxyApi.deleteChannelCredential(credId)
      toast.success('凭证已删除')
      await refetchCredentials()
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    }
  }

  const renderProgress = (current: number, total: number, label: string) => {
    const ratio = calcRatio(current, total)
    return (
      <div>
        <div class="flex items-center justify-between text-xs text-dark-400 mb-1.5">
          <span>{label}</span>
          <span>{current} / {total || '--'}</span>
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

  const baseUrlPlaceholder = () => {
    const pt = chForm().provider_type
    if (pt === 'anthropic') return '如 https://api.anthropic.com (不含 /v1)'
    return '如 https://api.siliconflow.cn (不含 /v1)'
  }

  return (
    <div class="animate-fade-in">
      <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">渠道管理</h1>
          <p class="text-sm text-dark-400 mt-1">查看运行状态、管理配置与凭证</p>
        </div>
        <button class="btn btn-primary btn-sm" onClick={() => openChannelModal()}>
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          新增渠道
        </button>
      </div>

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
              <input class="input" type="text" value={keyword()} onInput={(e) => setKeyword(e.currentTarget.value)} placeholder="渠道名 / 模型名 / URL" />
            </div>
          </div>
        </div>

        <Show when={!channelsData.loading && !channelConfigsData.loading} fallback={
          <div class="card py-12">
            <div class="flex items-center justify-center">
              <div class="loading-spinner" />
              <span class="ml-3 text-dark-400">加载渠道数据...</span>
            </div>
          </div>
        }>
          <Show
            when={filteredChannels().length > 0}
            fallback={
              <div class="card empty-state">
                <p class="empty-state-title">没有匹配的渠道</p>
                <p class="empty-state-description">请调整筛选条件或新增渠道。</p>
              </div>
            }
          >
            <div class="space-y-4">
              <For each={filteredChannels()}>
                {(channel: LLMChannelStatus) => {
                  const tokenRatio = calcRatio(channel.available_tokens, channel.max_tokens)
                  const dailyRatio = calcRatio(channel.daily_used, channel.daily_limit)
                  const cfg = () => configMap()[channel.name]
                  const tier = () => deriveTier(cfg()?.tier, channel.is_free)
                  const taskTypes = () => parseJsonArray(cfg()?.task_types)
                  return (
                    <div class="card card-hover">
                      <div class="flex flex-col xl:flex-row xl:items-start justify-between gap-4 mb-4">
                        <div>
                          <div class="flex flex-wrap items-center gap-2 mb-2">
                            <h2 class="text-lg font-semibold text-white">{channel.name}</h2>
                            <span class={`badge ${getTierBadge(tier())}`}>{getTierLabel(tier())}层</span>
                            <span class={`badge ${channel.is_free ? 'badge-primary' : 'badge-gray'}`}>{channel.is_free ? '免费' : '付费'}</span>
                            <span class={`badge ${getCircuitBadge(channel.health.state)}`}>{getCircuitLabel(channel.health.state)}</span>
                            <span class="badge badge-gray">优先级 {channel.priority}</span>
                          </div>
                          <div class="text-sm text-dark-400 break-all">{channel.base_url}</div>
                          <div class="flex flex-wrap gap-2 mt-3">
                            <For each={channel.models}>{(model) => <span class="badge badge-gray">{model}</span>}</For>
                          </div>
                          <Show when={taskTypes().length > 0}>
                            <div class="flex flex-wrap gap-1.5 mt-2">
                              <For each={taskTypes()}>
                                {(tt) => <span class="inline-flex px-2 py-0.5 rounded-md bg-primary-500/10 text-xs text-primary-300">{tt}</span>}
                              </For>
                            </div>
                          </Show>
                        </div>
                        <div class="flex items-center gap-2 flex-shrink-0">
                          <Show when={cfg()}>
                            <button class="btn btn-ghost btn-sm" onClick={() => openCredentialModal(cfg()!.id, channel.name)}>凭证</button>
                            <button class="btn btn-ghost btn-sm" onClick={() => openChannelModal(cfg())}>编辑</button>
                            <button class="btn btn-ghost btn-sm text-red-400" onClick={() => deleteChannel(cfg()!.id)}>删除</button>
                          </Show>
                          <button class="btn btn-secondary btn-sm" disabled={busyChannel() === channel.name} onClick={() => handleResetCircuit(channel.name)}>
                            {busyChannel() === channel.name ? '重置中...' : '重置熔断器'}
                          </button>
                        </div>
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
                          <Show when={cfg()?.balance_provider_type}>
                            <div class="p-3 bg-dark-700/40 rounded-xl text-sm">
                              <div class="text-dark-400 mb-1">余额供应商</div>
                              <div class="font-medium text-white">{cfg()!.balance_provider_type}</div>
                            </div>
                          </Show>
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
        </Show>
      </div>

      <Modal open={showChannelModal()} onClose={() => setShowChannelModal(false)} title={editingChannel() ? '编辑渠道' : '新增渠道'} size="xl">
        <div class="space-y-4">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div><label class="label">名称</label><input class="input" value={chForm().name} onInput={(e) => setChForm((p) => ({ ...p, name: e.currentTarget.value }))} placeholder="如 siliconflow" /></div>
            <div><label class="label">供应商类型</label><select class="input" value={chForm().provider_type} onChange={(e) => setChForm((p) => ({ ...p, provider_type: e.currentTarget.value }))}><option value="openai">OpenAI 兼容</option><option value="anthropic">Anthropic (Claude)</option></select></div>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div><label class="label">Base URL</label><input class="input" value={chForm().base_url} onInput={(e) => setChForm((p) => ({ ...p, base_url: e.currentTarget.value }))} placeholder={baseUrlPlaceholder()} /></div>
            <div class="flex items-center">
              <div class="p-3 rounded-lg bg-dark-700/40 border border-dark-600/50 text-xs text-dark-400 w-full">
                <span class="text-dark-200 font-medium">API Key 管理：</span>密钥已统一至凭证区管理，请点击渠道卡片上的「凭证」按钮添加/编辑。
              </div>
            </div>
          </div>
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <div><label class="label">RPM</label><input class="input" type="number" value={chForm().rpm} onInput={(e) => setChForm((p) => ({ ...p, rpm: parseInt(e.currentTarget.value) || 0 }))} /></div>
            <div><label class="label">RPD</label><input class="input" type="number" value={chForm().rpd} onInput={(e) => setChForm((p) => ({ ...p, rpd: parseInt(e.currentTarget.value) || 0 }))} /></div>
            <div><label class="label">优先级</label><input class="input" type="number" value={chForm().priority} onInput={(e) => setChForm((p) => ({ ...p, priority: parseInt(e.currentTarget.value) || 1 }))} /></div>
            <div class="flex items-end gap-4">
              <label class="flex items-center gap-2 cursor-pointer"><input type="checkbox" checked={chForm().is_free} onChange={(e) => setChForm((p) => ({ ...p, is_free: e.currentTarget.checked }))} class="w-4 h-4 rounded" /><span class="text-sm text-dark-200">免费</span></label>
              <label class="flex items-center gap-2 cursor-pointer"><input type="checkbox" checked={chForm().is_enabled} onChange={(e) => setChForm((p) => ({ ...p, is_enabled: e.currentTarget.checked }))} class="w-4 h-4 rounded" /><span class="text-sm text-dark-200">启用</span></label>
            </div>
          </div>
          <div><label class="label">模型列表 (JSON 数组)</label><textarea class="input min-h-[80px]" value={chForm().models} onInput={(e) => setChForm((p) => ({ ...p, models: e.currentTarget.value }))} placeholder='["Qwen/Qwen3-8B", "Qwen/Qwen2.5-7B-Instruct"]' /></div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">层级 (tier)</label>
              <select class="input" value={chForm().tier} onChange={(e) => setChForm((p) => ({ ...p, tier: e.currentTarget.value }))}>
                <option value="">自动 (由免费字段派生)</option>
                <option value="free">free</option>
                <option value="standard">standard</option>
                <option value="premium">premium</option>
              </select>
            </div>
            <div>
              <label class="label">任务类型 (JSON 数组)</label>
              <input class="input" value={chForm().task_types} onInput={(e) => setChForm((p) => ({ ...p, task_types: e.currentTarget.value }))} placeholder='["coding", "analysis"]' />
            </div>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">余额供应商类型</label>
              <input class="input" value={chForm().balance_provider_type} onInput={(e) => setChForm((p) => ({ ...p, balance_provider_type: e.currentTarget.value }))} placeholder="如 siliconflow、openrouter" />
            </div>
            <div>
              <label class="label">余额配置 (非密参数 JSON)</label>
              <input class="input" value={chForm().balance_config_json} onInput={(e) => setChForm((p) => ({ ...p, balance_config_json: e.currentTarget.value }))} placeholder='{"api_url": "..."}  — 密钥请用凭证区管理' />
            </div>
          </div>
          <div>
            <label class="label">模型 RPM 覆盖 (JSON: model→rpm)</label>
            <input class="input" value={chForm().model_rpm_overrides} onInput={(e) => setChForm((p) => ({ ...p, model_rpm_overrides: e.currentTarget.value }))} placeholder='{"deepseek-chat": 10, "Qwen/Qwen3-8B": 20}' />
          </div>
          <div class="flex justify-end gap-3 pt-2">
            <button class="btn btn-secondary" onClick={() => setShowChannelModal(false)}>取消</button>
            <button class="btn btn-primary" disabled={saving()} onClick={saveChannel}>{saving() ? '保存中...' : '保存'}</button>
          </div>
        </div>
      </Modal>

      <Modal open={showCredModal()} onClose={() => setShowCredModal(false)} title={`${credChannelName()} — 凭证管理`} size="lg">
        <div class="space-y-4">
          <div class="p-3 rounded-lg bg-dark-700/40 border border-dark-600/50 text-xs text-dark-400">
            <span class="text-dark-200 font-medium">安全说明：</span>凭证在服务端加密存储（AES-256-GCM），API 仅返回已掩盖的预览。可管理多个用途×来源的凭证。
          </div>

          <div>
            <label class="label">现有凭证</label>
            <Show
              when={credentials().length > 0}
              fallback={<p class="text-sm text-dark-500">暂无凭证。</p>}
            >
              <div class="space-y-2">
                <For each={credentials()}>
                  {(cred: LLMChannelCredentialView) => (
                    <div class="flex items-center justify-between p-3 rounded-lg bg-dark-700/30 border border-dark-600/50">
                      <div class="flex-1">
                        <div class="flex items-center gap-2 mb-1">
                          <span class="badge badge-primary">{cred.purpose}</span>
                          <span class={`badge ${cred.source === 'env' ? 'badge-success' : 'badge-warning'}`}>{cred.source === 'env' ? '环境变量' : '直接值'}</span>
                          <Show when={cred.label}><span class="text-xs text-dark-300">{cred.label}</span></Show>
                          <span class={`badge ${cred.status === 'active' ? 'badge-success' : cred.status === 'error' ? 'badge-danger' : 'badge-warning'}`}>{cred.status}</span>
                          <Show when={cred.priority > 0}><span class="text-xs text-dark-500">P{cred.priority}</span></Show>
                        </div>
                        <div class="text-xs text-dark-400">
                          <Show when={cred.source === 'env'} fallback={<span>预览: {cred.credential_preview}</span>}>
                            <span class="font-mono">${cred.env_var_name}</span>
                            <Show when={cred.env_var_resolved}><span class="ml-2 text-green-400">✓ 已解析</span></Show>
                            <Show when={!cred.env_var_resolved}><span class="ml-2 text-red-400">✗ 未配置</span></Show>
                          </Show>
                          <Show when={cred.provider_type}><span class="mx-2">•</span><span>{cred.provider_type}</span></Show>
                        </div>
                        <Show when={cred.error_message}><div class="text-xs text-red-400 mt-1">{cred.error_message}</div></Show>
                      </div>
                       <div class="flex items-center gap-2">
                         <button class="btn btn-ghost btn-sm" onClick={() => openEditCredential(cred)}>编辑</button>
                         <button class="btn btn-ghost btn-sm text-red-400" onClick={() => deleteCredential(cred.id)}>删除</button>
                       </div>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>

          <Show when={editingCred()}>
            <div class="border-t border-dark-600/30 pt-4">
              <label class="label mb-3">编辑凭证 <span class="text-dark-400 font-normal">({editingCred()!.label || editingCred()!.purpose})</span></label>
              <div class="space-y-3">
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
                  <div>
                    <label class="label text-sm">用途</label>
                    <select class="input" value={credForm().purpose} onChange={(e) => setCredForm((p) => ({ ...p, purpose: e.currentTarget.value }))}>
                      <option value="api">转发(api)</option>
                      <option value="balance">余额(balance)</option>
                    </select>
                  </div>
                  <div>
                    <label class="label text-sm">来源</label>
                    <select class="input" value={credForm().source} onChange={(e) => setCredForm((p) => ({ ...p, source: e.currentTarget.value }))}>
                      <option value="env">环境变量</option>
                      <option value="direct">直接值</option>
                    </select>
                  </div>
                  <div>
                    <label class="label text-sm">优先级</label>
                    <input class="input" type="number" value={credForm().priority} onInput={(e) => setCredForm((p) => ({ ...p, priority: parseInt(e.currentTarget.value) || 0 }))} placeholder="0 = 默认" />
                  </div>
                </div>
                <Show when={credForm().source === 'env'}>
                  <div>
                    <label class="label text-sm">环境变量名</label>
                    <input class="input" value={credForm().env_var_name} onInput={(e) => setCredForm((p) => ({ ...p, env_var_name: e.currentTarget.value }))} placeholder="如 LLM_SILICONFLOW_API_KEY" />
                  </div>
                </Show>
                <Show when={credForm().source === 'direct'}>
                  <div>
                    <label class="label text-sm">新凭证值 (留空则不变)</label>
                    <textarea class="input min-h-[60px]" value={credForm().credential} onInput={(e) => setCredForm((p) => ({ ...p, credential: e.currentTarget.value }))} placeholder="输入新值以轮换，留空保持原值" />
                  </div>
                </Show>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label class="label text-sm">标签</label>
                    <input class="input" value={credForm().label} onInput={(e) => setCredForm((p) => ({ ...p, label: e.currentTarget.value }))} placeholder="如 主 Key、备用 Key" />
                  </div>
                  <div>
                    <label class="label text-sm">供应商类型</label>
                    <input class="input" value={credForm().provider_type} onInput={(e) => setCredForm((p) => ({ ...p, provider_type: e.currentTarget.value }))} placeholder="如 deepseek、moonshot" />
                  </div>
                </div>
              </div>
            </div>
          </Show>

          <Show when={!editingCred()}>
            <div class="border-t border-dark-600/30 pt-4">
              <label class="label mb-3">添加新凭证</label>
              <div class="space-y-3">
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
                  <div>
                    <label class="label text-sm">用途</label>
                    <select class="input" value={credForm().purpose} onChange={(e) => setCredForm((p) => ({ ...p, purpose: e.currentTarget.value }))}>
                      <option value="api">转发(api)</option>
                      <option value="balance">余额(balance)</option>
                    </select>
                  </div>
                  <div>
                    <label class="label text-sm">来源</label>
                    <select class="input" value={credForm().source} onChange={(e) => setCredForm((p) => ({ ...p, source: e.currentTarget.value }))}>
                      <option value="env">环境变量</option>
                      <option value="direct">直接值</option>
                    </select>
                  </div>
                  <div>
                    <label class="label text-sm">优先级</label>
                    <input class="input" type="number" value={credForm().priority} onInput={(e) => setCredForm((p) => ({ ...p, priority: parseInt(e.currentTarget.value) || 0 }))} placeholder="0 = 默认" />
                  </div>
                </div>
                <Show when={credForm().source === 'env'}>
                  <div>
                    <label class="label text-sm">环境变量名</label>
                    <input class="input" value={credForm().env_var_name} onInput={(e) => setCredForm((p) => ({ ...p, env_var_name: e.currentTarget.value }))} placeholder="如 LLM_SILICONFLOW_API_KEY" />
                  </div>
                </Show>
                <Show when={credForm().source === 'direct'}>
                  <div>
                    <label class="label text-sm">凭证值 (将被加密)</label>
                    <textarea class="input min-h-[60px]" value={credForm().credential} onInput={(e) => setCredForm((p) => ({ ...p, credential: e.currentTarget.value }))} placeholder="API Key 或凭证 JSON" />
                  </div>
                </Show>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label class="label text-sm">标签 (可选)</label>
                    <input class="input" value={credForm().label} onInput={(e) => setCredForm((p) => ({ ...p, label: e.currentTarget.value }))} placeholder="如 主 Key、备用 Key" />
                  </div>
                  <div>
                    <label class="label text-sm">供应商类型 (可选)</label>
                    <input class="input" value={credForm().provider_type} onInput={(e) => setCredForm((p) => ({ ...p, provider_type: e.currentTarget.value }))} placeholder="如 deepseek、moonshot" />
                  </div>
                </div>
              </div>
            </div>
          </Show>

          <div class="flex justify-end gap-3 pt-2">
            <Show when={editingCred()}>
              <button class="btn btn-secondary" onClick={cancelEditCredential}>取消编辑</button>
            </Show>
            <Show when={!editingCred()}>
              <button class="btn btn-secondary" onClick={() => setShowCredModal(false)}>关闭</button>
            </Show>
            <button class="btn btn-primary" disabled={saving()} onClick={saveCredential}>{saving() ? '保存中...' : editingCred() ? '更新凭证' : '添加凭证'}</button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default LLMChannels
