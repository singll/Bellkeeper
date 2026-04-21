import { Component, JSX, Show } from 'solid-js'

interface EmptyStateProps {
  icon?: JSX.Element
  title: string
  description?: string
  action?: {
    label: string
    onClick: () => void
  }
  class?: string
}

/**
 * Empty state component for displaying when there's no data
 */
export const EmptyState: Component<EmptyStateProps> = (props) => {
  const defaultIcon = () => (
    <svg
      class="w-16 h-16 text-dark-500"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="1.5"
        d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
      />
    </svg>
  )

  return (
    <div class={`flex flex-col items-center justify-center py-12 px-4 text-center ${props.class || ''}`}>
      <div class="mb-4 text-dark-500">
        {props.icon || defaultIcon()}
      </div>
      <h3 class="text-lg font-medium text-dark-200 mb-2">{props.title}</h3>
      <Show when={props.description}>
        <p class="text-sm text-dark-400 max-w-md mb-6">{props.description}</p>
      </Show>
      <Show when={props.action}>
        <button
          onClick={props.action!.onClick}
          class="btn btn-primary"
        >
          {props.action!.label}
        </button>
      </Show>
    </div>
  )
}

/**
 * Common empty state variants
 */
export const EmptyStateVariants = {
  NoData: (props?: Partial<EmptyStateProps>) => (
    <EmptyState
      title="暂无数据"
      description="这里还没有内容，尝试先添加一些数据。"
      {...props}
    />
  ),

  NoResults: (props?: Partial<EmptyStateProps>) => (
    <EmptyState
      icon={
        <svg
          class="w-16 h-16 text-dark-500"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="1.5"
            d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
          />
        </svg>
      }
      title="未找到结果"
      description="尝试调整搜索或筛选条件。"
      {...props}
    />
  ),

  Error: (props?: Partial<EmptyStateProps> & { onRetry?: () => void }) => (
    <EmptyState
      icon={
        <svg
          class="w-16 h-16 text-red-500"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="1.5"
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
          />
        </svg>
      }
      title="出现错误"
      description="加载数据时发生错误。"
      action={props?.onRetry ? { label: '重试', onClick: props.onRetry } : undefined}
      {...props}
    />
  ),

  Loading: (props?: Partial<EmptyStateProps>) => (
    <EmptyState
      icon={
        <svg
          class="w-16 h-16 text-primary-500 animate-spin"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="1.5"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
      }
      title="加载中..."
      description="请稍候，正在获取数据。"
      {...props}
    />
  ),
}

export default EmptyState
