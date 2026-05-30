import { Component, For, Show, createSignal, createResource } from 'solid-js'
import type { LLMModelPricing } from '@/types'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'

const LLMPricing: Component = () => {
  const toast = useToast()

  const [pricingData, { refetch: refetchPricing }] = createResource(() => llmProxyApi.listPricing())
  const pricing = () => pricingData()?.data || []

  const [showModal, setShowModal] = createSignal(false)
  const [editingItem, setEditingItem] = createSignal<LLMModelPricing | null>(null)
  const [saving, setSaving] = createSignal(false)

  const [form, setForm] = createSignal({
    channel_name: '',
    model: '',
    input_price_per_1m_cents: 0,
    output_price_per_1m_cents: 0,
    cached_input_price_per_1m_cents: 0,
    currency: 'USD',
    notes: '',
  })

  const openCreate = () => {
    setEditingItem(null)
    setForm({ channel_name: '', model: '', input_price_per_1m_cents: 0, output_price_per_1m_cents: 0, cached_input_price_per_1m_cents: 0, currency: 'USD', notes: '' })
    setShowModal(true)
  }

  const openEdit = (p: LLMModelPricing) => {
    setEditingItem(p)
    setForm({
      channel_name: p.channel_name,
      model: p.model,
      input_price_per_1m_cents: p.input_price_per_1m_cents,
      output_price_per_1m_cents: p.output_price_per_1m_cents,
      cached_input_price_per_1m_cents: p.cached_input_price_per_1m_cents,
      currency: p.currency,
      notes: p.notes,
    })
    setShowModal(true)
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const f = form()
      const payload: Record<string, unknown> = { ...f }
      if (editingItem()) {
        await llmProxyApi.updatePricing(editingItem()!.id, payload)
        toast.success('Pricing updated')
      } else {
        await llmProxyApi.createPricing(payload)
        toast.success('Pricing created')
      }
      setShowModal(false)
      refetchPricing()
    } catch (e: any) {
      toast.error(e.message || 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this pricing rule?')) return
    try {
      await llmProxyApi.deletePricing(id)
      toast.success('Pricing deleted')
      refetchPricing()
    } catch (e: any) {
      toast.error(e.message || 'Delete failed')
    }
  }

  return (
    <div class="p-6">
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">模型定价</h1>
        <button
          class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors"
          onClick={openCreate}
        >
          + 新增定价
        </button>
      </div>

      <div class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
        <table class="w-full text-left">
          <thead class="bg-gray-50 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-sm uppercase">
            <tr>
              <th class="px-4 py-3">渠道</th>
              <th class="px-4 py-3">模型</th>
              <th class="px-4 py-3">Input / 1M</th>
              <th class="px-4 py-3">Output / 1M</th>
              <th class="px-4 py-3">Cached / 1M</th>
              <th class="px-4 py-3">货币</th>
              <th class="px-4 py-3 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-gray-700">
            <For each={pricing()}>
              {(p) => (
                <tr class="hover:bg-gray-50 dark:hover:bg-gray-750">
                  <td class="px-4 py-3 font-medium">{p.channel_name}</td>
                  <td class="px-4 py-3 font-mono text-sm">{p.model}</td>
                  <td class="px-4 py-3 text-sm">{p.input_price_per_1m_cents} ¢</td>
                  <td class="px-4 py-3 text-sm">{p.output_price_per_1m_cents} ¢</td>
                  <td class="px-4 py-3 text-sm">{p.cached_input_price_per_1m_cents > 0 ? p.cached_input_price_per_1m_cents + ' ¢' : 'auto'}</td>
                  <td class="px-4 py-3 text-sm">{p.currency}</td>
                  <td class="px-4 py-3 text-right space-x-2">
                    <button class="text-sm text-blue-600 hover:underline" onClick={() => openEdit(p)}>编辑</button>
                    <button class="text-sm text-red-600 hover:underline" onClick={() => handleDelete(p.id)}>删除</button>
                  </td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
      </div>

      <Show when={showModal()}>
        <Modal onClose={() => setShowModal(false)} title={editingItem() ? '编辑定价' : '新增定价'}>
          <div class="space-y-4 p-4">
            <div class="grid grid-cols-2 gap-3">
              <div>
                <label class="block text-sm font-medium mb-1">渠道</label>
                <input class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().channel_name} onInput={e => setForm({ ...form(), channel_name: e.currentTarget.value })} />
              </div>
              <div>
                <label class="block text-sm font-medium mb-1">模型</label>
                <input class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().model} onInput={e => setForm({ ...form(), model: e.currentTarget.value })} />
              </div>
            </div>
            <div class="grid grid-cols-3 gap-3">
              <div>
                <label class="block text-sm font-medium mb-1">Input / 1M (¢)</label>
                <input type="number" class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().input_price_per_1m_cents} onInput={e => setForm({ ...form(), input_price_per_1m_cents: parseInt(e.currentTarget.value) || 0 })} />
              </div>
              <div>
                <label class="block text-sm font-medium mb-1">Output / 1M (¢)</label>
                <input type="number" class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().output_price_per_1m_cents} onInput={e => setForm({ ...form(), output_price_per_1m_cents: parseInt(e.currentTarget.value) || 0 })} />
              </div>
              <div>
                <label class="block text-sm font-medium mb-1">Cached / 1M (¢)</label>
                <input type="number" class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" placeholder="0 = auto" value={form().cached_input_price_per_1m_cents} onInput={e => setForm({ ...form(), cached_input_price_per_1m_cents: parseInt(e.currentTarget.value) || 0 })} />
              </div>
            </div>
            <div>
              <label class="block text-sm font-medium mb-1">备注</label>
              <input class="w-full px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600" value={form().notes} onInput={e => setForm({ ...form(), notes: e.currentTarget.value })} />
            </div>
            <div class="flex justify-end gap-2 pt-2">
              <button class="px-4 py-2 border rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700" onClick={() => setShowModal(false)}>取消</button>
              <button class="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50" onClick={handleSave} disabled={saving()}>
                {saving() ? '保存中...' : editingItem() ? '更新' : '创建'}
              </button>
            </div>
          </div>
        </Modal>
      </Show>
    </div>
  )
}

export default LLMPricing
