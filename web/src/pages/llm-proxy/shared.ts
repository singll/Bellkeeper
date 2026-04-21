import type { LLMChannelHealth, LLMModelGroupMemberConfig } from '@/types'
import { formatDate } from '@/utils/format'

// Re-export shared formatting functions
export { formatDate as formatDateTime } from '@/utils/format'

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

// Re-export types for convenience
export type { LLMChannelHealth, LLMModelGroupMemberConfig }
