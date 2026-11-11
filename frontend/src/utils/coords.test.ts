import { describe, expect, it } from 'vitest'
import type { LatLngTuple } from '../api/types'
import { closeRing, interpolate, parseCoordinates, validatePolygon } from './coords'

const readme: LatLngTuple[] = [
  [37.7749, -122.4194],
  [37.7849, -122.4194],
  [37.7849, -122.4094],
  [37.7749, -122.4094],
  [37.7749, -122.4194],
]

describe('closeRing', () => {
  it('appends the first point when the ring is open', () => {
    expect(closeRing(readme.slice(0, 4))).toEqual(readme)
  })
  it('leaves an already-closed ring alone', () => {
    expect(closeRing(readme)).toBe(readme)
  })
})

describe('validatePolygon', () => {
  it('accepts the README polygon', () => expect(validatePolygon(readme)).toBeNull())
  it('rejects open rings', () => expect(validatePolygon(readme.slice(0, 4))).toMatch(/identical/))
  it('rejects too few points', () => expect(validatePolygon([[0, 0], [1, 1], [0, 0]])).toMatch(/at least 4/))
  it('rejects out-of-range latitude', () => expect(validatePolygon([[91, 0], [1, 0], [0, 1], [91, 0]])).toMatch(/latitude/))
  it('rejects out-of-range longitude', () => expect(validatePolygon([[0, 181], [1, 0], [0, 1], [0, 181]])).toMatch(/longitude/))
  it('rejects duplicate corners', () => expect(validatePolygon([[0, 0], [1, 1], [1, 1], [0, 0]])).toMatch(/unique/))
  it('rejects non-numbers', () => expect(validatePolygon([['a', 0], [1, 0], [0, 1], ['a', 0]])).toMatch(/pair/))
})

describe('parseCoordinates', () => {
  it('parses valid JSON', () => expect(parseCoordinates(JSON.stringify(readme)).coords).toEqual(readme))
  it('reports invalid JSON', () => expect(parseCoordinates('[[1,2],').error).toMatch(/JSON/))
  it('reports empty input', () => expect(parseCoordinates('  ').error).toBeTruthy())
})

describe('interpolate', () => {
  it('produces evenly spaced points ending at the target', () => {
    expect(interpolate([0, 0], [1, 2], 2)).toEqual([
      [0.5, 1],
      [1, 2],
    ])
  })
})
