/**
 * Shared date/time formatting utilities
 */

export const formatDate = (value?: string): string => {
  if (!value) return '--'
  return new Date(value).toLocaleString('zh-CN')
}

export const formatDateShort = (value?: string): string => {
  if (!value) return '--'
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
