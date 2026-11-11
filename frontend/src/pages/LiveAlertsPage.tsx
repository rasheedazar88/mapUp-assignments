import { ClearOutlined, EnvironmentOutlined, LoginOutlined, LogoutOutlined } from '@ant-design/icons'
import { Button, Card, Empty, Segmented, Tag, Typography } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { CircleMarker, Tooltip } from 'react-leaflet'
import { useNavigate } from 'react-router'
import { api } from '../api/endpoints'
import type { AlertMessage, LatLngTuple } from '../api/types'
import { CategoryTag, EventTag, PageHeader } from '../components/common'
import { BaseMap } from '../components/map/BaseMap'
import { GeofenceLayer } from '../components/map/GeofenceLayer'
import { useRequest } from '../hooks/useRequest'
import { useAlertStore } from '../store/alertStore'
import { formatLatLng } from '../utils/coords'
import { formatDateTime, fromNow } from '../utils/format'

export function LiveAlertsPage() {
  const navigate = useNavigate()
  const feed = useAlertStore((s) => s.feed)
  const status = useAlertStore((s) => s.status)
  const liveIds = useAlertStore((s) => s.liveIds)
  const markRead = useAlertStore((s) => s.markRead)
  const clear = useAlertStore((s) => s.clear)
  const [filter, setFilter] = useState<'all' | 'entry' | 'exit'>('all')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const geofences = useRequest(() => api.listGeofences(), [])

  // Being on this page means the user has seen the alerts.
  useEffect(() => markRead(), [feed.length, markRead])

  const items = useMemo(() => feed.filter((a) => filter === 'all' || a.event_type === filter), [feed, filter])
  const selected = items.find((a) => a.event_id === selectedId) ?? items[0]
  const selectedGeofence = geofences.data?.geofences.find((g) => g.id === selected?.geofence.geofence_id)
  const point: LatLngTuple | undefined = selected ? [selected.location.latitude, selected.location.longitude] : undefined

  return (
    <>
      <PageHeader
        title="Live alerts"
        subtitle="Pushed instantly over WebSocket when a vehicle triggers a configured alert rule"
        extra={
          <>
            <Segmented
              value={filter}
              onChange={(v) => setFilter(v as typeof filter)}
              options={[
                { value: 'all', label: 'All' },
                { value: 'entry', label: 'Entries' },
                { value: 'exit', label: 'Exits' },
              ]}
            />
            <Button icon={<ClearOutlined />} onClick={clear} disabled={feed.length === 0}>
              Clear feed
            </Button>
          </>
        }
      />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
        <Card
          className="xl:col-span-7"
          title={
            <span className="flex items-center gap-2">
              Alert feed <Tag>{items.length}</Tag>
            </span>
          }
          extra={
            <span className="text-xs text-slate-500">
              {status === 'open' ? 'Connected. New alerts appear here instantly.' : 'Connection lost. Reconnecting automatically…'}
            </span>
          }
          styles={{ body: { padding: 0 } }}
        >
          {items.length === 0 ? (
            <div className="py-12">
              <Empty
                description={
                  <span>
                    No alerts yet.
                    <br />
                    Create an alert rule, then move a vehicle across that geofence.
                  </span>
                }
              >
                <div className="flex justify-center gap-2">
                  <Button onClick={() => navigate('/alert-rules')}>Configure rules</Button>
                  <Button type="primary" onClick={() => navigate('/tracking')}>
                    Move a vehicle
                  </Button>
                </div>
              </Empty>
            </div>
          ) : (
            <div className="max-h-[calc(100vh-240px)] min-h-[300px] overflow-auto">
              {items.map((a) => (
                <AlertRow key={a.event_id} alert={a} live={liveIds.has(a.event_id)} active={a.event_id === selected?.event_id} onClick={() => setSelectedId(a.event_id)} />
              ))}
            </div>
          )}
        </Card>

        <Card className="xl:col-span-5 self-start" styles={{ body: { padding: 12 } }}>
          {selected && point ? (
            <>
              <BaseMap className="h-[340px]" fitTo={selectedGeofence ? [...selectedGeofence.coordinates, point] : [point]} fitKey={selected.event_id}>
                {selectedGeofence && <GeofenceLayer geofences={[selectedGeofence]} highlightId={selectedGeofence.id} />}
                <CircleMarker center={point} radius={9} pathOptions={{ color: '#fff', weight: 3, fillColor: selected.event_type === 'entry' ? '#ea580c' : '#2563eb', fillOpacity: 1 }}>
                  <Tooltip permanent direction="top" offset={[0, -8]}>
                    {selected.vehicle.vehicle_number}
                  </Tooltip>
                </CircleMarker>
              </BaseMap>
              <div className="mt-3 grid grid-cols-2 gap-2 text-sm">
                <div className="text-slate-500">Event ID</div>
                <Typography.Text copyable className="text-xs">{selected.event_id}</Typography.Text>
                <div className="text-slate-500">Vehicle</div>
                <div>
                  {selected.vehicle.vehicle_number} ({selected.vehicle.vehicle_id})
                </div>
                <div className="text-slate-500">Driver</div>
                <div>{selected.vehicle.driver_name}</div>
                <div className="text-slate-500">Geofence</div>
                <div>
                  {selected.geofence.geofence_name} <CategoryTag category={selected.geofence.category} />
                </div>
                <div className="text-slate-500">Location</div>
                <div>{formatLatLng(selected.location.latitude, selected.location.longitude)}</div>
                <div className="text-slate-500">Time</div>
                <div>{formatDateTime(selected.timestamp)}</div>
              </div>
            </>
          ) : (
            <Empty className="py-12" description="Select an alert to see it on the map" />
          )}
        </Card>
      </div>
    </>
  )
}

function AlertRow({ alert, live, active, onClick }: { alert: AlertMessage; live: boolean; active: boolean; onClick: () => void }) {
  const entry = alert.event_type === 'entry'
  const restricted = alert.geofence.category === 'restricted_zone'
  return (
    <div
      onClick={onClick}
      className={`flex cursor-pointer gap-3 border-b border-slate-100 px-4 py-3 ${live ? 'alert-flash' : ''}`}
      style={{ background: active ? '#eff6ff' : undefined }}
    >
      <div
        className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-base"
        style={{ background: entry ? (restricted ? '#fee2e2' : '#ffedd5') : '#dbeafe', color: entry ? (restricted ? '#dc2626' : '#ea580c') : '#2563eb' }}
      >
        {entry ? <LoginOutlined /> : <LogoutOutlined />}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">
            {alert.vehicle.vehicle_number} {entry ? 'entered' : 'exited'} {alert.geofence.geofence_name}
          </span>
          {live && <Tag color="green">live</Tag>}
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-1">
          <EventTag type={alert.event_type} />
          <CategoryTag category={alert.geofence.category} />
          <span className="text-xs text-slate-500">Driver: {alert.vehicle.driver_name}</span>
        </div>
        <div className="mt-1 flex flex-wrap gap-x-4 text-xs text-slate-500">
          <span title={alert.timestamp}>
            {formatDateTime(alert.timestamp)} · {fromNow(alert.timestamp)}
          </span>
          <span>
            <EnvironmentOutlined /> {formatLatLng(alert.location.latitude, alert.location.longitude)}
          </span>
        </div>
      </div>
    </div>
  )
}
