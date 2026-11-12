import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { AlertMessage } from '../api/types'

export type SocketStatus = 'connecting' | 'open' | 'reconnecting' | 'closed'

const MAX_FEED = 200

interface AlertState {
  feed: AlertMessage[] // newest first
  unread: number
  status: SocketStatus
  revision: number // +1 on every live alert, so pages know to refresh their data
  liveIds: Record<string, boolean> // alerts that arrived live (for the highlight)
}

const initialState: AlertState = {
  feed: [],
  unread: 0,
  status: 'connecting',
  revision: 0,
  liveIds: {},
}

// Newest first, keep at most MAX_FEED.
function sortAndTrim(list: AlertMessage[]) {
  return list
    .sort((a, b) => b.timestamp.localeCompare(a.timestamp) || b.event_id.localeCompare(a.event_id))
    .slice(0, MAX_FEED)
}

// Redux Toolkit lets reducers "mutate" state directly; it makes the immutable copy for us.
const alertSlice = createSlice({
  name: 'alerts',
  initialState,
  reducers: {
    alertReceived(state, action: PayloadAction<AlertMessage>) {
      const alert = action.payload
      if (state.feed.some((a) => a.event_id === alert.event_id)) return // already have it
      state.feed = sortAndTrim([alert, ...state.feed])
      state.unread += 1
      state.revision += 1
      state.liveIds[alert.event_id] = true
    },
    historyLoaded(state, action: PayloadAction<AlertMessage[]>) {
      const fresh = action.payload.filter((m) => !state.feed.some((a) => a.event_id === m.event_id))
      if (fresh.length > 0) state.feed = sortAndTrim([...state.feed, ...fresh])
    },
    statusChanged(state, action: PayloadAction<SocketStatus>) {
      state.status = action.payload
    },
    markRead(state) {
      state.unread = 0
    },
    clear(state) {
      state.feed = []
      state.unread = 0
      state.liveIds = {}
    },
  },
})

export const { alertReceived, historyLoaded, statusChanged, markRead, clear } = alertSlice.actions
export default alertSlice.reducer
