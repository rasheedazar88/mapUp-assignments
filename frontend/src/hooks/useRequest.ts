import { useCallback, useEffect, useRef, useState } from 'react'
import { errorMessage } from '../api/client'

/**
 * Minimal data-fetching hook: { data, loading, error, reload }.
 * Re-runs when `deps` change; ignores responses from outdated requests.
 * `silent` reloads (polling) keep showing old data instead of a spinner.
 */
export function useRequest<T>(fn: () => Promise<T>, deps: unknown[] = []) {
  const [data, setData] = useState<T | undefined>(undefined)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const seq = useRef(0)
  // "Latest callback" ref: always call the newest `fn` without re-creating `run`.
  const fnRef = useRef(fn)
  useEffect(() => {
    fnRef.current = fn
  })

  const run = useCallback((silent = false) => {
    const id = ++seq.current
    if (!silent) setLoading(true)
    return fnRef
      .current()
      .then((res) => {
        if (id === seq.current) {
          setData(res)
          setError(null)
        }
      })
      .catch((err: unknown) => {
        if (id === seq.current) setError(errorMessage(err))
      })
      .finally(() => {
        if (id === seq.current) setLoading(false)
      })
  }, [])

  useEffect(() => {
    void run()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  return { data, loading, error, reload: run }
}

/** Calls `fn` every `ms` while the tab is visible. */
export function useInterval(fn: () => void, ms: number) {
  const ref = useRef(fn)
  useEffect(() => {
    ref.current = fn
  })
  useEffect(() => {
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') ref.current()
    }, ms)
    return () => clearInterval(id)
  }, [ms])
}
