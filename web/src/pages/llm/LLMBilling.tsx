import { Component, For, Show, createSignal, createResource } from 'solid-js'
import { llmProxyApi } from '@/api'
import { useToast } from '@/components/Toast'

const LLMBilling: Component = () => {
  const toast = useToast()

  const [groupBy, setGroupBy] = createSignal('date')
  const [fromDate, setFromDate] = createSignal('')
  const [toDate, setToDate] = createSignal('')

  const [usageData, { refetch: refetchUsage }] = createResource(
    () => ({ g: groupBy(), f: fromDate(), t: toDate() }),
    ({ g, f, t }) => llmProxyApi.getUsage(g, f || undefined, t || undefined)
  )
  const rows = () => usageData()?.data || []

  const totalCost = () => rows().reduce((sum: number, r: any) => sum + (r.cost_cents || 0), 0)
  const totalRequests = () => rows().reduce((sum: number, r: any) => sum + (r.requests || 0), 0)
  const totalTokens = () => rows().reduce((sum: number, r: any) => sum + (r.prompt_tokens || 0) + (r.completion_tokens || 0), 0)

  return (
    <div class="p-6">
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">计费与统计</h1>
        <div class="flex gap-2 items-center">
          <select
            class="px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600"
            value={groupBy()}
            onChange={e => setGroupBy(e.currentTarget.value)}
          >
            <option value="date">按日期</option>
            <option value="token">按 Token</option>
            <option value="model">按模型</option>
          </select>
          <input
            type="date"
            class="px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600"
            value={fromDate()}
            onChange={e => setFromDate(e.currentTarget.value)}
            placeholder="开始日期"
          />
          <input
            type="date"
            class="px-3 py-2 border rounded-lg dark:bg-gray-700 dark:border-gray-600"
            value={toDate()}
            onChange={e => setToDate(e.currentTarget.value)}
            placeholder="结束日期"
          />
          <button
            class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors"
            onClick={() => refetchUsage()}
          >
            刷新
          </button>
        </div>
      </div>

      {/* KPI Cards */}
      <div class="grid grid-cols-3 gap-4 mb-6">
        <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <div class="text-sm text-gray-500 dark:text-gray-400">总请求数</div>
          <div class="text-2xl font-bold mt-1">{totalRequests().toLocaleString()}</div>
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <div class="text-sm text-gray-500 dark:text-gray-400">总 Token 数</div>
          <div class="text-2xl font-bold mt-1">{totalTokens().toLocaleString()}</div>
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <div class="text-sm text-gray-500 dark:text-gray-400">总成本</div>
          <div class="text-2xl font-bold mt-1">${(totalCost() / 100).toFixed(2)}</div>
        </div>
      </div>

      {/* Data Table */}
      <div class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
        <table class="w-full text-left">
          <thead class="bg-gray-50 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-sm uppercase">
            <tr>
              <th class="px-4 py-3">维度</th>
              <th class="px-4 py-3">请求数</th>
              <th class="px-4 py-3">Prompt Tokens</th>
              <th class="px-4 py-3">Completion Tokens</th>
              <th class="px-4 py-3">Cached Tokens</th>
              <th class="px-4 py-3">错误数</th>
              <th class="px-4 py-3 text-right">成本</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-gray-700">
            <For each={rows()}>
              {(r: any) => (
                <tr class="hover:bg-gray-50 dark:hover:bg-gray-750">
                  <td class="px-4 py-3 font-medium">
                    {r.date || r.token_id || r.model || '-'}
                  </td>
                  <td class="px-4 py-3 text-sm">{(r.requests || 0).toLocaleString()}</td>
                  <td class="px-4 py-3 text-sm">{(r.prompt_tokens || 0).toLocaleString()}</td>
                  <td class="px-4 py-3 text-sm">{(r.completion_tokens || 0).toLocaleString()}</td>
                  <td class="px-4 py-3 text-sm">{(r.cached_tokens || 0).toLocaleString()}</td>
                  <td class="px-4 py-3 text-sm">{(r.error_count || 0).toLocaleString()}</td>
                  <td class="px-4 py-3 text-sm text-right font-medium">
                    ${((r.cost_cents || 0) / 100).toFixed(2)}
                  </td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
      </div>
    </div>
  )
}

export default LLMBilling
