import { useEffect, useRef, useState } from 'react'

export function useLiveQuery<T>(load: (signal: AbortSignal) => Promise<T>, live: boolean) {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const [revision, setRevision] = useState(0)
  const previous = useRef<{ load: typeof load; revision: number }>()

  useEffect(() => {
    const changed = previous.current?.load !== load
    const manual = previous.current?.revision !== revision
    previous.current = { load, revision }
    if (changed) { setData(null); setUpdatedAt(null); setError('') }
    if (!live && !changed && !manual) { setLoading(false); return }
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    async function refresh() {
      setLoading(true)
      try {
        const result = await load(controller.signal)
        if (controller.signal.aborted) return
        setData(result)
        setUpdatedAt(new Date())
        setError('')
      } catch (err) {
        if (!controller.signal.aborted) setError((err as Error).message)
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false)
          // Wait after completion: slow reads never overlap the next poll.
          if (live) timer = setTimeout(refresh, 3000)
        }
      }
    }
    void refresh()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [load, live, revision])

  return { data, loading, error, updatedAt, refresh: () => setRevision(value => value + 1) }
}
