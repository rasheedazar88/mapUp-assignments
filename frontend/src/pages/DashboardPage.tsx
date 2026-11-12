import { AlertOutlined, CarOutlined, GatewayOutlined, HistoryOutlined } from '@ant-design/icons'
import { Button, Card, Empty, Progress, Table, Tag } from 'antd'
import dayjs from 'dayjs'
import { useEffect, useMemo } from 'react'
import { useNavigate } from 'react-router'
import { api } from '../api/endpoints'
import type { EventType, Violation } from '../api/types'
import { CategoryTag, EventTag, LoadError, PageHeader, StatCard } from '../components/common'
import { BaseMap } from '../components/map/BaseMap'
import { GeofenceLayer } from '../components/map/GeofenceLayer'
import { VehicleMarkers } from '../components/map/VehicleMarkers'
import { useInterval, useRequest } from '../hooks/useRequest'
import { useAppSelector } from '../store/hooks'
import { formatDateTime, fromNow } from '../utils/format'

export function DashboardPage() {
  const navigate = useNavigate()
  const revision = useAppSelector((s) => s.alerts.revision)
  const feed = useAppSelector((s) => s.alerts.feed)

  const geofences = useRequest(() => api.listGeofences(), [])
  const fleet = useRequest(() => api.listVehicleLocations(), [])
  const rules = useRequest(() => api.listAlerts(), [])
  const recent = useRequest(() => api.violationHistory({ limit: 8 }), [])
  const last24h = useRequest(() => api.violationHistory({ start_date: dayjs().subtract(24, 'hour').toISOString(), limit: 1 }), [])

  const refreshLive = () => {
    void fleet.reload(true)
    void recent.reload(true)
    void last24h.reload(true)
  }
  useInterval(refreshLive, 15_000)
  useEffect(() => {
    if (revision > 0) refreshLive()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revision])

  const vehicles = useMemo(() => fleet.data?.vehicles ?? [], [fleet.data])
  const allGeofences = useMemo(() => geofences.data?.geofences ?? [], [geofences.data])
  const activeRules = (rules.data?.alerts ?? []).filter((r) => r.status === 'active').length
  const restrictedCount = allGeofences.filter((g) => g.category === 'restricted_zone').length

  const overview = useMemo(() => {
    const noLocation = vehicles.filter((v) => !v.current_location)
    const inRestricted = vehicles.filter((v) => v.current_geofences.some((g) => g.category === 'restricted_zone'))
    const inZone = vehicles.filter((v) => v.current_geofences.length > 0 && !inRestricted.includes(v))
    const outside = vehicles.filter((v) => v.current_location && v.current_geofences.length === 0)
    return { noLocation, inRestricted, inZone, outside }
  }, [vehicles])

  const fitPoints = useMemo(() => {
    const pts = allGeofences.flatMap((g) => g.coordinates)
    vehicles.forEach((v) => v.current_location && pts.push([v.current_location.latitude, v.current_location.longitude]))
    return pts
  }, [allGeofences, vehicles])

  if (geofences.error && fleet.error) {
    return <LoadError error={geofences.error} onRetry={() => window.location.reload()} />
  }

  const pct = (n: number) => (vehicles.length ? Math.round((n / vehicles.length) * 100) : 0)

  return (
    <>
      <PageHeader title="Fleet overview" subtitle="Geofences, vehicles and alerts at a glance" />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard title="Vehicles" value={vehicles.length} icon={<CarOutlined />} loading={fleet.loading}
          hint={`${vehicles.length - overview.noLocation.length} reporting location`} />
        <StatCard title="Geofences" value={allGeofences.length} icon={<GatewayOutlined />} color="#16a34a" loading={geofences.loading}
          hint={`${restrictedCount} restricted`} />
        <StatCard title="Active alert rules" value={activeRules} icon={<AlertOutlined />} color="#dc2626" loading={rules.loading}
          hint={`${feed.length} alerts in feed`} />
        <StatCard title="Events (last 24h)" value={last24h.data?.total_count ?? '—'} icon={<HistoryOutlined />} color="#d97706" loading={last24h.loading}
          hint="Entries and exits" />
      </div>

      <div className="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-12">
        <Card title="Live map" className="xl:col-span-8" styles={{ body: { padding: 12 } }} extra={<Button size="small" onClick={() => navigate('/tracking')}>Open tracking</Button>}>
          <BaseMap className="h-[380px]" fitTo={fitPoints} fitKey={geofences.data && fleet.data ? 'ready' : 'loading'}>
            <GeofenceLayer geofences={allGeofences} />
            <VehicleMarkers vehicles={vehicles} />
          </BaseMap>
        </Card>

        <Card title="Vehicle status" className="xl:col-span-4" loading={fleet.loading}>
          {vehicles.length === 0 ? (
            <Empty description="No vehicles registered">
              <Button type="primary" onClick={() => navigate('/vehicles')}>Register a vehicle</Button>
            </Empty>
          ) : (
            <div className="flex flex-col gap-3">
              <StatusBar label="In a restricted zone" count={overview.inRestricted.length} percent={pct(overview.inRestricted.length)} color="#dc2626" />
              <StatusBar label="Inside other geofences" count={overview.inZone.length} percent={pct(overview.inZone.length)} color="#2563eb" />
              <StatusBar label="Outside all geofences" count={overview.outside.length} percent={pct(overview.outside.length)} color="#475569" />
              <StatusBar label="No location yet" count={overview.noLocation.length} percent={pct(overview.noLocation.length)} color="#cbd5e1" />
              {overview.inRestricted.length > 0 && (
                <div className="mt-2 rounded-lg bg-red-50 p-3">
                  <div className="mb-1 text-xs font-medium text-red-700">Currently in restricted zones</div>
                  {overview.inRestricted.map((v) => (
                    <div key={v.vehicle_id} className="text-sm">
                      <b>{v.vehicle_number}</b> → {v.current_geofences.filter((g) => g.category === 'restricted_zone').map((g) => g.geofence_name).join(', ')}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </Card>
      </div>

      <div className="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-12">
        <Card title="Recent violations" className="xl:col-span-7" styles={{ body: { padding: 0 } }}
          extra={<Button size="small" onClick={() => navigate('/history')}>View all</Button>}>
          <Table<Violation>
            rowKey="id"
            size="small"
            loading={recent.loading}
            dataSource={recent.data?.violations ?? []}
            pagination={false}
            scroll={{ x: 560 }}
            locale={{ emptyText: <Empty description="No events yet" /> }}
            columns={[
              { title: 'Time', dataIndex: 'timestamp', render: (t: string) => <span className="text-xs">{formatDateTime(t)}</span> },
              { title: 'Vehicle', dataIndex: 'vehicle_number' },
              { title: 'Geofence', dataIndex: 'geofence_name' },
              { title: 'Event', dataIndex: 'event_type', render: (t: EventType) => <EventTag type={t} /> },
            ]}
          />
        </Card>

        <Card title="Latest alerts" className="xl:col-span-5" extra={<Button size="small" onClick={() => navigate('/alerts')}>Open feed</Button>}>
          {feed.length === 0 ? (
            <Empty description="No alerts yet" />
          ) : (
            <div className="flex flex-col gap-3">
              {feed.slice(0, 5).map((a) => (
                <div key={a.event_id} className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">
                      {a.vehicle.vehicle_number} {a.event_type === 'entry' ? 'entered' : 'exited'} {a.geofence.geofence_name}
                    </div>
                    <div className="mt-0.5 flex items-center gap-1">
                      <CategoryTag category={a.geofence.category} />
                      <span className="text-xs text-slate-500">{fromNow(a.timestamp)}</span>
                    </div>
                  </div>
                  <Tag color={a.event_type === 'entry' ? 'volcano' : 'geekblue'}>{a.event_type}</Tag>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>
    </>
  )
}

function StatusBar({ label, count, percent, color }: { label: string; count: number; percent: number; color: string }) {
  return (
    <div>
      <div className="flex justify-between text-sm">
        <span>{label}</span>
        <b>{count}</b>
      </div>
      <Progress percent={percent} showInfo={false} strokeColor={color} size="small" />
    </div>
  )
}
