import type { LatLngTuple } from '../api/types'

/** Round to 6 decimals (~11 cm), which is plenty for vehicles and keeps payloads readable. */
export const round6 = (n: number) => Math.round(n * 1e6) / 1e6

/** Leaflet/Geoman give an OPEN ring; the API requires the first point repeated at the end. */
export function closeRing(points: LatLngTuple[]): LatLngTuple[] {
  if (points.length === 0) return points
  const [first] = points
  const last = points[points.length - 1]
  if (first[0] === last[0] && first[1] === last[1]) return points
  return [...points, [first[0], first[1]]]
}

/**
 * Client-side copy of the backend rules, for instant feedback in the form.
 * The backend validates again (and also rejects self-intersecting shapes).
 * Returns an error message, or null if valid.
 */
export function validatePolygon(coords: unknown): string | null {
  if (!Array.isArray(coords)) return 'Coordinates must be an array of [latitude, longitude] pairs'
  if (coords.length < 4) return `A polygon needs at least 4 points (3 corners + closing point); got ${coords.length}`
  for (let i = 0; i < coords.length; i++) {
    const p = coords[i]
    if (!Array.isArray(p) || p.length !== 2 || typeof p[0] !== 'number' || typeof p[1] !== 'number') {
      return `Point ${i + 1} must be a [latitude, longitude] pair of numbers`
    }
    if (!Number.isFinite(p[0]) || p[0] < -90 || p[0] > 90) return `Point ${i + 1}: latitude must be between -90 and 90`
    if (!Number.isFinite(p[1]) || p[1] < -180 || p[1] > 180) return `Point ${i + 1}: longitude must be between -180 and 180`
  }
  const first = coords[0] as LatLngTuple
  const last = coords[coords.length - 1] as LatLngTuple
  if (first[0] !== last[0] || first[1] !== last[1]) return 'First and last coordinates must be identical (closed polygon)'
  const unique = new Set((coords as LatLngTuple[]).slice(0, -1).map((p) => `${p[0]},${p[1]}`))
  if (unique.size < 3) return 'A polygon needs at least 3 unique points'
  return null
}

/** Parse the coordinates textarea. Returns { coords } or { error }. */
export function parseCoordinates(text: string): { coords?: LatLngTuple[]; error?: string } {
  if (!text.trim()) return { error: 'Draw a polygon on the map or paste coordinates' }
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    return { error: 'Not valid JSON. Expected e.g. [[37.77, -122.41], [37.78, -122.41], ...]' }
  }
  const error = validatePolygon(parsed)
  return error ? { error } : { coords: parsed as LatLngTuple[] }
}

export function formatCoordinates(coords: LatLngTuple[]): string {
  return '[\n' + coords.map((p) => `  [${p[0]}, ${p[1]}]`).join(',\n') + '\n]'
}

export const formatLatLng = (lat: number, lng: number) => `${lat.toFixed(5)}, ${lng.toFixed(5)}`

/** Linear interpolation between two points, used by the trip simulator. */
export function interpolate(a: LatLngTuple, b: LatLngTuple, steps: number): LatLngTuple[] {
  const out: LatLngTuple[] = []
  for (let i = 1; i <= steps; i++) {
    const t = i / steps
    out.push([round6(a[0] + (b[0] - a[0]) * t), round6(a[1] + (b[1] - a[1]) * t)])
  }
  return out
}
