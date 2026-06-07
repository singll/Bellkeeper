import { Component, For, Show, createSignal, createResource, createMemo } from 'solid-js'
import type { LLMToken, LLMTokenUsageDaily, LLMModelPricing, LLMUsageByModel } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import { formatCents, formatDateShort } from './shared'

const LLMUsageAndBilling: Component = () => {
  const toast = useToast()

  const [groupBy, setGroupBy] = createSignal('date')
  const [fromDate, setFromDate] = createSignal('')
  const [toDate, setToDate] = createSignal('')

  const [usageData, { refetch: refetchUsage }] = createResource(
    () => ({ g: groupBy(), f: fromDate(), t: toDate() }),
    ({ g, f, t }) => llmProxyApi.getUsage(g, f || undefined, t || undefined)
  )
  const rows = () => usageData()?.data || []

  const totalCost = () => rows().reduce((sum: number, r: LLMUsageByModel) => sum + (r.cost_cents || 0), 0)
  const totalRequests = () => rows().reduce((sum: number, r: LLMUsageByModel) => sum + (r.requests || 0), 0)
  const totalTokens = () => rows().reduce((sum: number, r: LLMUsageByModel) => sum + (r.prompt_tokens || 0) + (r.completion_tokens || 0), 0)

  const [pricingData, { refetch: refetchPricing }] = createResource(() => llmProxyApi.listPricing())
  const pricing = () => pricingData()?.data || []

  const [tokensData, { refetch: refetchTokens }] = createResource(() => llmProxyApi.listTokens())
  const tokens = () => tokensData()?.data || []

  const [showPricingModal, setShowPricingModal] = createSignal(false)
  const [editingPricing, setEditingPricing] = createSignal<LLMModelPricing | null>(null)
  const [showTokenModal, setShowTokenModal] = createSignal(false)
  const [editingToken, setEditingToken] = createSignal<LLMToken | null>(null)
  const [showKeyModal, setShowKeyModal] = createSignal(false)
  const [newKey, setNewKey] = createSignal('')
  const [saving, setSaving] = createSignal(false)

  const [pricingForm, setPricingForm] = createSignal({
    channel_name: '',
    model: '',
    input_price_per_1m_cents: 0,
    output_price_per_1m_cents: 0,
    cached_input_price_per_1m_cents: 0,
    currency: 'USD',
    notes: '',
  })

  const [tokenForm, setTokenForm] = createSignal({
    name: '',
    caller_id: '',
    allowed_models: '',
    allowed_groups: '',
    quota_requests_daily: 0,
    quota_tokens_daily: 0,
    quota_cost_monthly_cents: 0,
    expires_at: '',
  })

  const openPricingCreate = () => {
    setEditingPricing(null)
    setPricingForm({ channel_name: '', model: '', input_price_per_1m_cents: 0, output_price_per_1m_cents: 0, cached_input_price_per_1m_cents: 0, currency: 'USD', notes: '' })
    setShowPricingModal(true)
  }

  const openPricingEdit = (p: LLMModelPricing) => {
    setEditingPricing(p)
    setPricingForm({
      channel_name: p.channel_name, model: p.model,
      input_price_per_1m_cents: p.input_price_per_1m_cents,
      output_price_per_1m_cents: p.output_price_per_1m_cents,
      cached_input_price_per_1m_cents: p.cached_input_price_per_1m_cents,
      currency: p.currency, notes: p.notes,
    })
    setShowPricingModal(true)
  }

  const savePricing = async () => {
    setSaving(true)
    try {
      const f = pricingForm()
      const payload: Record<string, unknown> = { ...f }
      if (editingPricing()) {
        await llmProxyApi.updatePricing(editingPricing()!.id, payload)
        toast.success('定价已更新')
      } else {
        await llmProxyApi.createPricing(payload)
        toast.success('定价已创建')
      }
      setShowPricingModal(false)
      refetchPricing()
    } catch (err) {
      toast.error((err instanceof Error ? err.message : String(err)) || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const deletePricing = async (id: number) => {
    if (!confirm('确定删除此定价规则？')) return
    try {
      await llmProxyApi.deletePricing(id)
      toast.success('定价已删除')
      refetchPricing()
    } catch (err) {
      toast.error((err instanceof Error ? err.message : String(err)) || '删除失败')
    }
  }

  const openTokenCreate = () => {
    setEditingToken(null)
    setTokenForm({ name: '', caller_id: '', allowed_models: '', allowed_groups: '', quota_requests_daily: 0, quota_tokens_daily: 0, quota_cost_monthly_cents: 0, expires_at: '' })
    setShowTokenModal(true)
  }

  const openTokenEdit = (t: LLMToken) => {
    setEditingToken(t)
    setTokenForm({
      name: t.name, caller_id: t.caller_id,
      allowed_models: t.allowed_models, allowed_groups: t.allowed_groups,
      quota_requests_daily: t.quota_requests_daily, quota_tokens_daily: t.quota_tokens_daily,
      quota_cost_monthly_cents: t.quota_cost_monthly_cents, expires_at: t.expires_at || '',
    })
    setShowTokenModal(true)
  }

  const saveToken = async () => {
    setSaving(true)
    try {
      const f = tokenForm()
      const payload: Record<string, unknown> = {
        name: f.name, caller_id: f.caller_id,
        allowed_models: f.allowed_models ? JSON.parse(f.allowed_models) : [],
        allowed_groups: f.allowed_groups ? JSON.parse(f.allowed_groups) : [],
        quota_requests_daily: f.quota_requests_daily, quota_tokens_daily: f.quota_tokens_daily,
        quota_cost_monthly_cents: f.quota_cost_monthly_cents,
      }
      if (f.expires_at) payload.expires_at = new Date(f.expires_at).toISOString()

      if (editingToken()) {
        await llmProxyApi.updateToken(editingToken()!.id, payload)
        toast.success('Token 已更新')
      } else {
        const res = await llmProxyApi.createToken(payload)
        setNewKey(res.key)
        setShowKeyModal(true)
        toast.success('Token 已创建')
      }
      setShowTokenModal(false)
      refetchTokens()
    } catch (err) {
      toast.error((err instanceof Error ? err.message : String(err)) || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const deleteToken = async (id: number) => {
    if (!confirm('确定删除此 Token？')) return
    try {
      await llmProxyApi.deleteToken(id)
      toast.success('Token 已删除')
      refetchTokens()
    } catch (err) {
      toast.error((err instanceof Error ? err.message : String(err)) || '删除失败')
    }
  }

  const regenerateKey = async (id: number) => {
    if (!confirm('重新生成 API Key？旧 Key 将立即失效。')) return
    try {
      const res = await llmProxyApi.regenerateTokenKey(id)
      setNewKey(res.key)
      setShowKeyModal(true)
      toast.success('Key 已重新生成')
      refetchTokens()
    } catch (err) {
      toast.error((err instanceof Error ? err.message : String(err)) || '重新生成失败')
    }
  }

  const refetchAll = () => Promise.all([refetchUsage(), refetchPricing(), refetchTokens()])

  return (
    <div class="animate-fade-in">
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">用量与计费</h1>
        <p class="text-sm text-dark-400 mt-1">用量统计、定价配置与 Token 管理</p>
      </div>

      <div class="space-y-6">
        <div class="card">
          <div class="flex flex-wrap items-center justify-between gap-4 mb-4">
            <h2 class="text-lg font-semibold text-white">用量统计</h2>
            <div class="flex flex-wrap items-center gap-2">
              <select class="input w-auto" value={groupBy()} onChange={(e) => setGroupBy(e.currentTarget.value)}>
                <option value="date">按日期</option>
                <option value="token">按 Token</option>
                <option value="model">按模型</option>
              </select>
              <input type="date" class="input w-auto" value={fromDate()} onChange={(e) => setFromDate(e.currentTarget.value)} />
              <input type="date" class="input w-auto" value={toDate()} onChange={(e) => setToDate(e.currentTarget.value)} />
              <button class="btn btn-primary btn-sm" onClick={() => refetchUsage()}>查询</button>
            </div>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-4">
            <div class="p-4 bg-dark-700/40 rounded-xl">
              <div class="text-sm text-dark-400">总请求数</div>
              <div class="text-2xl font-bold text-white mt-1">{totalRequests().toLocaleString()}</div>
            </div>
            <div class="p-4 bg-dark-700/40 rounded-xl">
              <div class="text-sm text-dark-400">总 Token 数</div>
              <div class="text-2xl font-bold text-white mt-1">{totalTokens().toLocaleString()}</div>
            </div>
            <div class="p-4 bg-dark-700/40 rounded-xl">
              <div class="text-sm text-dark-400">总成本</div>
              <div class="text-2xl font-bold text-white mt-1">{formatCents(totalCost())}</div>
            </div>
          </div>

          <Show
            when={rows().length > 0}
            fallback={<div class="text-sm text-dark-500 py-4">暂无用量数据。</div>}
          >
            <div class="overflow-x-auto rounded-xl border border-dark-600/50">
              <table class="table">
                <thead>
                  <tr>
                    <th>维度</th><th>请求数</th><th>Prompt</th><th>Completion</th><th>Cached</th><th>错误</th><th class="text-right">成本</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={rows()}>
                    {(r: LLMUsageByModel) => (
                      <tr>
                        <td class="font-medium text-white">{r.date || r.token_id || r.model || '-'}</td>
                        <td class="text-sm text-dark-300">{(r.requests || 0).toLocaleString()}</td>
                        <td class="text-sm text-dark-300">{(r.prompt_tokens || 0).toLocaleString()}</td>
                        <td class="text-sm text-dark-300">{(r.completion_tokens || 0).toLocaleString()}</td>
                        <td class="text-sm text-dark-300">{(r.cached_tokens || 0).toLocaleString()}</td>
                        <td class="text-sm text-dark-300">{(r.error_count || 0).toLocaleString()}</td>
                        <td class="text-sm text-right font-medium text-white">{formatCents(r.cost_cents || 0)}</td>
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
            <h2 class="text-lg font-semibold text-white">模型定价</h2>
            <button class="btn btn-primary btn-sm" onClick={openPricingCreate}>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
              </svg>
              新增定价
            </button>
          </div>
          <Show
            when={pricing().length > 0}
            fallback={<div class="text-sm text-dark-500 py-4">暂无定价配置。</div>}
          >
            <div class="overflow-x-auto rounded-xl border border-dark-600/50">
              <table class="table">
                <thead>
                  <tr><th>渠道</th><th>模型</th><th>Input / 1M</th><th>Output / 1M</th><th>Cached / 1M</th><th>货币</th><th class="text-right">操作</th></tr>
                </thead>
                <tbody>
                  <For each={pricing()}>
                    {(p) => (
                      <tr>
                        <td class="font-medium text-white">{p.channel_name}</td>
                        <td class="font-mono text-xs text-dark-300">{p.model}</td>
                        <td class="text-sm text-dark-300">{p.input_price_per_1m_cents} ¢</td>
                        <td class="text-sm text-dark-300">{p.output_price_per_1m_cents} ¢</td>
                        <td class="text-sm text-dark-300">{p.cached_input_price_per_1m_cents > 0 ? p.cached_input_price_per_1m_cents + ' ¢' : 'auto'}</td>
                        <td class="text-sm text-dark-300">{p.currency}</td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-2">
                            <button class="btn btn-ghost btn-sm" onClick={() => openPricingEdit(p)}>编辑</button>
                            <button class="btn btn-ghost btn-sm text-red-400" onClick={() => deletePricing(p.id)}>删除</button>
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

        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">API Token 管理</h2>
            <button class="btn btn-primary btn-sm" onClick={openTokenCreate}>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
              </svg>
              新增 Token
            </button>
          </div>
          <Show
            when={tokens().length > 0}
            fallback={<div class="text-sm text-dark-500 py-4">暂无 Token。</div>}
          >
            <div class="overflow-x-auto rounded-xl border border-dark-600/50">
              <table class="table">
                <thead>
                  <tr><th>名称</th><th>Key 前缀</th><th>Caller ID</th><th>配额 (req/tok/$)</th><th>过期</th><th>状态</th><th class="text-right">操作</th></tr>
                </thead>
                <tbody>
                  <For each={tokens()}>
                    {(t) => (
                      <tr>
                        <td class="font-medium text-white">{t.name}</td>
                        <td class="font-mono text-xs text-dark-400">{t.key_prefix}***</td>
                        <td class="text-sm text-dark-300">{t.caller_id}</td>
                        <td class="text-sm text-dark-300">
                          {t.quota_requests_daily > 0 ? t.quota_requests_daily : '∞'} /
                          {t.quota_tokens_daily > 0 ? t.quota_tokens_daily : '∞'} /
                          {t.quota_cost_monthly_cents > 0 ? formatCents(t.quota_cost_monthly_cents) : '∞'}
                        </td>
                        <td class="text-sm text-dark-300">{t.expires_at ? formatDateShort(t.expires_at) : '永不过期'}</td>
                        <td>
                          <span class={`badge ${t.enabled ? 'badge-success' : 'badge-gray'}`}>
                            {t.enabled ? '启用' : '禁用'}
                          </span>
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-2">
                            <button class="btn btn-ghost btn-sm" onClick={() => openTokenEdit(t)}>编辑</button>
                            <button class="btn btn-ghost btn-sm text-amber-400" onClick={() => regenerateKey(t.id)}>重置 Key</button>
                            <button class="btn btn-ghost btn-sm text-red-400" onClick={() => deleteToken(t.id)}>删除</button>
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
      </div>

      <Modal open={showPricingModal()} onClose={() => setShowPricingModal(false)} title={editingPricing() ? '编辑定价' : '新增定价'}>
        <div class="space-y-4">
          <div class="grid grid-cols-2 gap-3">
            <div><label class="label">渠道</label><input class="input" value={pricingForm().channel_name} onInput={(e) => setPricingForm((p) => ({ ...p, channel_name: e.currentTarget.value }))} /></div>
            <div><label class="label">模型</label><input class="input" value={pricingForm().model} onInput={(e) => setPricingForm((p) => ({ ...p, model: e.currentTarget.value }))} /></div>
          </div>
          <div class="grid grid-cols-3 gap-3">
            <div><label class="label">Input / 1M (¢)</label><input type="number" class="input" value={pricingForm().input_price_per_1m_cents} onInput={(e) => setPricingForm((p) => ({ ...p, input_price_per_1m_cents: parseInt(e.currentTarget.value) || 0 }))} /></div>
            <div><label class="label">Output / 1M (¢)</label><input type="number" class="input" value={pricingForm().output_price_per_1m_cents} onInput={(e) => setPricingForm((p) => ({ ...p, output_price_per_1m_cents: parseInt(e.currentTarget.value) || 0 }))} /></div>
            <div><label class="label">Cached / 1M (¢)</label><input type="number" class="input" placeholder="0 = auto" value={pricingForm().cached_input_price_per_1m_cents} onInput={(e) => setPricingForm((p) => ({ ...p, cached_input_price_per_1m_cents: parseInt(e.currentTarget.value) || 0 }))} /></div>
          </div>
          <div><label class="label">备注</label><input class="input" value={pricingForm().notes} onInput={(e) => setPricingForm((p) => ({ ...p, notes: e.currentTarget.value }))} /></div>
          <div class="flex justify-end gap-3 pt-2">
            <button class="btn btn-secondary" onClick={() => setShowPricingModal(false)}>取消</button>
            <button class="btn btn-primary" disabled={saving()} onClick={savePricing}>{saving() ? '保存中...' : editingPricing() ? '更新' : '创建'}</button>
          </div>
        </div>
      </Modal>

      <Modal open={showTokenModal()} onClose={() => setShowTokenModal(false)} title={editingToken() ? '编辑 Token' : '新增 Token'} size="lg">
        <div class="space-y-4">
          <div><label class="label">名称</label><input class="input" value={tokenForm().name} onInput={(e) => setTokenForm((p) => ({ ...p, name: e.currentTarget.value }))} /></div>
          <Show when={!editingToken()}>
            <div><label class="label">Caller ID</label><input class="input" value={tokenForm().caller_id} onInput={(e) => setTokenForm((p) => ({ ...p, caller_id: e.currentTarget.value }))} /></div>
          </Show>
          <div><label class="label">允许模型 (JSON 数组)</label><input class="input font-mono text-sm" placeholder='["gpt-4", "claude-sonnet"]' value={tokenForm().allowed_models} onInput={(e) => setTokenForm((p) => ({ ...p, allowed_models: e.currentTarget.value }))} /></div>
          <div><label class="label">允许组 (JSON 数组)</label><input class="input font-mono text-sm" placeholder='["pool-chat-free"]' value={tokenForm().allowed_groups} onInput={(e) => setTokenForm((p) => ({ ...p, allowed_groups: e.currentTarget.value }))} /></div>
          <div class="grid grid-cols-3 gap-3">
            <div><label class="label">每日请求数</label><input type="number" class="input" value={tokenForm().quota_requests_daily} onInput={(e) => setTokenForm((p) => ({ ...p, quota_requests_daily: parseInt(e.currentTarget.value) || 0 }))} /></div>
            <div><label class="label">每日 Token</label><input type="number" class="input" value={tokenForm().quota_tokens_daily} onInput={(e) => setTokenForm((p) => ({ ...p, quota_tokens_daily: parseInt(e.currentTarget.value) || 0 }))} /></div>
            <div><label class="label">月成本 (¢)</label><input type="number" class="input" value={tokenForm().quota_cost_monthly_cents} onInput={(e) => setTokenForm((p) => ({ ...p, quota_cost_monthly_cents: parseInt(e.currentTarget.value) || 0 }))} /></div>
          </div>
          <div><label class="label">过期时间</label><input type="datetime-local" class="input" value={tokenForm().expires_at} onInput={(e) => setTokenForm((p) => ({ ...p, expires_at: e.currentTarget.value }))} /></div>
          <div class="flex justify-end gap-3 pt-2">
            <button class="btn btn-secondary" onClick={() => setShowTokenModal(false)}>取消</button>
            <button class="btn btn-primary" disabled={saving()} onClick={saveToken}>{saving() ? '保存中...' : editingToken() ? '更新' : '创建'}</button>
          </div>
        </div>
      </Modal>

      <Modal open={showKeyModal()} onClose={() => setShowKeyModal(false)} title="API Key 已生成">
        <div class="space-y-4">
          <p class="text-sm text-dark-300">这是唯一一次显示完整 Key 的机会，请立即复制保存。</p>
          <div class="bg-dark-700/60 p-3 rounded-lg font-mono text-sm break-all text-white">{newKey()}</div>
          <div class="flex justify-end">
            <button class="btn btn-primary" onClick={() => { navigator.clipboard.writeText(newKey()); toast.success('已复制'); }}>
              复制到剪贴板
            </button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default LLMUsageAndBilling
