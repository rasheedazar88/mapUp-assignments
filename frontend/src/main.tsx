import { StyleProvider } from '@ant-design/cssinjs'
import { App as AntApp, ConfigProvider } from 'antd'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import '@geoman-io/leaflet-geoman-free/dist/leaflet-geoman.css'
import 'leaflet/dist/leaflet.css'
import App from './App'
import './index.css'
import { store } from './store'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Provider store={store}>
    {/* layer: put Ant Design's styles in @layer antd, ordered BELOW Tailwind
        utilities in index.css, so classes like w-full can override them. */}
    <StyleProvider layer>
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: '#2563eb',
          borderRadius: 8,
          fontFamily: "'Inter', system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif",
          colorBgLayout: '#f4f6fb',
        },
        components: { Layout: { headerBg: '#fff', siderBg: '#fff' } },
      }}
    >
      {/* AntApp provides context-aware message/notification/modal via App.useApp() */}
      <AntApp>
        <App />
      </AntApp>
    </ConfigProvider>
    </StyleProvider>
    </Provider>
  </StrictMode>,
)
