import { createSignal, onCleanup, Accessor } from 'solid-js'

export interface UseAutoRefreshOptions {
  /** Refresh interval in milliseconds. Default: 30000 (30s) */
  interval?: number
  /** Whether auto-refresh is enabled by default. Default: true */
  enabled?: boolean
  /** Callback when refresh happens */
  onRefresh?: () => void | Promise<void>
}

/**
 * Hook for auto-refresh functionality
 *
 * @example
 * ```tsx
 * const { enabled, setEnabled, refresh } = useAutoRefresh({
 *   interval: 30000,
 *   onRefresh: () => fetchData()
 * });
 * ```
 */
export function useAutoRefresh<T>(
  fetchFn: () => T | Promise<T>,
  options: UseAutoRefreshOptions = {}
): {
  data: Accessor<T | undefined>
  loading: Accessor<boolean>
  error: Accessor<Error | undefined>
  enabled: Accessor<boolean>
  setEnabled: (value: boolean) => void
  refresh: () => Promise<void>
  lastUpdated: Accessor<Date | undefined>
} {
  const { interval = 30000, enabled: defaultEnabled = true, onRefresh } = options

  const [data, setData] = createSignal<T | undefined>(undefined)
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal<Error | undefined>(undefined)
  const [enabled, setEnabled] = createSignal(defaultEnabled)
  const [lastUpdated, setLastUpdated] = createSignal<Date | undefined>(undefined)

  let timerId: number | undefined

  const refresh = async () => {
    setLoading(true)
    setError(undefined)
    try {
      const result = await fetchFn()
      setData(result)
      setLastUpdated(new Date())
      onRefresh?.()
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)))
    } finally {
      setLoading(false)
    }
  }

  const startTimer = () => {
    if (timerId) return
    timerId = window.setInterval(refresh, interval)
  }

  const stopTimer = () => {
    if (timerId) {
      window.clearInterval(timerId)
      timerId = undefined
    }
  }

  // Initial fetch
  refresh()

  // Watch enabled state
  createEffect(() => {
    if (enabled()) {
      startTimer()
    } else {
      stopTimer()
    }
  })

  onCleanup(() => {
    stopTimer()
  })

  return {
    data,
    loading,
    error,
    enabled,
    setEnabled,
    refresh,
    lastUpdated,
  }
}

/**
 * Simplified hook for just managing auto-refresh state
 *
 * @example
 * ```tsx
 * const { enabled, setEnabled, refresh } = useAutoRefreshState(30000);
 *
 * return (
 *   <button onClick={() => enabled() ? setEnabled(false) : setEnabled(true)}>
 *     {enabled() ? 'Auto-refresh ON' : 'Auto-refresh OFF'}
 *   </button>
 * );
 * ```
 */
export function useAutoRefreshState(defaultInterval = 30000): {
  enabled: Accessor<boolean>
  setEnabled: (value: boolean) => void
  interval: Accessor<number>
  setInterval: (value: number) => void
  refresh: () => void
} {
  const [enabled, setEnabled] = createSignal(true)
  const [interval, setInterval] = createSignal(defaultInterval)
  const [refreshFn, setRefreshFn] = createSignal<() => void>(() => {})

  let timerId: number | undefined

  const startTimer = () => {
    stopTimer()
    timerId = window.setInterval(() => {
      refreshFn()()
    }, interval())
  }

  const stopTimer = () => {
    if (timerId) {
      window.clearInterval(timerId)
      timerId = undefined
    }
  }

  const refresh = () => refreshFn()()

  // Watch enabled state
  createEffect(() => {
    if (enabled()) {
      startTimer()
    } else {
      stopTimer()
    }
  })

  // Watch interval changes
  createEffect(() => {
    const _ = interval()
    if (enabled()) {
      startTimer()
    }
  })

  onCleanup(() => {
    stopTimer()
  })

  return {
    enabled,
    setEnabled,
    interval,
    setInterval,
    refresh,
  }
}

export default useAutoRefresh
