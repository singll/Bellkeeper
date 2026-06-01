import type { LLMChannelHealth, LLMModelGroupMemberConfig } from '@/types'
import { formatDate } from '@/utils/format'

// Re-export shared formatting functions
export { formatDate as formatDateTime, formatDateShort } from '@/utils/format'

export const formatPercent = (value: number) => `${Math.round(value * 100)}%`

// Circuit breaker status
export const getCircuitBadge = (state: string) => {
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

export const getCircuitLabel = (state: string) => {
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

export const getCircuitDot = (state: string) => {
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

// Color helpers
export const getSuccessRateColor = (rate: number) => {
  if (rate >= 0.9) return 'text-emerald-400'
  if (rate >= 0.7) return 'text-amber-400'
  return 'text-red-400'
}

export const getProgressClass = (ratio: number) => {
  if (ratio >= 0.7) return 'bg-emerald-500'
  if (ratio >= 0.35) return 'bg-amber-500'
  return 'bg-red-500'
}

// Math helpers
export const calcRatio = (current: number, total: number) => {
  if (total <= 0) return 0
  return Math.min(current / total, 1)
}

// Alert severity styling (Tier 5)
export const getSeverityBadge = (severity: string) => {
  switch (severity) {
    case 'critical':
    case 'error':
      return 'badge-danger'
    case 'warning':
      return 'badge-warning'
    case 'info':
      return 'badge-primary'
    default:
      return 'badge-gray'
  }
}

export const getSeverityLabel = (severity: string) => {
  switch (severity) {
    case 'critical':
      return '严重'
    case 'error':
      return '错误'
    case 'warning':
      return '警告'
    case 'info':
      return '信息'
    default:
      return severity
  }
}

export const getSeverityDot = (severity: string) => {
  switch (severity) {
    case 'critical':
    case 'error':
      return 'status-dot-danger'
    case 'warning':
      return 'status-dot-warning'
    case 'info':
      return 'status-dot-success'
    default:
      return 'status-dot-gray'
  }
}

// Channel tier helpers (Tier 4). Empty tier ⇒ derived from is_free.
export const deriveTier = (tier?: string, isFree?: boolean): string => {
  if (tier && tier.trim() !== '') return tier
  return isFree ? 'free' : 'standard'
}

export const getTierLabel = (tier: string) => {
  switch (tier) {
    case 'free':
      return '免费'
    case 'standard':
      return '标准'
    case 'premium':
      return '高级'
    default:
      return tier
  }
}

export const getTierBadge = (tier: string) => {
  switch (tier) {
    case 'free':
      return 'badge-success'
    case 'standard':
      return 'badge-primary'
    case 'premium':
      return 'badge-warning'
    default:
      return 'badge-gray'
  }
}

// Parse a backend JSON-array-string column (e.g. models, task_types) into string[].
export const parseJsonArray = (raw?: string): string[] => {
  if (!raw || raw.trim() === '') return []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.map((v) => String(v)) : []
  } catch {
    return []
  }
}

// Format integer cents as a USD string, e.g. 12345 → "$123.45".
export const formatCents = (cents: number): string => `$${(cents / 100).toFixed(2)}`

// Re-export types for convenience
export type { LLMChannelHealth, LLMModelGroupMemberConfig }
