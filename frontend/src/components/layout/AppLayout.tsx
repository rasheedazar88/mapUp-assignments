import {
  AimOutlined,
  AlertOutlined,
  BellOutlined,
  CarOutlined,
  DashboardOutlined,
  GatewayOutlined,
  HistoryOutlined,
  MenuOutlined,
  SettingOutlined,
} from '@ant-design/icons'
import { Badge, Button, Drawer, Grid, Layout, Menu, Spin, Tooltip, Typography } from 'antd'
import { Suspense, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { useAlertStore, type SocketStatus } from '../../store/alertStore'

const NAV = [
  { key: '/', icon: <DashboardOutlined />, label: 'Dashboard' },
  { key: '/geofences', icon: <GatewayOutlined />, label: 'Geofences' },
  { key: '/vehicles', icon: <CarOutlined />, label: 'Vehicles' },
  { key: '/tracking', icon: <AimOutlined />, label: 'Live Tracking' },
  { key: '/alert-rules', icon: <SettingOutlined />, label: 'Alert Rules' },
  { key: '/alerts', icon: <AlertOutlined />, label: 'Live Alerts' },
  { key: '/history', icon: <HistoryOutlined />, label: 'Violation History' },
]

const STATUS: Record<SocketStatus, { color: string; label: string }> = {
  open: { color: '#16a34a', label: 'Live' },
  connecting: { color: '#d97706', label: 'Connecting…' },
  reconnecting: { color: '#d97706', label: 'Reconnecting…' },
  closed: { color: '#94a3b8', label: 'Offline' },
}

function Brand() {
  return (
    <div className="flex h-16 items-center gap-2 px-5">
      <div className="flex h-8 w-8 items-center justify-center rounded-lg text-white" style={{ background: '#2563eb' }}>
        <GatewayOutlined />
      </div>
      <div className="leading-tight">
        <div className="font-semibold text-slate-900">GeoFleet</div>
        <div className="text-[11px] text-slate-500">Geofencing &amp; alerts</div>
      </div>
    </div>
  )
}

export function AppLayout() {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const screens = Grid.useBreakpoint()
  const isMobile = !screens.lg
  const [drawerOpen, setDrawerOpen] = useState(false)
  const status = useAlertStore((s) => s.status)
  const unread = useAlertStore((s) => s.unread)

  const selected = NAV.find((n) => n.key !== '/' && pathname.startsWith(n.key))?.key ?? '/'
  const title = NAV.find((n) => n.key === selected)?.label

  const menu = (
    <Menu
      mode="inline"
      selectedKeys={[selected]}
      items={NAV.map((n) => ({
        ...n,
        label: n.key === '/alerts' && unread > 0 ? (
          <span className="flex items-center justify-between">
            {n.label} <Badge count={unread} size="small" />
          </span>
        ) : (
          n.label
        ),
      }))}
      onClick={({ key }) => {
        navigate(key)
        setDrawerOpen(false)
      }}
      style={{ borderInlineEnd: 'none' }}
    />
  )

  const s = STATUS[status]

  return (
    <Layout style={{ minHeight: '100vh' }}>
      {!isMobile && (
        <Layout.Sider width={232} theme="light" style={{ borderRight: '1px solid #e5e7eb', position: 'sticky', top: 0, height: '100vh' }}>
          <Brand />
          {menu}
        </Layout.Sider>
      )}
      {isMobile && (
        <Drawer placement="left" open={drawerOpen} onClose={() => setDrawerOpen(false)} size={260} styles={{ body: { padding: 0 } }} closable={false}>
          <Brand />
          {menu}
        </Drawer>
      )}

      <Layout>
        <Layout.Header
          style={{ background: '#fff', borderBottom: '1px solid #e5e7eb', padding: '0 16px', position: 'sticky', top: 0, zIndex: 10, height: 64 }}
          className="flex items-center justify-between gap-3"
        >
          <div className="flex min-w-0 items-center gap-2">
            {isMobile && <Button type="text" icon={<MenuOutlined />} onClick={() => setDrawerOpen(true)} aria-label="Open menu" />}
            <Typography.Text strong className="truncate text-base">
              {title}
            </Typography.Text>
          </div>
          <div className="flex items-center gap-3">
            <Tooltip title="Real-time alert connection (WebSocket /ws/alerts)">
              <span className="flex items-center gap-2 rounded-full border border-slate-200 px-3 py-1 text-xs text-slate-600">
                <span className="inline-block h-2 w-2 rounded-full" style={{ background: s.color }} />
                {s.label}
              </span>
            </Tooltip>
            <Badge count={unread} size="small" overflowCount={99}>
              <Button shape="circle" icon={<BellOutlined />} onClick={() => navigate('/alerts')} aria-label="Live alerts" />
            </Badge>
          </div>
        </Layout.Header>

        <Layout.Content style={{ padding: isMobile ? 16 : 24 }}>
          <div className="mx-auto max-w-[1440px]">
            <Suspense fallback={<div className="flex justify-center py-24"><Spin size="large" /></div>}>
              <Outlet />
            </Suspense>
          </div>
        </Layout.Content>
      </Layout>
    </Layout>
  )
}
