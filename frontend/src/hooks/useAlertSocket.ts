import { App } from 'antd'
import { useEffect } from 'react'
import { api } from '../api/endpoints'
import type { AlertMessage } from '../api/types'
import { WS_URL } from '../config'
import { CATEGORIES } from '../constants'
import { useAlertStore } from '../store/alertStore'
import { formatDateTime } from '../utils/format'

const MAX_BACKOFF_MS = 30_000

function isAlertMessage(x: unknown): x is AlertMessage {
  const m = x as AlertMessage
  return !!m && typeof m.event_id === 'string' && !!m.vehicle && !!m.geofence && !!m.location
}

/**
 * Keeps ONE WebSocket connection to /ws/alerts open for the whole app.
 *
 * - On every (re)connect it backfills from GET /alerts/events, so alerts fired
 *   while the connection was down are not lost from the feed.
 * - If the connection drops, it reconnects with exponential backoff
 *   (1s, 2s, 4s … max 30s, plus jitter so many tabs don't reconnect in lockstep).
 * - New alerts are added to the Zustand store and shown as a toast.
 */
export function useAlertSocket() {
  const { notification } = App.useApp()

  useEffect(() => {
    const { addLive, mergeHistory, setStatus } = useAlertStore.getState()
    let ws: WebSocket | null = null
    let attempt = 0
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let stopped = false

    const backfill = () =>
      api
        .firedAlerts(50)
        .then((r) => mergeHistory(r.events))
        .catch(() => {
          /* history is best-effort; live feed still works */
        })

    const notify = (msg: AlertMessage) => {
      const restricted = msg.geofence.category === 'restricted_zone'
      const verb = msg.event_type === 'entry' ? 'entered' : 'exited'
      const open = restricted && msg.event_type === 'entry' ? notification.error : notification.warning
      open({
        key: msg.event_id,
        title: `${msg.vehicle.vehicle_number} ${verb} ${msg.geofence.geofence_name}`,
        description: `${CATEGORIES[msg.geofence.category]?.label ?? msg.geofence.category} · driver ${msg.vehicle.driver_name} · ${formatDateTime(msg.timestamp)}`,
        placement: 'bottomRight',
        duration: 6,
      })
    }

    const connect = () => {
      setStatus(attempt === 0 ? 'connecting' : 'reconnecting')
      ws = new WebSocket(WS_URL)

      ws.onopen = () => {
        attempt = 0
        setStatus('open')
        void backfill()
      }
      ws.onmessage = (ev) => {
        try {
          const data: unknown = JSON.parse(ev.data as string)
          if (isAlertMessage(data) && addLive(data)) notify(data)
        } catch {
          /* ignore malformed frames */
        }
      }
      ws.onerror = () => ws?.close()
      ws.onclose = () => {
        if (stopped) return
        setStatus('reconnecting')
        const delay = Math.min(MAX_BACKOFF_MS, 1000 * 2 ** attempt) + Math.random() * 500
        attempt++
        retryTimer = setTimeout(connect, delay)
      }
    }

    connect()

    // Reconnect immediately when the tab becomes visible / network comes back.
    const wake = () => {
      if (ws && ws.readyState === WebSocket.CLOSED && !stopped) {
        clearTimeout(retryTimer)
        attempt = 0
        connect()
      }
    }
    window.addEventListener('online', wake)
    document.addEventListener('visibilitychange', wake)

    return () => {
      stopped = true
      clearTimeout(retryTimer)
      window.removeEventListener('online', wake)
      document.removeEventListener('visibilitychange', wake)
      if (ws?.readyState === WebSocket.CONNECTING) {
        // Closing a socket mid-handshake logs a browser warning (React StrictMode
        // mounts effects twice in dev); close it as soon as it opens instead.
        const pending = ws
        pending.onopen = () => pending.close()
        pending.onclose = null
        pending.onmessage = null
      } else {
        ws?.close()
      }
      setStatus('closed')
    }
  }, [notification])
}
