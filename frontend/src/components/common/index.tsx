import { ReloadOutlined } from '@ant-design/icons'
import { Button, Card, Result, Tag, Typography } from 'antd'
import type { ReactNode } from 'react'
import type { EventType, GeofenceCategory, RuleEventType } from '../../api/types'
import { CATEGORIES } from '../../constants'

export function PageHeader({ title, subtitle, extra }: { title: string; subtitle?: string; extra?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <Typography.Title level={3} style={{ margin: 0 }}>
          {title}
        </Typography.Title>
        {subtitle && <Typography.Text type="secondary">{subtitle}</Typography.Text>}
      </div>
      {extra && <div className="flex flex-wrap gap-2">{extra}</div>}
    </div>
  )
}

export function CategoryTag({ category }: { category: GeofenceCategory | string }) {
  const c = CATEGORIES[category as GeofenceCategory]
  return <Tag color={c?.tag ?? 'default'}>{c?.label ?? category}</Tag>
}

export function EventTag({ type }: { type: EventType | RuleEventType }) {
  const map: Record<string, { color: string; label: string }> = {
    entry: { color: 'volcano', label: 'Entry' },
    exit: { color: 'geekblue', label: 'Exit' },
    both: { color: 'purple', label: 'Entry & exit' },
  }
  const m = map[type] ?? { color: 'default', label: type }
  return <Tag color={m.color}>{m.label}</Tag>
}

export function StatCard({ title, value, icon, hint, color = '#2563eb', loading }: {
  title: string
  value: ReactNode
  icon: ReactNode
  hint?: ReactNode
  color?: string
  loading?: boolean
}) {
  return (
    <Card loading={loading} styles={{ body: { padding: 20 } }}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <Typography.Text type="secondary">{title}</Typography.Text>
          <div className="mt-1 text-3xl font-semibold" style={{ color: '#0f172a' }}>
            {value}
          </div>
          {hint && <div className="mt-1 text-xs text-slate-500">{hint}</div>}
        </div>
        <div
          className="flex h-11 w-11 items-center justify-center rounded-xl text-xl"
          style={{ background: `${color}18`, color }}
        >
          {icon}
        </div>
      </div>
    </Card>
  )
}

/** Error state with a retry button, used when a page's data fails to load. */
export function LoadError({ error, onRetry }: { error: string; onRetry: () => void }) {
  return (
    <Result
      status="warning"
      title="Couldn't load data"
      subTitle={error}
      extra={
        <Button icon={<ReloadOutlined />} onClick={onRetry}>
          Try again
        </Button>
      }
    />
  )
}
