import { CaretRightOutlined, ClearOutlined, PauseOutlined, SendOutlined } from '@ant-design/icons'
import { App, Button, Card, DatePicker, Empty, Form, InputNumber, Select, Switch, Tabs, Tag, Typography } from 'antd'
import dayjs, { type Dayjs } from 'dayjs'
import { useEffect, useMemo, useRef, useState } from 'react'
import { CircleMarker, Polyline } from 'react-leaflet'
import { useSearchParams } from 'react-router'
import { errorMessage } from '../api/client'
import { api } from '../api/endpoints'
import type { GeofenceRef, LatLngTuple, LocationUpdateResponse } from '../api/types'
import { PageHeader } from '../components/common'
import { BaseMap } from '../components/map/BaseMap'
import { ClickToPick } from '../components/map/ClickToPick'
import { GeofenceLayer } from '../components/map/GeofenceLayer'
import { VehicleMarkers } from '../components/map/VehicleMarkers'
import { useInterval, useRequest } from '../hooks/useRequest'
import { useAlertStore } from '../store/alertStore'
import { formatLatLng, interpolate } from '../utils/coords'
import { formatNs } from '../utils/format'

interface UpdateResult {
  vehicleNumber: string
  response: LocationUpdateResponse
  entered: { id: string; name: string }[]
  exited: { id: string; name: string }[]
}

/** Compare geofence names before/after an update so the UI can say "entered X / exited Y". */
function diffNames(before: GeofenceRef[], after: LocationUpdateResponse) {
  const prev = new Map(before.map((g) => [g.geofence_id, g.geofence_name]))
  const now = new Map(after.current_geofences.map((g) => [g.geofence_id, g.geofence_name]))
  return {
    entered: [...now].filter(([id]) => !prev.has(id)).map(([id, name]) => ({ id, name })),
    exited: [...prev].filter(([id]) => !now.has(id)).map(([id, name]) => ({ id, name })),
  }
}

export function TrackingPage() {
  const { message } = App.useApp()
  const [params] = useSearchParams()
  const [vehicleId, setVehicleId] = useState<string | undefined>(params.get('vehicle') ?? undefined)
  const [tab, setTab] = useState('manual')
  const revision = useAlertStore((s) => s.revision)

  const geofences = useRequest(() => api.listGeofences(), [])
  const fleet = useRequest(() => api.listVehicleLocations(), [])
  useInterval(() => void fleet.reload(true), 4_000) // pick up updates made from other tabs / curl
  useEffect(() => {
    if (revision > 0) void fleet.reload(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revision])

  const vehicles = useMemo(() => fleet.data?.vehicles ?? [], [fleet.data])
  const selected = vehicles.find((v) => v.vehicle_id === vehicleId)

  // ---- manual update state ----
  const [point, setPoint] = useState<LatLngTuple | null>(null)
  const [useNow, setUseNow] = useState(true)
  const [timestamp, setTimestamp] = useState<Dayjs>(dayjs())
  const [sending, setSending] = useState(false)
  const [result, setResult] = useState<UpdateResult | null>(null)

  // ---- simulator state ----
  const [waypoints, setWaypoints] = useState<LatLngTuple[]>([])
  const [steps, setSteps] = useState(6)
  const [intervalMs, setIntervalMs] = useState(1000)
  const [running, setRunning] = useState(false)
  const [progress, setProgress] = useState(0)
  const stopRef = useRef(false)

  const fitPoints = useMemo(() => {
    const pts = (geofences.data?.geofences ?? []).flatMap((g) => g.coordinates)
    vehicles.forEach((v) => v.current_location && pts.push([v.current_location.latitude, v.current_location.longitude]))
    return pts
  }, [geofences.data, vehicles])

  const vehicleOptions = vehicles.map((v) => ({ value: v.vehicle_id, label: `${v.vehicle_number} · ${v.driver_name}` }))

  /** Sends ONE update to POST /vehicles/location (README endpoint 5). */
  const send = async (lat: number, lng: number, ts: string) => {
    if (!vehicleId) throw new Error('Select a vehicle first')
    // Fresh "before" state from GET /vehicles/location/{id} (README endpoint 6), so the
    // entered/exited summary is correct even if the vehicle was moved from elsewhere.
    const before = await api.getVehicleLocation(vehicleId)
    const response = await api.updateLocation({ vehicle_id: vehicleId, latitude: lat, longitude: lng, timestamp: ts })
    const { entered, exited } = diffNames(before.current_geofences, response)
    setResult({ vehicleNumber: before.vehicle_number, response, entered, exited })
    void fleet.reload(true)
    return response
  }

  const submitManual = async () => {
    if (!vehicleId) return message.warning('Select a vehicle')
    if (!point) return message.warning('Click on the map or enter latitude and longitude')
    setSending(true)
    try {
      await send(point[0], point[1], (useNow ? dayjs() : timestamp).toISOString())
    } catch (e) {
      message.error(errorMessage(e))
    } finally {
      setSending(false)
    }
  }

  const runTrip = async () => {
    if (!vehicleId) return message.warning('Select a vehicle')
    if (waypoints.length < 2) return message.warning('Add at least 2 waypoints by clicking the map')
    const path: LatLngTuple[] = [waypoints[0]]
    for (let i = 1; i < waypoints.length; i++) path.push(...interpolate(waypoints[i - 1], waypoints[i], steps))

    stopRef.current = false
    setRunning(true)
    setProgress(0)
    try {
      for (let i = 0; i < path.length; i++) {
        if (stopRef.current) break
        await send(path[i][0], path[i][1], new Date().toISOString())
        setProgress(Math.round(((i + 1) / path.length) * 100))
        if (i < path.length - 1) await new Promise((r) => setTimeout(r, intervalMs))
      }
    } catch (e) {
      message.error(errorMessage(e))
    } finally {
      setRunning(false)
    }
  }

  const onMapClick = (p: LatLngTuple) => {
    if (tab === 'manual') setPoint(p)
    else if (!running) setWaypoints((w) => [...w, p])
  }

  return (
    <>
      <PageHeader title="Live tracking" subtitle="Update vehicle positions and watch geofence status change in real time" />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
        <Card className="xl:col-span-8" styles={{ body: { padding: 12 } }}>
          <BaseMap className="h-[420px] xl:h-[calc(100vh-210px)] xl:min-h-[520px]" fitTo={fitPoints} fitKey={geofences.data ? 'ready' : 'loading'}>
            <GeofenceLayer geofences={geofences.data?.geofences ?? []} />
            <VehicleMarkers vehicles={vehicles} selectedId={vehicleId} onSelect={(v) => setVehicleId(v.vehicle_id)} />
            <ClickToPick value={tab === 'manual' ? point : null} onPick={onMapClick} />
            {tab === 'simulate' && waypoints.length > 0 && (
              <>
                <Polyline positions={waypoints} pathOptions={{ color: '#f59e0b', dashArray: '6 6', weight: 3 }} />
                {waypoints.map((w, i) => (
                  <CircleMarker key={i} center={w} radius={5} pathOptions={{ color: '#f59e0b', fillOpacity: 1 }} />
                ))}
              </>
            )}
          </BaseMap>
          <div className="mt-2 text-xs text-slate-500">
            {tab === 'manual' ? 'Click the map to choose the new position.' : 'Click the map to add waypoints for the simulated trip.'} Markers:{' '}
            <span style={{ color: '#dc2626' }}>●</span> in restricted zone · <span style={{ color: '#2563eb' }}>●</span> in a geofence ·{' '}
            <span style={{ color: '#475569' }}>●</span> outside
          </div>
        </Card>

        <div className="flex flex-col gap-4 xl:col-span-4">
          <Card styles={{ body: { padding: 16 } }}>
            <Typography.Text strong>Vehicle</Typography.Text>
            <Select
              className="mt-1 w-full"
              showSearch={{ optionFilterProp: 'label' }}
              placeholder={fleet.loading ? 'Loading vehicles…' : 'Select a vehicle'}
              options={vehicleOptions}
              value={vehicleId}
              onChange={setVehicleId}
              notFoundContent={<Empty description="No vehicles. Register one first." />}
            />
            {selected && (
              <div className="mt-3 rounded-lg bg-slate-50 p-3 text-sm">
                <div>
                  <span className="text-slate-500">Position: </span>
                  {selected.current_location ? formatLatLng(selected.current_location.latitude, selected.current_location.longitude) : 'no location yet'}
                </div>
                <div className="mt-1">
                  <span className="text-slate-500">Inside: </span>
                  {selected.current_geofences.length === 0
                    ? 'no geofence'
                    : selected.current_geofences.map((g) => (
                        <Tag key={g.geofence_id} color={g.category === 'restricted_zone' ? 'red' : 'blue'}>
                          {g.geofence_name}
                        </Tag>
                      ))}
                </div>
              </div>
            )}
          </Card>

          <Card styles={{ body: { padding: 16 } }}>
            <Tabs
              activeKey={tab}
              onChange={setTab}
              items={[
                {
                  key: 'manual',
                  label: 'Update location',
                  children: (
                    <Form layout="vertical" onFinish={submitManual}>
                      <div className="grid grid-cols-2 gap-3">
                        <Form.Item label="Latitude" required htmlFor="loc-lat">
                          <InputNumber id="loc-lat" className="w-full" min={-90} max={90} step={0.0001} value={point?.[0]} placeholder="37.7849"
                            onChange={(v) => setPoint([v ?? 0, point?.[1] ?? 0])} />
                        </Form.Item>
                        <Form.Item label="Longitude" required htmlFor="loc-lng">
                          <InputNumber id="loc-lng" className="w-full" min={-180} max={180} step={0.0001} value={point?.[1]} placeholder="-122.4194"
                            onChange={(v) => setPoint([point?.[0] ?? 0, v ?? 0])} />
                        </Form.Item>
                      </div>
                      <Form.Item label="Timestamp">
                        <div className="flex items-center gap-2">
                          <Switch checked={useNow} onChange={setUseNow} /> <span className="text-sm">Use current time</span>
                        </div>
                        {!useNow && (
                          <DatePicker showTime className="mt-2 w-full" value={timestamp} onChange={(d) => d && setTimestamp(d)} />
                        )}
                      </Form.Item>
                      <Button type="primary" htmlType="submit" icon={<SendOutlined />} loading={sending} block>
                        Send location update
                      </Button>
                    </Form>
                  ),
                },
                {
                  key: 'simulate',
                  label: 'Simulate trip',
                  children: (
                    <div className="flex flex-col gap-3">
                      <Typography.Text type="secondary" className="text-sm">
                        Click the map to add waypoints. The simulator sends a location update every interval, moving along the
                        path, which is ideal for showing entry/exit alerts live.
                      </Typography.Text>
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <div className="mb-1 text-xs text-slate-500">Steps per segment</div>
                          <InputNumber className="w-full" min={1} max={50} value={steps} onChange={(v) => setSteps(v ?? 6)} disabled={running} />
                        </div>
                        <div>
                          <div className="mb-1 text-xs text-slate-500">Interval (ms)</div>
                          <InputNumber className="w-full" min={250} max={10000} step={250} value={intervalMs} onChange={(v) => setIntervalMs(v ?? 1000)} disabled={running} />
                        </div>
                      </div>
                      <div className="text-sm">
                        Waypoints: <b>{waypoints.length}</b>
                        {running && <span className="ml-2 text-slate-500">· {progress}% sent</span>}
                      </div>
                      <div className="flex gap-2">
                        {running ? (
                          <Button danger icon={<PauseOutlined />} onClick={() => (stopRef.current = true)} block>
                            Stop
                          </Button>
                        ) : (
                          <Button type="primary" icon={<CaretRightOutlined />} onClick={runTrip} block disabled={waypoints.length < 2}>
                            Start trip
                          </Button>
                        )}
                        <Button icon={<ClearOutlined />} onClick={() => setWaypoints([])} disabled={running}>
                          Clear
                        </Button>
                      </div>
                    </div>
                  ),
                },
              ]}
            />
          </Card>

          {result && (
            <Card size="small" title="Last update result" extra={<span className="text-xs text-slate-500">server: {formatNs(result.response.time_ns)}</span>}>
              <div className="text-sm">
                <div className="mb-2">
                  <b>{result.vehicleNumber}</b> · location_updated: <Tag color="green">{String(result.response.location_updated)}</Tag>
                </div>
                {result.entered.map((g) => (
                  <div key={'e' + g.id} className="text-orange-600">↘ Entered {g.name}</div>
                ))}
                {result.exited.map((g) => (
                  <div key={'x' + g.id} className="text-blue-600">↗ Exited {g.name}</div>
                ))}
                <div className="mt-2 text-slate-500">current_geofences:</div>
                {result.response.current_geofences.length === 0 ? (
                  <div>Not inside any geofence</div>
                ) : (
                  result.response.current_geofences.map((g) => (
                    <div key={g.geofence_id}>
                      <Tag color="blue">{g.status}</Tag> {g.geofence_name}
                    </div>
                  ))
                )}
              </div>
            </Card>
          )}
        </div>
      </div>
    </>
  )
}
