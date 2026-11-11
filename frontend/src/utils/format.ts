import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

dayjs.extend(relativeTime)

export const formatDateTime = (iso?: string | null) => (iso ? dayjs(iso).format('DD MMM YYYY, HH:mm:ss') : '—')
export const fromNow = (iso?: string | null) => (iso ? dayjs(iso).fromNow() : '—')

/** "1234567" ns → "1.23 ms" (shown next to API results for transparency). */
export function formatNs(ns?: string): string {
  const n = Number(ns)
  if (!Number.isFinite(n)) return ''
  if (n < 1_000) return `${n} ns`
  if (n < 1_000_000) return `${(n / 1_000).toFixed(1)} µs`
  return `${(n / 1_000_000).toFixed(2)} ms`
}
