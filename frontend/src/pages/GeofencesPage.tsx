import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { App, Button, Card, Checkbox, Descriptions, Drawer, Empty, Form, Input, Select, Table, Tag, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { Polygon } from 'react-leaflet'
import { useNavigate } from 'react-router'
import { ApiError, errorMessage } from '../api/client'
import { api } from '../api/endpoints'
import type { Geofence, GeofenceCategory, LatLngTuple } from '../api/types'
import { CategoryTag, EventTag, LoadError, PageHeader } from '../components/common'
import { BaseMap } from '../components/map/BaseMap'
import { DrawControl } from '../components/map/DrawControl'
import { GeofenceLayer } from '../components/map/GeofenceLayer'
import { CATEGORIES, CATEGORY_OPTIONS } from '../constants'
import { useRequest } from '../hooks/useRequest'
import { formatCoordinates, parseCoordinates } from '../utils/coords'
import { formatDateTime, formatNs } from '../utils/format'

const README_EXAMPLE: LatLngTuple[] = [
  [37.7749, -122.4194],
  [37.7849, -122.4194],
  [37.7849, -122.4094],
  [37.7749, -122.4094],
  [37.7749, -122.4194],
]

export function GeofencesPage() {
  const [category, setCategory] = useState<GeofenceCategory | undefined>()
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Geofence | null>(null)
  const [creating, setCreating] = useState(false)

  // Category filtering is done by the API (GET /geofences?category=…); name search is local.
  const { data, loading, error, reload } = useRequest(() => api.listGeofences(category), [category])
  const geofences = useMemo(
    () => (data?.geofences ?? []).filter((g) => g.name.toLowerCase().includes(search.trim().toLowerCase())),
    [data, search],
  )
  const allPoints = useMemo(() => geofences.flatMap((g) => g.coordinates), [geofences])

  return (
    <>
      <PageHeader
        title="Geofences"
        subtitle="Virtual boundaries that trigger entry and exit events"
        extra={
          <>
            <Button icon={<ReloadOutlined />} onClick={() => reload()}>
              Refresh
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
              New geofence
            </Button>
          </>
        }
      />

      {error ? (
        <LoadError error={error} onRetry={() => reload()} />
      ) : (
        <div className="grid grid-cols-1 gap-4 xl:grid-cols-5">
          <Card className="xl:col-span-2" styles={{ body: { padding: 16 } }}>
            <div className="mb-3 flex flex-wrap gap-2">
              <Input
                prefix={<SearchOutlined />}
                placeholder="Search by name"
                allowClear
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="min-w-[160px] flex-1"
              />
              <Select
                allowClear
                placeholder="All categories"
                options={CATEGORY_OPTIONS}
                value={category}
                onChange={setCategory}
                className="min-w-[170px]"
              />
            </div>
            <Table<Geofence>
              rowKey="id"
              size="middle"
              loading={loading}
              dataSource={geofences}
              pagination={{ pageSize: 8, hideOnSinglePage: true }}
              scroll={{ x: 420 }}
              locale={{ emptyText: <Empty description={category || search ? 'No geofences match the filter' : 'No geofences yet. Create your first one.'} /> }}
              onRow={(g) => ({ onClick: () => setSelected(g), style: { cursor: 'pointer' } })}
              rowClassName={(g) => (g.id === selected?.id ? 'ant-table-row-selected' : '')}
              columns={[
                {
                  title: 'Name',
                  dataIndex: 'name',
                  render: (name: string, g) => (
                    <div>
                      <div className="font-medium">{name}</div>
                      <div className="text-xs text-slate-500">{g.id}</div>
                    </div>
                  ),
                },
                { title: 'Category', dataIndex: 'category', render: (c: GeofenceCategory) => <CategoryTag category={c} /> },
                { title: 'Created', dataIndex: 'created_at', render: (d: string) => <span className="text-xs">{formatDateTime(d)}</span> },
              ]}
            />
          </Card>

          <Card className="xl:col-span-3" styles={{ body: { padding: 12 } }}>
            <BaseMap className="h-[420px] xl:h-[560px]" fitTo={allPoints} fitKey={`${category}-${data?.geofences.length ?? 0}`}>
              <GeofenceLayer geofences={geofences} highlightId={selected?.id} onSelect={setSelected} />
            </BaseMap>
            <div className="mt-3 flex flex-wrap gap-3 text-xs text-slate-600">
              {CATEGORY_OPTIONS.map((c) => (
                <span key={c.value} className="flex items-center gap-1">
                  <span className="inline-block h-3 w-3 rounded-sm" style={{ background: CATEGORIES[c.value].color }} />
                  {c.label}
                </span>
              ))}
            </div>
          </Card>
        </div>
      )}

      <GeofenceDetails geofence={selected} onClose={() => setSelected(null)} />
      <CreateGeofenceDrawer
        open={creating}
        existing={data?.geofences ?? []}
        onClose={() => setCreating(false)}
        onCreated={() => {
          setCreating(false)
          void reload()
        }}
      />
    </>
  )
}

// ---------------------------------------------------------------------------

function GeofenceDetails({ geofence, onClose }: { geofence: Geofence | null; onClose: () => void }) {
  const navigate = useNavigate()
  const id = geofence?.id
  const rules = useRequest(() => (id ? api.listAlerts({ geofence_id: id }) : Promise.resolve(undefined)), [id])
  const vehicles = useRequest(() => (id ? api.listVehicleLocations() : Promise.resolve(undefined)), [id])
  const inside = (vehicles.data?.vehicles ?? []).filter((v) => v.current_geofences.some((g) => g.geofence_id === id))

  return (
    <Drawer open={!!geofence} onClose={onClose} size={560} title={geofence?.name} destroyOnHidden>
      {geofence && (
        <div className="flex flex-col gap-4">
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="ID">
              <Typography.Text copyable>{geofence.id}</Typography.Text>
            </Descriptions.Item>
            <Descriptions.Item label="Category">
              <CategoryTag category={geofence.category} />
            </Descriptions.Item>
            <Descriptions.Item label="Description">{geofence.description || <span className="text-slate-400">—</span>}</Descriptions.Item>
            <Descriptions.Item label="Status">
              <Tag color="green">{geofence.status ?? 'active'}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Created">{formatDateTime(geofence.created_at)}</Descriptions.Item>
            <Descriptions.Item label="Points">{geofence.coordinates.length} (closed ring)</Descriptions.Item>
          </Descriptions>

          <BaseMap className="h-[260px]" fitTo={geofence.coordinates} fitKey={geofence.id}>
            <GeofenceLayer geofences={[geofence]} highlightId={geofence.id} />
          </BaseMap>

          <div>
            <Typography.Title level={5}>Vehicles inside now</Typography.Title>
            {vehicles.loading ? (
              <Typography.Text type="secondary">Loading…</Typography.Text>
            ) : inside.length === 0 ? (
              <Typography.Text type="secondary">No vehicles are inside this geofence.</Typography.Text>
            ) : (
              <div className="flex flex-wrap gap-2">
                {inside.map((v) => (
                  <Tag key={v.vehicle_id}>{v.vehicle_number}</Tag>
                ))}
              </div>
            )}
          </div>

          <div>
            <div className="mb-2 flex items-center justify-between">
              <Typography.Title level={5} style={{ margin: 0 }}>
                Alert rules
              </Typography.Title>
              <Button size="small" onClick={() => navigate(`/alert-rules?geofence_id=${geofence.id}`)}>
                Configure alerts
              </Button>
            </div>
            {(rules.data?.alerts ?? []).length === 0 ? (
              <Typography.Text type="secondary">{rules.loading ? 'Loading…' : 'No alert rules. Events are recorded in history but nobody is notified.'}</Typography.Text>
            ) : (
              <div className="flex flex-col gap-1">
                {rules.data!.alerts.map((r) => (
                  <div key={r.alert_id} className="flex items-center gap-2 text-sm">
                    <EventTag type={r.event_type} />
                    {r.vehicle_number ?? <Tag>All vehicles</Tag>}
                    {r.status === 'inactive' && <Tag>inactive</Tag>}
                  </div>
                ))}
              </div>
            )}
          </div>

          <div>
            <Typography.Title level={5}>Coordinates [latitude, longitude]</Typography.Title>
            <pre className="max-h-48 overflow-auto rounded-lg bg-slate-50 p-3 text-xs">{formatCoordinates(geofence.coordinates)}</pre>
          </div>
        </div>
      )}
    </Drawer>
  )
}

// ---------------------------------------------------------------------------

interface FormValues {
  name: string
  description?: string
  category: GeofenceCategory
}

function CreateGeofenceDrawer({ open, existing, onClose, onCreated }: {
  open: boolean
  existing: Geofence[]
  onClose: () => void
  onCreated: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<FormValues>()
  const category = Form.useWatch('category', form)
  const [coordsText, setCoordsText] = useState('')
  const [source, setSource] = useState<'draw' | 'text'>('draw')
  const [resetKey, setResetKey] = useState(0)
  const [touchedCoords, setTouchedCoords] = useState(false)
  const [serverCoordError, setServerCoordError] = useState<string | null>(null)
  const [alertOnEntry, setAlertOnEntry] = useState(true)
  const [submitting, setSubmitting] = useState(false)

  const parsed = useMemo(() => parseCoordinates(coordsText), [coordsText])
  const coordError = serverCoordError ?? (touchedCoords ? parsed.error : undefined)
  const color = category ? CATEGORIES[category].color : '#2563eb'

  const reset = () => {
    form.resetFields()
    setCoordsText('')
    setTouchedCoords(false)
    setServerCoordError(null)
    setSource('draw')
    setResetKey((k) => k + 1)
  }

  const onDrawn = (coords: LatLngTuple[] | null) => {
    setSource('draw')
    setServerCoordError(null)
    setTouchedCoords(true)
    setCoordsText(coords ? formatCoordinates(coords) : '')
  }

  const onTyped = (text: string) => {
    if (source === 'draw') setResetKey((k) => k + 1) // remove the drawn shape; preview the typed one instead
    setSource('text')
    setServerCoordError(null)
    setTouchedCoords(true)
    setCoordsText(text)
  }

  const submit = async () => {
    setTouchedCoords(true)
    let values: FormValues
    try {
      values = await form.validateFields()
    } catch {
      return
    }
    if (!parsed.coords) return

    setSubmitting(true)
    try {
      const res = await api.createGeofence({ ...values, coordinates: parsed.coords })
      let extra = ''
      if (values.category === 'restricted_zone' && alertOnEntry) {
        try {
          await api.configureAlert({ geofence_id: res.id, event_type: 'entry' })
          extra = ' with an entry alert for all vehicles'
        } catch (e) {
          message.warning(`Geofence created, but the alert rule failed: ${errorMessage(e)}`)
        }
      }
      message.success(`Geofence "${res.name}" created${extra} (${formatNs(res.time_ns)})`)
      reset()
      onCreated()
    } catch (e) {
      if (e instanceof ApiError && Object.keys(e.fields).length > 0) {
        const { coordinates, ...rest } = e.fields
        if (coordinates) setServerCoordError(coordinates)
        form.setFields(Object.entries(rest).map(([name, err]) => ({ name: name as keyof FormValues, errors: [err] })))
      } else {
        message.error(errorMessage(e))
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Drawer
      open={open}
      onClose={onClose}
      title="New geofence"
      size={720}
      extra={
        <Button type="primary" loading={submitting} onClick={submit}>
          Create geofence
        </Button>
      }
    >
      <Form form={form} layout="vertical" requiredMark="optional" initialValues={{ category: 'delivery_zone' }}>
        <div className="grid grid-cols-1 gap-x-4 sm:grid-cols-2">
          <Form.Item name="name" label="Name" rules={[{ required: true, whitespace: true, message: 'Name is required' }, { max: 120 }]}>
            <Input placeholder="Downtown Delivery Zone" />
          </Form.Item>
          <Form.Item name="category" label="Category" rules={[{ required: true }]}>
            <Select options={CATEGORY_OPTIONS} />
          </Form.Item>
        </div>
        <Form.Item name="description" label="Description" rules={[{ max: 1000 }]}>
          <Input.TextArea rows={2} placeholder="What is this area used for?" />
        </Form.Item>
        {category === 'restricted_zone' && (
          <Form.Item>
            <Checkbox checked={alertOnEntry} onChange={(e) => setAlertOnEntry(e.target.checked)}>
              Also create an alert rule: notify when <b>any vehicle enters</b> this restricted zone
            </Checkbox>
          </Form.Item>
        )}
      </Form>

      <Typography.Text strong>Boundary</Typography.Text>
      <div className="mb-2 text-xs text-slate-500">
        Use the polygon tool (top-left of the map) and click to place corners, then click the first corner to finish. You can
        drag corners afterwards. Or type/paste coordinates below.
      </div>
      <BaseMap className="h-[340px]" fitTo={existing.flatMap((g) => g.coordinates)}>
        <GeofenceLayer geofences={existing} interactive={false} />
        {open && <DrawControl onChange={onDrawn} resetKey={resetKey} color={color} />}
        {source === 'text' && parsed.coords && <Polygon positions={parsed.coords} pathOptions={{ color, fillOpacity: 0.25 }} />}
      </BaseMap>

      <div className="mt-3">
        <div className="mb-1 flex items-center justify-between">
          <Typography.Text>Coordinates — [latitude, longitude] pairs, first = last</Typography.Text>
          <Button size="small" type="link" onClick={() => onTyped(formatCoordinates(README_EXAMPLE))}>
            Use example
          </Button>
        </div>
        <Input.TextArea
          rows={7}
          value={coordsText}
          onChange={(e) => onTyped(e.target.value)}
          status={coordError ? 'error' : undefined}
          placeholder={'[\n  [37.7749, -122.4194],\n  [37.7849, -122.4194],\n  [37.7849, -122.4094],\n  [37.7749, -122.4194]\n]'}
          style={{ fontFamily: 'ui-monospace, monospace', fontSize: 12 }}
        />
        {coordError ? (
          <div className="mt-1 text-xs text-red-600">{coordError}</div>
        ) : parsed.coords ? (
          <div className="mt-1 text-xs text-green-700">Valid polygon with {parsed.coords.length - 1} corners.</div>
        ) : null}
      </div>
    </Drawer>
  )
}
