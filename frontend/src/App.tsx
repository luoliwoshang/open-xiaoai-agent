import { useHashPage } from './hooks/useHashPage'
import { useDashboardState } from './hooks/useDashboardState'
import { Sidebar } from './components/Sidebar'
import { DashboardPage } from './pages/DashboardPage'
import { LogsPage } from './pages/LogsPage'
import { SettingsPage } from './pages/SettingsPage'

export default function App() {
  const page = useHashPage()
  const { state, error, reload } = useDashboardState()

  return (
    <div className="app-layout">
      <Sidebar page={page} assistant={state.assistant} xiaoai={state.xiaoai} />
      <main className="main-content">
        {error && (
          <div style={{
            padding: '12px 32px',
            background: 'var(--red-dim)',
            color: 'var(--red)',
            fontFamily: 'var(--font-mono)',
            fontSize: 12,
            borderBottom: '1px solid var(--red)',
          }}>
            连接错误: {error}
          </div>
        )}
        {page === 'dashboard' && <DashboardPage state={state} onReload={reload} />}
        {page === 'logs' && <LogsPage />}
        {page === 'settings' && <SettingsPage state={state} onReload={reload} />}
      </main>
    </div>
  )
}
