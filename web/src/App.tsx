import { AppTopbar } from './components/AppTopbar'
import { useDashboardState } from './hooks/useDashboardState'
import { useHashPage } from './hooks/useHashPage'
import { DashboardPage } from './pages/DashboardPage'
import { SettingsPage } from './pages/SettingsPage'

export default function App() {
  const page = useHashPage()
  const { data, setData, loading, error, refresh } = useDashboardState()

  return (
    <div className="app-shell">
      <div className="aurora aurora-left" />
      <div className="aurora aurora-right" />

      <AppTopbar page={page} />

      {page === 'dashboard' ? (
        <DashboardPage
          data={data}
          error={error}
          loading={loading}
          setData={setData}
        />
      ) : (
        <SettingsPage
          data={data}
          error={error}
          refresh={refresh}
          setData={setData}
        />
      )}
    </div>
  )
}

