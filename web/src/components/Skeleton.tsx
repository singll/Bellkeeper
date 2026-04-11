import { Component, JSX, Show } from 'solid-js'

interface SkeletonProps {
  class?: string
  variant?: 'text' | 'circular' | 'rectangular'
  width?: string | number
  height?: string | number
  animation?: 'pulse' | 'wave' | 'none'
}

/**
 * Skeleton loading component
 */
export const Skeleton: Component<SkeletonProps> = (props) => {
  const variant = () => props.variant || 'text'
  const animation = () => props.animation || 'pulse'

  const baseClass = 'bg-dark-700/50'

  const getVariantClass = () => {
    switch (variant()) {
      case 'circular':
        return 'rounded-full'
      case 'rectangular':
        return 'rounded-lg'
      case 'text':
      default:
        return 'rounded'
    }
  }

  const getAnimationClass = () => {
    switch (animation()) {
      case 'wave':
        return 'animate-pulse'
      case 'none':
        return ''
      case 'pulse':
      default:
        return 'animate-pulse'
    }
  }

  const style = () => ({
    width: props.width ? (typeof props.width === 'number' ? `${props.width}px` : props.width) : undefined,
    height: props.height ? (typeof props.height === 'number' ? `${props.height}px` : props.height) : undefined,
  })

  return (
    <div
      class={`${baseClass} ${getVariantClass()} ${getAnimationClass()} ${props.class || ''}`}
      style={style()}
    />
  )
}

/**
 * Skeleton text line
 */
export const SkeletonText: Component<{ lines?: number; class?: string }> = (props) => {
  const lines = () => props.lines || 3

  return (
    <div class={`space-y-2 ${props.class || ''}`}>
      {Array.from({ length: lines() }, (_, i) => (
        <div class="flex gap-2">
          <Skeleton class="flex-1" variant="text" height={16} />
          {i === lines() - 1 && <Skeleton class="w-1/3" variant="text" height={16} />}
        </div>
      ))}
    </div>
  )
}

/**
 * Skeleton card
 */
export const SkeletonCard: Component<{ class?: string }> = (props) => {
  return (
    <div class={`bg-dark-800 rounded-xl p-4 border border-dark-600 ${props.class || ''}`}>
      <div class="flex items-start gap-4">
        <Skeleton variant="circular" width={48} height={48} />
        <div class="flex-1 space-y-2">
          <Skeleton variant="text" width="60%" height={20} />
          <SkeletonText lines={2} />
        </div>
      </div>
    </div>
  )
}

/**
 * Skeleton table row
 */
export const SkeletonTableRow: Component<{ columns?: number; class?: string }> = (props) => {
  const columns = () => props.columns || 4

  return (
    <tr class={`border-b border-dark-700 ${props.class || ''}`}>
      {Array.from({ length: columns() }, (_, i) => (
        <td class="px-4 py-3">
          <Skeleton variant="text" width={i === 0 ? '80%' : '60%'} height={16} />
        </td>
      ))}
    </tr>
  )
}

/**
 * Skeleton table
 */
export const SkeletonTable: Component<{ rows?: number; columns?: number; class?: string }> = (props) => {
  const rows = () => props.rows || 5
  const columns = () => props.columns || 4

  return (
    <div class={`overflow-hidden rounded-lg border border-dark-600 ${props.class || ''}`}>
      <table class="w-full">
        <thead class="bg-dark-800">
          <tr>
            {Array.from({ length: columns() }, (_, i) => (
              <th class="px-4 py-3 text-left">
                <Skeleton variant="text" width={60} height={14} />
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: rows() }, (_, i) => (
            <SkeletonTableRow columns={columns()} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

export default Skeleton
