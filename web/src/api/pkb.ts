import type { PKBProposal, PKBDomain } from '@/types'

const API_BASE = '/api'

async function request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_BASE}${endpoint}`, {
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
    ...options,
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(error.error || `HTTP ${response.status}`)
  }

  return response.json()
}

// PKB 调方向掌舵面 API（Phase I）：待批骨架提议审批 + 设领域大方向(scope)。
// 后端 internal/handler/pkb_steer.go，与 Matrix !pkb / CLI 同一路径（前端接管，Matrix 保留兜底）。
export const pkbSteerApi = {
  // 列出全部待批骨架「大动作」提议
  listProposals: () => request<{ data: PKBProposal[] }>('/pkb/proposals'),

  // 批准提议（应用：快照旧 _index.md → 替换知识树）
  approveProposal: (id: string) =>
    request<{ data: { message: string } }>(`/pkb/proposals/${encodeURIComponent(id)}/approve`, {
      method: 'POST',
    }),

  // 驳回提议（删提议文件，骨架不动）
  rejectProposal: (id: string) =>
    request<{ data: { message: string } }>(`/pkb/proposals/${encodeURIComponent(id)}/reject`, {
      method: 'POST',
    }),

  // 列出全部领域及其当前大方向(scope)
  listDomains: () => request<{ data: PKBDomain[] }>('/pkb/domains'),

  // 设某领域的一句话大方向（外科式写回 domains.yaml，资讯流/兜底域会被后端拒）
  setScope: (name: string, scope: string) =>
    request<{ data: { message: string } }>(`/pkb/domains/${encodeURIComponent(name)}/scope`, {
      method: 'PUT',
      body: JSON.stringify({ scope }),
    }),
}
