import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { App, Button, Card, Empty, Form, Popconfirm, Radio, Select, Switch, Table, Tag, Typography } from 'antd'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { errorMessage } from '../api/client'
import { api } from '../api/endpoints'
import type { AlertRule, RuleEventType } from '../api/types'
import { CategoryTag, EventTag, LoadError, PageHeader } from '../components/common'
import { CATEGORIES, RULE_EVENT_OPTIONS } from '../constants'
import { useRequest } from '../hooks/useRequest'
import { formatDateTime, formatNs } from '../utils/format'

const ALL_VEHICLES = '__all__'

interface FormValues {
  geofence_id: string
  vehicle_id: string
  event_type: RuleEventType
}

export function AlertRulesPage() {
  const { message } = App.useApp()
  const [params] = useSearchParams()
  const [form] = Form.useForm<FormValues>()
  const [submitting, setSubmitting] = useState(false)
  const [filter, setFilter] = useState<{ geofence_id?: string; vehicle_id?: string }>({})
  const [busyId, setBusyId] = useState<string | null>(null)

  const geofences = useRequest(() => api.listGeofences(), [])
  const vehicles = useRequest(() => api.listVehicles(), [])
  // Filters are applied by the API: GET /alerts?geofence_id=&vehicle_id=
  const rules = useRequest(() => api.listAlerts(filter), [filter.geofence_id, filter.vehicle_id])

  const geofenceOptions = (geofences.data?.geofences ?? []).map((g) => ({
    value: g.id,
    label: g.name,
    category: g.category,
  }))
  const vehicleOptions = (vehicles.data?.vehicles ?? []).map((v) => ({ value: v.id, label: `${v.vehicle_number} · ${v.driver_name}` }))

  const submit = async (values: FormValues) => {
    setSubmitting(true)
    try {
      const res = await api.configureAlert({
        geofence_id: values.geofence_id,
        // "All vehicles" → omit vehicle_id, as the README specifies
        ...(values.vehicle_id && values.vehicle_id !== ALL_VEHICLES ? { vehicle_id: values.vehicle_id } : {}),
        event_type: values.event_type,
      })
      message.success(`Alert rule ${res.alert_id} created (${formatNs(res.time_ns)})`)
      void rules.reload()
    } catch (e) {
      message.error(errorMessage(e))
    } finally {
      setSubmitting(false)
    }
  }

  const toggle = async (rule: AlertRule, active: boolean) => {
    setBusyId(rule.alert_id)
    try {
      await api.setAlertStatus(rule.alert_id, active ? 'active' : 'inactive')
      await rules.reload(true)
    } catch (e) {
      message.error(errorMessage(e))
    } finally {
      setBusyId(null)
    }
  }

  const remove = async (rule: AlertRule) => {
    try {
      await api.deleteAlert(rule.alert_id)
      message.success('Alert rule deleted')
      await rules.reload(true)
    } catch (e) {
      message.error(errorMessage(e))
    }
  }

  return (
    <>
      <PageHeader title="Alert rules" subtitle="Choose which geofence events send real-time alerts" />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
        <Card title="New alert rule" className="xl:col-span-4 self-start">
          <Form
            form={form}
            layout="vertical"
            requiredMark="optional"
            onFinish={submit}
            initialValues={{ geofence_id: params.get('geofence_id') ?? undefined, vehicle_id: ALL_VEHICLES, event_type: 'entry' }}
          >
            <Form.Item name="geofence_id" label="Geofence" rules={[{ required: true, message: 'Select a geofence' }]}>
              <Select
                showSearch={{ optionFilterProp: 'label' }}
                placeholder={geofences.loading ? 'Loading…' : 'Select a geofence'}
                options={geofenceOptions}
                optionRender={(o) => (
                  <div className="flex items-center justify-between gap-2">
                    <span>{o.label}</span>
                    <CategoryTag category={o.data.category} />
                  </div>
                )}
                notFoundContent={<Empty description="No geofences yet" />}
              />
            </Form.Item>
            <Form.Item name="vehicle_id" label="Vehicle" extra="“All vehicles” applies the rule to every vehicle, including ones registered later.">
              <Select
                showSearch={{ optionFilterProp: 'label' }}
                options={[{ value: ALL_VEHICLES, label: 'All vehicles' }, ...vehicleOptions]}
              />
            </Form.Item>
            <Form.Item name="event_type" label="Trigger on" rules={[{ required: true }]}>
              <Radio.Group optionType="button" buttonStyle="solid" options={RULE_EVENT_OPTIONS} />
            </Form.Item>
            <Button type="primary" htmlType="submit" icon={<PlusOutlined />} loading={submitting} block>
              Create rule
            </Button>
          </Form>
        </Card>

        <Card className="xl:col-span-8" styles={{ body: { padding: 16 } }}>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <Typography.Text strong className="mr-2">
              Configured rules
            </Typography.Text>
            <Select
              allowClear
              placeholder="Filter by geofence"
              className="min-w-[200px]"
              options={geofenceOptions}
              value={filter.geofence_id}
              onChange={(v) => setFilter((f) => ({ ...f, geofence_id: v }))}
            />
            <Select
              allowClear
              placeholder="Filter by vehicle"
              className="min-w-[200px]"
              options={vehicleOptions}
              value={filter.vehicle_id}
              onChange={(v) => setFilter((f) => ({ ...f, vehicle_id: v }))}
            />
          </div>
          {rules.error ? (
            <LoadError error={rules.error} onRetry={() => rules.reload()} />
          ) : (
            <Table<AlertRule>
              rowKey="alert_id"
              loading={rules.loading}
              dataSource={rules.data?.alerts ?? []}
              scroll={{ x: 760 }}
              pagination={{ pageSize: 10, hideOnSinglePage: true }}
              locale={{ emptyText: <Empty description="No alert rules yet" /> }}
              columns={[
                {
                  title: 'Geofence',
                  dataIndex: 'geofence_name',
                  render: (name: string, r) => {
                    const cat = geofences.data?.geofences.find((g) => g.id === r.geofence_id)?.category
                    return (
                      <div>
                        <div className="font-medium">{name}</div>
                        {cat && <span className="text-xs" style={{ color: CATEGORIES[cat].color }}>{CATEGORIES[cat].label}</span>}
                      </div>
                    )
                  },
                },
                {
                  title: 'Vehicle',
                  dataIndex: 'vehicle_number',
                  render: (n: string | null) => (n ? n : <Tag color="purple">All vehicles</Tag>),
                },
                { title: 'Event', dataIndex: 'event_type', render: (t: RuleEventType) => <EventTag type={t} /> },
                {
                  title: 'Active',
                  dataIndex: 'status',
                  render: (s: string, r) => (
                    <Switch size="small" checked={s === 'active'} loading={busyId === r.alert_id} onChange={(on) => toggle(r, on)} />
                  ),
                },
                { title: 'Created', dataIndex: 'created_at', render: (d: string) => <span className="text-xs">{formatDateTime(d)}</span> },
                {
                  title: '',
                  width: 60,
                  render: (_, r) => (
                    <Popconfirm title="Delete this alert rule?" okText="Delete" okButtonProps={{ danger: true }} onConfirm={() => remove(r)}>
                      <Button size="small" type="text" danger icon={<DeleteOutlined />} aria-label="Delete rule" />
                    </Popconfirm>
                  ),
                },
              ]}
            />
          )}
        </Card>
      </div>
    </>
  )
}
