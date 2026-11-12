import { describe, expect, it } from 'vitest'
import type { AlertMessage } from '../api/types'
import reducer, { alertReceived, clear, historyLoaded, markRead, statusChanged } from './alertSlice'

function alert(id: string, timestamp: string): AlertMessage {
  return {
    event_id: id,
    event_type: 'entry',
    timestamp,
    vehicle: { vehicle_id: 'veh_1', vehicle_number: 'KA-01-1234', driver_name: 'Asha' },
    geofence: { geofence_id: 'geo_1', geofence_name: 'Depot', category: 'delivery_zone' },
    location: { latitude: 12.97, longitude: 77.59 },
  }
}

const start = reducer(undefined, { type: 'init' })

describe('alert slice', () => {
  it('adds live alerts newest first and ignores duplicates', () => {
    let s = reducer(start, alertReceived(alert('a', '2026-01-01T10:00:00Z')))
    s = reducer(s, alertReceived(alert('b', '2026-01-01T11:00:00Z')))
    s = reducer(s, alertReceived(alert('a', '2026-01-01T10:00:00Z')))

    expect(s.feed.map((a) => a.event_id)).toEqual(['b', 'a'])
    expect(s.unread).toBe(2)
    expect(s.revision).toBe(2)
    expect(s.liveIds).toEqual({ a: true, b: true })
  })

  it('loads history without duplicates and without changing unread', () => {
    let s = reducer(start, alertReceived(alert('b', '2026-01-01T11:00:00Z')))
    s = reducer(s, historyLoaded([alert('b', '2026-01-01T11:00:00Z'), alert('c', '2026-01-01T12:00:00Z')]))

    expect(s.feed.map((a) => a.event_id)).toEqual(['c', 'b'])
    expect(s.unread).toBe(1)
    expect(s.liveIds).toEqual({ b: true })
  })

  it('keeps at most 200 alerts', () => {
    const many = Array.from({ length: 250 }, (_, i) => alert(`e${i}`, new Date(Date.UTC(2026, 0, 1, 0, 0, i)).toISOString()))
    const s = reducer(start, historyLoaded(many))
    expect(s.feed).toHaveLength(200)
    expect(s.feed[0].event_id).toBe('e249')
  })

  it('handles status, markRead and clear', () => {
    let s = reducer(start, alertReceived(alert('a', '2026-01-01T10:00:00Z')))
    s = reducer(s, statusChanged('open'))
    s = reducer(s, markRead())
    expect(s).toMatchObject({ status: 'open', unread: 0 })

    s = reducer(s, clear())
    expect(s).toMatchObject({ feed: [], unread: 0, liveIds: {}, status: 'open', revision: 1 })
  })
})
