import { Component, For, Show, createSignal, createResource } from 'solid-js'
import type { LLMToken } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'

const LLMTokens: Component = () => {
  const toast = useToast()

  const [tokensData, { refetch: refetchTokens }] = createResource(() => llmProxyApi.listTokens())
  const tokens = () => tokensData()?.data || []

  const [showModal, setShowModal] = createSignal(false)
  const [editingToken, setEditingToken] = createSignal<LLMToken | null>(null)
  const [showKeyModal, setShowKeyModal] = createSignal(false)
  const [newKey, setNewKey] = createSignal('')
  const [saving, setSaving] = createSignal(false)

  const [form, setForm] = createSignal({
    name: '',
    caller_id: '',
    allowed_models: '',
    allowed_groups: '',
    quota_requests_daily: 0,
    quota_tokens_daily: 0,
    quota_cost_monthly_cents: 0,
    expires_at: '',
  })

  const openCreate = () => {
    setEditingToken(null)
    setForm({ name: '', caller_id: '', allowed_models: '', allowed_groups: '', quota_requests_daily: 0, quota_tokens_daily: 0, quota_cost_monthly_cents: 0, expires_at: '' })
    setShowModal(true)
  }

  const openEdit = (t: LLMToken) => {
    setEditingToken(t)
    setForm({
      name: t.name,
      caller_id: t.caller_id,
      allowed_models: t.allowed_models,
      allowed_groups: t.allowed_groups,
      quota_requests_daily: t.quota_requests_daily,
      quota_tokens_daily: t.quota_tokens_daily,
      quota_cost_monthly_cents: t.quota_cost_monthly_cents,
      expires_at: t.expires_at || '',
    })
    setShowModal(true)
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const f = form()
      const payload: Record<string, unknown> = {
        name: f.name,
        caller_id: f.caller_id,
        allowed_models: f.allowed_models ? JSON.parse(f.allowed_models) : [],
        allowed_groups: f.allowed_groups ? JSON.parse(f.allowed_groups) : [],
        quota_requests_daily: f.quota_requests_daily,
        quota_tokens_daily: f.quota_tokens_daily,
        quota_cost_monthly_cents: f.quota_cost_monthly_cents,
      }
      if (f.expires_at) {
        payload.expires_at = new Date(f.expires_at).toISOString()
      }

      if (editingToken()) {
        await llmProxyApi.updateToken(editingToken()!.id, payload)
        toast.success('Token updated')
      } else {
        const res = await llmProxyApi.createToken(payload)
        setNewKey(res.key)
        setShowKeyModal(true)
        toast.success('Token created')
      }
      setShowModal(false)
      refetchTokens()
    } catch (e: any) {
      toast.error(e.message || 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this token?')) return
    try {
      await llmProxyApi.deleteToken(id)
      toast.success('Token deleted')
      refetchTokens()
    } catch (e: any) {
      toast.error(e.message || 'Delete failed')
    }
  }

  const handleRegenerate = async (id: number) => {
    if (!confirm('Regenerate API key? Old key will stop working immediately.')) return
    try {
      const res = await llmProxyApi.regenerateTokenKey(id)
      setNewKey(res.key)
      setShowKeyModal(true)
      toast.success('Key regenerated')
      refetchTokens()
    } catch (e: any) {
      toast.error(e.message || 'Regenerate failed')
    }
  }

  return (
    <div class="p-6">
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">API Tokens</h1>
        <button
          class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors"
          onClick={openCreate}
        >
          + New Token
        </button>
      </div>

      <div class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
        <table class="w-full text-left">
          <thead class="bg-gray-50 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-sm uppercase">
            <tr>
              <th class="px-4 py-3">Name</th>
              <th class="px-4 py-3">Key Prefix</th>
              <th class="px-4 py-3">Caller ID</th>
              <th class="px-4 py-3">Quota (req/tok/$)</th>
              <th class="px-4 py-3">Expires</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-gray-700">
            <For each={tokens()}>
              {(t) => (
                <tr class="hover:bg-gray-50 dark:hover:bg-gray-750">
                  <td class="px-4 py-3 font-medium">{t.name}</td>
                  <td class="px-4 py-3 font-mono text-sm text-gray-500">{t.key_prefix}***</td>
                  <td class="px-4 py-3 text-sm">{t.caller_id}</td>
                  <td class="px-4 py-3 text-sm">
                    {t.quota_requests_daily > 0 ? t.quota_requests_daily : '∞'} /
                    {t.quota_tokens_daily > 0 ? t.quota_tokens_daily : '∞'} /
                    {t.quota_cost_monthly_cents > 0 ? '$' + (t.quota_cost_monthly_cents / 100).toFixed(2) : '∞'}
                  </td>
                  <td class="px-4 py-3 text-sm">
                    {t.expires_at ? new Date(t.expires_at).toLocaleDateString() : 'Never'}
                  </td>
                  <td class="px-4 py-3">
                    <span class={`px-2 py-1 rounded-full text-xs font-medium ${t.enabled ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'}`}>
                      {t.enabled ? 'Active' : 'Disabled'}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-right space-x-2">
                    <button class="text-sm text-blue-600 hover:underline" onClick={() => openEdit(t)}>Edit</button>
                    <button class="text-sm text-orange-600 hover:underline" onClick={() => handleRegenerate(t.id)}>Reset Key</button>
                    <button class="text-sm text-red-600 hover:underline" onClick={() => handleDelete(t.id)}>Delete</button>
                  </td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
      </div>

      {/* Create/Edit Modal */}
      <Show when={showModal()}>
        <Modal onClose={() => setShowModal(false)} title={editingToken() ? 'Edit Token' : 'Create Token'}>
          <div class="space-y-4 p-4">
            <div>
              <label class="block text-sm font-medium mb-1">Name</label>
              <input class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().name} onInput={e => setForm({ ...form(), name: e.currentTarget.value })} />
            </div>
            <Show when={!editingToken()}>
              <div>
                <label class="block text-sm font-medium mb-1">Caller ID</label>
                <input class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().caller_id} onInput={e => setForm({ ...form(), caller_id: e.currentTarget.value })} />
              </div>
            </Show>
            <div>
              <label class="block text-sm font-medium mb-1">Allowed Models (JSON array)</label>
              <input class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600 font-mono text-sm" placeholder='["gpt-4", "claude-sonnet"]' value={form().allowed_models} onInput={e => setForm({ ...form(), allowed_models: e.currentTarget.value })} />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1">Allowed Groups (JSON array)</label>
              <input class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600 font-mono text-sm" placeholder='["pool-chat-free", "pool-chat-balanced"]' value={form().allowed_groups} onInput={e => setForm({ ...form(), allowed_groups: e.currentTarget.value })} />
            </div>
            <div class="grid grid-cols-3 gap-3">
              <div>
                <label class="block text-sm font-medium mb-1">Daily Requests</label>
                <input type="number" class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().quota_requests_daily} onInput={e => setForm({ ...form(), quota_requests_daily: parseInt(e.currentTarget.value) || 0 })} />
              </div>
              <div>
                <label class="block text-sm font-medium mb-1">Daily Tokens</label>
                <input type="number" class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().quota_tokens_daily} onInput={e => setForm({ ...form(), quota_tokens_daily: parseInt(e.currentTarget.value) || 0 })} />
              </div>
              <div>
                <label class="block text-sm font-medium mb-1">Monthly Cost (cents)</label>
                <input type="number" class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().quota_cost_monthly_cents} onInput={e => setForm({ ...form(), quota_cost_monthly_cents: parseInt(e.currentTarget.value) || 0 })} />
              </div>
            </div>
            <div>
              <label class="block text-sm font-medium mb-1">Expires At</label>
              <input type="datetime-local" class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().expires_at} onInput={e => setForm({ ...form(), expires_at: e.currentTarget.value })} />
            </div>
            <div class="flex justify-end gap-2 pt-2">
              <button class="px-4 py-2 border rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700" onClick={() => setShowModal(false)}>Cancel</button>
              <button class="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50" onClick={handleSave} disabled={saving()}>
                {saving() ? 'Saving...' : editingToken() ? 'Update' : 'Create'}
              </button>
            </div>
          </div>
        </Modal>
      </Show>

      {/* Show Key Modal */}
      <Show when={showKeyModal()}>
        <Modal onClose={() => setShowKeyModal(false)} title="API Key Generated">
          <div class="p-4 space-y-4">
            <p class="text-sm text-gray-600 dark:text-gray-300">
              This is the only time you will see the full key. Copy it now.
            </p>
            <div class="bg-gray-100 dark:bg-gray-700 p-3 rounded-lg font-mono text-sm break-all">
              {newKey()}
            </div>
            <div class="flex justify-end">
              <button class="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700" onClick={() => { navigator.clipboard.writeText(newKey()); toast.success('Copied'); }}>
                Copy to Clipboard
              </button>
            </div>
          </div>
        </Modal>
      </Show>
    </div>
  )
}

export default LLMTokens
