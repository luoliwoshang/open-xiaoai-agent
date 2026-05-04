import { useEffect, useState } from 'react'
import type { Page } from '../types'
import { currentPageFromHash } from '../lib/dashboard'

export function useHashPage(): Page {
  const [page, setPage] = useState<Page>(() => {
    const p = currentPageFromHash()
    return (p === 'settings' || p === 'logs' || p === 'memory' ? p : 'dashboard') as Page
  })

  useEffect(() => {
    const handler = () => {
      const p = currentPageFromHash()
      setPage((p === 'settings' || p === 'logs' || p === 'memory' ? p : 'dashboard') as Page)
    }
    window.addEventListener('hashchange', handler)
    return () => window.removeEventListener('hashchange', handler)
  }, [])

  return page
}
