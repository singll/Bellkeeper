import { Component, Show } from 'solid-js'
import type { ErrorBoundaryProps } from 'solid-js'

/**
 * Fallback UI shown when an uncaught error bubbles up to the ErrorBoundary.
 * Prevents a single page exception from white-screening the entire app.
 */
const ErrorFallback: Component<ErrorBoundaryProps> = (props) => {
  const error = () => {
    const e = props.error
    return e instanceof Error ? e.message : String(e)
  }

  return (
    <div class="min-h-screen flex items-center justify-center bg-dark-900 p-4">
      <div class="card max-w-md w-full text-center">
        <div class="mb-4">
          <svg class="w-16 h-16 mx-auto text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
        </div>
        <h2 class="text-xl font-bold text-white mb-2">页面加载出错</h2>
        <p class="text-sm text-dark-400 mb-4">
          抱歉，页面渲染时发生了错误。这可能是临时性问题，请尝试刷新页面。
        </p>
        <Show when={error()}>
          <div class="text-xs text-dark-500 bg-dark-700 rounded-lg p-3 mb-4 font-mono text-left max-h-32 overflow-auto">
            {error()}
          </div>
        </Show>
        <button
          class="btn btn-primary w-full"
          onClick={() => window.location.reload()}
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          刷新页面
        </button>
      </div>
    </div>
  )
}

export default ErrorFallback
