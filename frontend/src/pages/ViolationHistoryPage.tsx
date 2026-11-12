import { EnvironmentOutlined, ReloadOutlined } from '@ant-design/icons'
import { Button, Card, DatePicker, Empty, Modal, Select, Table, Tag } from 'antd'
import type { Dayjs } from 'dayjs'
import { useEffect, useState } from 'react'
import { CircleMarker } from 'react-leaflet'
import { api } from '../api/endpoints'
import type { EventType, Violation } from '../api/types'
import { CategoryTag, EventTag, LoadError, PageHeader } from '../components/common'
import { BaseMap } from '../components/map/BaseMap'
import { GeofenceLayer } from '../components/map/GeofenceLayer'
import { useRequest } from '../hooks/useRequest'
import { useAppSelector } from '../store/hooks'
import { formatLatLng } from '../utils/coords'
import { formatDateTime, formatNs } from '../utils/format'

export function ViolationHistoryPage() {
  const [vehicleId, setVehicleId] = useState<string | undefined>()
  const [geofenceId, setGeofenceId] = useState<string | undefined>()
  const [range, setRange] = useState<[Dayjs | null, Dayjs | null] | null>(null)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [mapItem, setMapItem] = useState<Violation | null>(null)
  const revision = useAppSelector((s) => s.alerts.revision)

  const vehicles = useRequest(() => api.listVehicles(), [])
  const geofences = useRequest(() => api.listGeofences(), [])

  const startDate = range?.[0]?.toISOString()
  const endDate = range?.[1]?.toISOString()

  // Server-side filtering + pagination: GET /violations/history?…&limit=&offset=
  const history = useRequest(
    () =>
      api.violationHistory({
        vehicle_id: vehicleId,
        geofence_id: geofenceId,
        start_date: startDate,
        end_date: endDate,
        limit: pageSize,
        offset: (page - 1) * pageSize,
      }),
    [vehicleId, geofenceId, startDate, endDate, page, pageSize],
  )

  // New live alerts usually mean new history rows; refresh quietly when on page 1.
  useEffect(() => {
    if (revision > 0 && page === 1) void history.reload(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revision])

  const resetPage = <T,>(setter: (v: T) => void) => (v: T) => {
    setter(v)
    setPage(1)
  }
  const hasFilters = !!(vehicleId || geofenceId || range)
  const mapGeofence = geofences.data?.geofences.find((g) => g.id === mapItem?.geofence_id)

  return (
    <>
      <PageHeader
        title="Violation history"
        subtitle="Every recorded geofence entry and exit"
        extra={
          <Button icon={<ReloadOutlined />} onClick={() => history.reload()}>
            Refresh
          </Button>
        }
      />

      <Card styles={{ body: { padding: 16 } }}>
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <Select
            allowClear
            showSearch={{ optionFilterProp: 'label' }}
            placeholder="All vehicles"
            className="min-w-[200px]"
            loading={vehicles.loading}
            value={vehicleId}
            onChange={resetPage(setVehicleId)}
            options={(vehicles.data?.vehicles ?? []).map((v) => ({ value: v.id, label: v.vehicle_number }))}
          />
          <Select
            allowClear
            showSearch={{ optionFilterProp: 'label' }}
            placeholder="All geofences"
            className="min-w-[200px]"
            loading={geofences.loading}
            value={geofenceId}
            onChange={resetPage(setGeofenceId)}
            options={(geofences.data?.geofences ?? []).map((g) => ({ value: g.id, label: g.name }))}
          />
          <DatePicker.RangePicker showTime value={range} onChange={resetPage(setRange)} allowEmpty={[true, true]} />
          {hasFilters && (
            <Button
              type="link"
              onClick={() => {
                setVehicleId(undefined)
                setGeofenceId(undefined)
                setRange(null)
                setPage(1)
              }}
            >
              Reset filters
            </Button>
          )}
          <div className="ml-auto text-xs text-slate-500">
            {history.data && (
              <>
                <b>{history.data.total_count}</b> events · server {formatNs(history.data.time_ns)}
              </>
            )}
          </div>
        </div>

        {history.error ? (
          <LoadError error={history.error} onRetry={() => history.reload()} />
        ) : (
          <Table<Violation>
            rowKey="id"
            loading={history.loading}
            dataSource={history.data?.violations ?? []}
            scroll={{ x: 900 }}
            locale={{ emptyText: <Empty description={hasFilters ? 'No events match these filters' : 'No geofence events recorded yet'} /> }}
            pagination={{
              current: page,
              pageSize,
              total: history.data?.total_count ?? 0,
              showSizeChanger: true,
              pageSizeOptions: [10, 20, 50, 100],
              showTotal: (total, [from, to]) => `${from}–${to} of ${total}`,
              onChange: (p, size) => {
                setPage(size !== pageSize ? 1 : p)
                setPageSize(size)
              },
            }}
            columns={[
              { title: 'Time', dataIndex: 'timestamp', render: (t: string) => formatDateTime(t), width: 190 },
              {
                title: 'Vehicle',
                dataIndex: 'vehicle_number',
                render: (n: string, v) => (
                  <div>
                    <div className="font-medium">{n}</div>
                    <div className="text-xs text-slate-500">{v.vehicle_id}</div>
                  </div>
                ),
              },
              {
                title: 'Geofence',
                dataIndex: 'geofence_name',
                render: (n: string, v) => (
                  <div className="flex flex-wrap items-center gap-1">
                    <span>{n}</span>
                    {v.category && <CategoryTag category={v.category} />}
                  </div>
                ),
              },
              { title: 'Event', dataIndex: 'event_type', render: (t: EventType) => <EventTag type={t} />, width: 90 },
              { title: 'Location', render: (_, v) => <span className="text-xs">{formatLatLng(v.latitude, v.longitude)}</span> },
              { title: 'Event ID', dataIndex: 'id', render: (id: string) => <Tag className="text-xs">{id}</Tag> },
              {
                title: '',
                width: 60,
                render: (_, v) => <Button size="small" icon={<EnvironmentOutlined />} onClick={() => setMapItem(v)} aria-label="Show on map" />,
              },
            ]}
          />
        )}
      </Card>

      <Modal open={!!mapItem} onCancel={() => setMapItem(null)} footer={null} title={mapItem ? `${mapItem.vehicle_number} · ${mapItem.event_type} · ${mapItem.geofence_name}` : ''} width={720} destroyOnHidden>
        {mapItem && (
          <BaseMap className="h-[420px]" fitTo={[...(mapGeofence?.coordinates ?? []), [mapItem.latitude, mapItem.longitude]]} fitKey={mapItem.id}>
            {mapGeofence && <GeofenceLayer geofences={[mapGeofence]} highlightId={mapGeofence.id} />}
            <CircleMarker center={[mapItem.latitude, mapItem.longitude]} radius={9} pathOptions={{ color: '#fff', weight: 3, fillColor: mapItem.event_type === 'entry' ? '#ea580c' : '#2563eb', fillOpacity: 1 }} />
          </BaseMap>
        )}
      </Modal>
    </>
  )
}
