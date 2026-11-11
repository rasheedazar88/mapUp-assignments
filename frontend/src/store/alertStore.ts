import { create } from 'zustand'
import type { AlertMessage } from '../api/types'

export type SocketStatus = 'connecting' | 'open' | 'reconnecting' | 'closed'

const MAX_FEED = 200

interface AlertState {
  feed: AlertMessage[] // newest first (by event timestamp)
  unread: number
  status: SocketStatus
  /** Incremented on every new live alert, so pages can refresh their data. */
  revision: number
  /** event_ids that arrived live in this session (used for highlight animation). */
  liveIds: Set<string>

  addLive: (msg: AlertMessage) => boolean
  mergeHistory: (msgs: AlertMessage[]) => void
  setStatus: (s: SocketStatus) => void
  markRead: () => void
  clear: () => void
}

function sortNewestFirst(list: AlertMessage[]) {
  return list.sort((a, b) => b.timestamp.localeCompare(a.timestamp) || b.event_id.localeCompare(a.event_id))
}

/**
 * Global alert state (Zustand). Shared by the header badge, the Live Alerts page,
 * the dashboard and the map pages, so it lives outside any single component.
 */
export const useAlertStore = create<AlertState>((set, get) => ({
  feed: [],
  unread: 0,
  status: 'connecting',
  revision: 0,
  liveIds: new Set(),

  // Returns false for duplicates (e.g. an alert seen live AND in the reconnect backfill).
  addLive: (msg) => {
    if (get().feed.some((a) => a.event_id === msg.event_id)) return false
    set((s) => ({
      feed: sortNewestFirst([msg, ...s.feed]).slice(0, MAX_FEED),
      unread: s.unread + 1,
      revision: s.revision + 1,
      liveIds: new Set(s.liveIds).add(msg.event_id),
    }))
    return true
  },

  mergeHistory: (msgs) =>
    set((s) => {
      const known = new Set(s.feed.map((a) => a.event_id))
      const fresh = msgs.filter((m) => !known.has(m.event_id))
      if (fresh.length === 0) return s
      return { feed: sortNewestFirst([...s.feed, ...fresh]).slice(0, MAX_FEED) }
    }),

  setStatus: (status) => set({ status }),
  markRead: () => set({ unread: 0 }),
  clear: () => set({ feed: [], unread: 0, liveIds: new Set() }),
}))
