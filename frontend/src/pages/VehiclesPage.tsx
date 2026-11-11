import { AimOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { App, AutoComplete, Button, Card, Descriptions, Drawer, Empty, Form, Input, Modal, Table, Tag, Typography } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import { ApiError, errorMessage } from '../api/client'
import { api } from '../api/endpoints'
import type { CreateVehicleRequest, GeofenceRef, Vehicle, VehicleStatus } from '../api/types'
import { CategoryTag, EventTag, LoadError, PageHeader } from '../components/common'
import { BaseMap } from '../components/map/BaseMap'
import { GeofenceLayer } from '../components/map/GeofenceLayer'
import { VehicleMarkers } from '../components/map/VehicleMarkers'
import { VEHICLE_TYPES } from '../constants'
import { useInterval, useRequest } from '../hooks/useRequest'
import { useAlertStore } from '../store/alertStore'
import { formatLatLng } from '../utils/coords'
import { formatDateTime, formatNs, fromNow } from '../utils/format'

type Row = Vehicle & { live?: VehicleStatus }

export function GeofenceStatusTags({ refs, hasLocation }: { refs: GeofenceRef[]; hasLocation: boolean }) {
  if (!hasLocation) return <Tag>No location yet</Tag>
  if (refs.length === 0) return <Tag color="default">Outside all geofences</Tag>
  return (
    <div className="flex flex-wrap gap-1">
      {refs.map((g) => (
        <CategoryTag key={g.geofence_id} category={g.category} />
      ))}
      <span className="text-xs text-slate-600">{refs.map((g) => g.geofence_name).join(', ')}</span>
    </div>
  )
}

export function VehiclesPage() {
  const navigate = useNavigate()
  const [registering, setRegistering] = useState(false)
  const [detailsId, setDetailsId] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const revision = useAlertStore((s) => s.revision)

  const vehicles = useRequest(() => api.listVehicles(), [])
  const live = useRequest(() => api.listVehicleLocations(), [])
  useInterval(() => void live.reload(true), 10_000)
  useEffect(() => {
    if (revision > 0) void live.reload(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revision])

  const rows: Row[] = useMemo(() => {
    const byId = new Map((live.data?.vehicles ?? []).map((v) => [v.vehicle_id, v]))
    const q = search.trim().toLowerCase()
    return (vehicles.data?.vehicles ?? [])
      .filter((v) => !q || v.vehicle_number.toLowerCase().includes(q) || v.driver_name.toLowerCase().includes(q))
      .map((v) => ({ ...v, live: byId.get(v.id) }))
  }, [vehicles.data, live.data, search])

  const reloadAll = () => {
    void vehicles.reload()
    void live.reload()
  }

  return (
    <>
      <PageHeader
        title="Vehicles"
        subtitle="Registered fleet with current location and geofence status"
        extra={
          <>
            <Button icon={<ReloadOutlined />} onClick={reloadAll}>
              Refresh
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setRegistering(true)}>
              Register vehicle
            </Button>
          </>
        }
      />

      {vehicles.error ? (
        <LoadError error={vehicles.error} onRetry={reloadAll} />
      ) : (
        <Card styles={{ body: { padding: 16 } }}>
          <Input
            prefix={<SearchOutlined />}
            placeholder="Search by vehicle number or driver"
            allowClear
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="mb-3 max-w-sm"
          />
          <Table<Row>
            rowKey="id"
            loading={vehicles.loading}
            dataSource={rows}
            scroll={{ x: 980 }}
            pagination={{ pageSize: 10, hideOnSinglePage: true }}
            locale={{ emptyText: <Empty description="No vehicles registered yet" /> }}
            onRow={(v) => ({ onClick: () => setDetailsId(v.id), style: { cursor: 'pointer' } })}
            columns={[
              {
                title: 'Vehicle',
                dataIndex: 'vehicle_number',
                render: (n: string, v) => (
                  <div>
                    <div className="font-medium">{n}</div>
                    <div className="text-xs text-slate-500">{v.id}</div>
                  </div>
                ),
              },
              { title: 'Driver', dataIndex: 'driver_name' },
              { title: 'Phone', dataIndex: 'phone' },
              { title: 'Type', dataIndex: 'vehicle_type', render: (t: string) => <Tag>{t}</Tag> },
              { title: 'Status', dataIndex: 'status', render: (s: string) => <Tag color={s === 'active' ? 'green' : 'default'}>{s}</Tag> },
              {
                title: 'Current location',
                render: (_, v) =>
                  v.live?.current_location ? (
                    <div>
                      <div className="text-xs">{formatLatLng(v.live.current_location.latitude, v.live.current_location.longitude)}</div>
                      <div className="text-xs text-slate-500">{fromNow(v.live.current_location.timestamp)}</div>
                    </div>
                  ) : (
                    <span className="text-slate-400">—</span>
                  ),
              },
              {
                title: 'Geofence status',
                render: (_, v) => <GeofenceStatusTags refs={v.live?.current_geofences ?? []} hasLocation={!!v.live?.current_location} />,
              },
              {
                title: '',
                width: 90,
                render: (_, v) => (
                  <Button
                    size="small"
                    icon={<AimOutlined />}
                    onClick={(e) => {
                      e.stopPropagation()
                      navigate(`/tracking?vehicle=${v.id}`)
                    }}
                  >
                    Move
                  </Button>
                ),
              },
            ]}
          />
        </Card>
      )}

      <RegisterVehicleModal
        open={registering}
        onClose={() => setRegistering(false)}
        onCreated={() => {
          setRegistering(false)
          reloadAll()
        }}
      />
      <VehicleDetails vehicleId={detailsId} vehicle={rows.find((r) => r.id === detailsId)} onClose={() => setDetailsId(null)} />
    </>
  )
}

function RegisterVehicleModal({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: () => void }) {
  const { message } = App.useApp()
  const [form] = Form.useForm<CreateVehicleRequest>()
  const [submitting, setSubmitting] = useState(false)

  const submit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const res = await api.createVehicle(values)
      message.success(`Vehicle ${res.vehicle_number} registered (${formatNs(res.time_ns)})`)
      form.resetFields()
      onCreated()
    } catch (e) {
      if (e instanceof ApiError && Object.keys(e.fields).length > 0) {
        form.setFields(Object.entries(e.fields).map(([name, err]) => ({ name: name as keyof CreateVehicleRequest, errors: [err] })))
      } else if (e instanceof ApiError && e.status === 409) {
        form.setFields([{ name: 'vehicle_number', errors: [e.message] }])
      } else {
        message.error(errorMessage(e))
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal open={open} title="Register vehicle" onCancel={onClose} onOk={submit} okText="Register" confirmLoading={submitting} destroyOnHidden>
      <Form form={form} layout="vertical" requiredMark="optional" initialValues={{ vehicle_type: 'truck' }}>
        <Form.Item name="vehicle_number" label="Vehicle number" rules={[{ required: true, whitespace: true, message: 'Vehicle number is required' }, { max: 32 }]}>
          <Input placeholder="KA-01-AB-1234" />
        </Form.Item>
        <Form.Item name="driver_name" label="Driver name" rules={[{ required: true, whitespace: true, message: 'Driver name is required' }, { max: 100 }]}>
          <Input placeholder="John Doe" />
        </Form.Item>
        <Form.Item name="vehicle_type" label="Vehicle type" rules={[{ required: true, whitespace: true, message: 'Vehicle type is required' }, { max: 30 }]}>
          <AutoComplete options={VEHICLE_TYPES.map((t) => ({ value: t }))} placeholder="truck, car, van, motorcycle…" />
        </Form.Item>
        <Form.Item
          name="phone"
          label="Driver phone"
          rules={[
            { required: true, message: 'Phone is required' },
            { pattern: /^\+?[0-9][0-9 ()-]{5,18}[0-9]$/, message: 'Enter a valid phone number, e.g. +1234567890' },
          ]}
        >
          <Input placeholder="+1234567890" />
        </Form.Item>
      </Form>
    </Modal>
  )
}

function VehicleDetails({ vehicleId, vehicle, onClose }: { vehicleId: string | null; vehicle?: Row; onClose: () => void }) {
  const navigate = useNavigate()
  // README endpoint 6: GET /vehicles/location/{vehicle_id}
  const loc = useRequest(() => (vehicleId ? api.getVehicleLocation(vehicleId) : Promise.resolve(undefined)), [vehicleId])
  const events = useRequest(() => (vehicleId ? api.violationHistory({ vehicle_id: vehicleId, limit: 8 }) : Promise.resolve(undefined)), [vehicleId])
  const geofences = useRequest(() => (vehicleId ? api.listGeofences() : Promise.resolve(undefined)), [vehicleId])

  const current = loc.data?.current_location
  const status: VehicleStatus | undefined =
    loc.data && vehicle
      ? { ...loc.data, driver_name: vehicle.driver_name, vehicle_type: vehicle.vehicle_type, status: vehicle.status }
      : undefined

  return (
    <Drawer open={!!vehicleId} onClose={onClose} size={600} title={vehicle?.vehicle_number ?? 'Vehicle'} destroyOnHidden
      extra={vehicleId && <Button type="primary" icon={<AimOutlined />} onClick={() => navigate(`/tracking?vehicle=${vehicleId}`)}>Update location</Button>}
    >
      {loc.error && <LoadError error={loc.error} onRetry={() => loc.reload()} />}
      {vehicle && (
        <div className="flex flex-col gap-4">
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="ID">
              <Typography.Text copyable>{vehicle.id}</Typography.Text>
            </Descriptions.Item>
            <Descriptions.Item label="Driver">{vehicle.driver_name}</Descriptions.Item>
            <Descriptions.Item label="Phone">{vehicle.phone}</Descriptions.Item>
            <Descriptions.Item label="Type">{vehicle.vehicle_type}</Descriptions.Item>
            <Descriptions.Item label="Registered">{formatDateTime(vehicle.created_at)}</Descriptions.Item>
            <Descriptions.Item label="Current location">
              {current ? `${formatLatLng(current.latitude, current.longitude)} · ${formatDateTime(current.timestamp)}` : loc.loading ? 'Loading…' : 'No location reported yet'}
            </Descriptions.Item>
            <Descriptions.Item label="Geofence status">
              <GeofenceStatusTags refs={loc.data?.current_geofences ?? []} hasLocation={!!current} />
            </Descriptions.Item>
          </Descriptions>

          {current && status && (
            <BaseMap className="h-[260px]" fitTo={[[current.latitude, current.longitude]]} fitKey={vehicleId ?? ''}>
              <GeofenceLayer geofences={geofences.data?.geofences ?? []} />
              <VehicleMarkers vehicles={[status]} selectedId={vehicleId ?? undefined} />
            </BaseMap>
          )}

          <div>
            <Typography.Title level={5}>Recent geofence events</Typography.Title>
            <Table
              size="small"
              rowKey="id"
              loading={events.loading}
              dataSource={events.data?.violations ?? []}
              pagination={false}
              locale={{ emptyText: 'No entry/exit events yet' }}
              columns={[
                { title: 'Time', dataIndex: 'timestamp', render: (t: string) => <span className="text-xs">{formatDateTime(t)}</span> },
                { title: 'Event', dataIndex: 'event_type', render: (t: 'entry' | 'exit') => <EventTag type={t} /> },
                { title: 'Geofence', dataIndex: 'geofence_name' },
              ]}
            />
          </div>
        </div>
      )}
    </Drawer>
  )
}
