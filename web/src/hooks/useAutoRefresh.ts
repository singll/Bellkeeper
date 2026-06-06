import { createSignal, onCleanup, Accessor, createEffect } from 'solid-js'

export interface UseAutoRefreshOptions {
  /** Refresh interval in milliseconds. Default: 30000 (30s) */
  interval?: number
  /** Whether auto-refresh is enabled by default. Default: true */
  enabled?: boolean
  /** Callback when refresh happens */
  onRefresh?: () => void | Promise<void>
  /** Show countdown in UI. Default: true */
  showCountdown?: boolean
}

/**
 * Hook for auto-refresh functionality with countdown support
 *
 * @example
 * ```tsx
 * const { enabled, setEnabled, refresh, countdown } = useAutoRefreshState({
 *   interval: 30000,
 *   onRefresh: () => fetchData()
 * });
 *
 * // In JSX:
 * <button onClick={() => setEnabled(!enabled())}>
 *   {enabled() ? `${countdown()}s` : 'Paused'}
 * </button>
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
  countdown: Accessor<number>
} {
  const { interval = 30000, enabled: defaultEnabled = true, onRefresh, showCountdown = true } = options

  const [data, setData] = createSignal<T | undefined>(undefined)
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal<Error | undefined>(undefined)
  const [enabled, setEnabled] = createSignal(defaultEnabled)
  const [lastUpdated, setLastUpdated] = createSignal<Date | undefined>(undefined)
  const [countdown, setCountdown] = createSignal(Math.ceil(interval / 1000))

  let timerId: number | undefined
  let countdownId: number | undefined

  const refresh = async () => {
    setLoading(true)
    setError(undefined)
    try {
      const result = await fetchFn()
      setData(() => result)
      setLastUpdated(new Date())
      setCountdown(Math.ceil(interval / 1000))
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

    // Start countdown if enabled
    if (showCountdown) {
      setCountdown(Math.ceil(interval / 1000))
      countdownId = window.setInterval(() => {
        setCountdown((c) => (c <= 1 ? Math.ceil(interval / 1000) : c - 1))
      }, 1000)
    }
  }

  const stopTimer = () => {
    if (timerId) {
      window.clearInterval(timerId)
      timerId = undefined
    }
    if (countdownId) {
      window.clearInterval(countdownId)
      countdownId = undefined
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
    countdown,
  }
}

/**
 * Simplified hook for just managing auto-refresh state with countdown
 *
 * @example
 * ```tsx
 * const { enabled, setEnabled, refresh, countdown } = useAutoRefreshState(30000);
 *
 * return (
 *   <button onClick={() => enabled() ? setEnabled(false) : setEnabled(true)}>
 *     {enabled() ? `${countdown()}s` : 'Auto-refresh OFF'}
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
  countdown: Accessor<number>
} {
  const [enabled, setEnabled] = createSignal(true)
  const [interval, setInterval] = createSignal(defaultInterval)
  const [refreshFn, setRefreshFn] = createSignal<() => void>(() => {})
  const [countdown, setCountdown] = createSignal(Math.ceil(defaultInterval / 1000))

  let timerId: number | undefined
  let countdownId: number | undefined

  const startTimer = () => {
    stopTimer()
    timerId = window.setInterval(() => {
      refreshFn()()
    }, interval())

    // Start countdown
    setCountdown(Math.ceil(interval() / 1000))
    countdownId = window.setInterval(() => {
      setCountdown((c) => (c <= 1 ? Math.ceil(interval() / 1000) : c - 1))
    }, 1000)
  }

  const stopTimer = () => {
    if (timerId) {
      window.clearInterval(timerId)
      timerId = undefined
    }
    if (countdownId) {
      window.clearInterval(countdownId)
      countdownId = undefined
    }
  }

  const refresh = () => {
    refreshFn()()
    setCountdown(Math.ceil(interval() / 1000))
  }

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
    countdown,
  }
}

export default useAutoRefresh
