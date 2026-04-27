import { useEffect, useState } from 'react'
import { currentPageFromHash, type Page } from '../lib/dashboard'

export function useHashPage() {
  const [page, setPage] = useState<Page>(() => currentPageFromHash())

  useEffect(() => {
    const handleHashChange = () => {
      setPage(currentPageFromHash())
    }

    if (!window.location.hash) {
      window.location.hash = '#/'
    }
    handleHashChange()
    window.addEventListener('hashchange', handleHashChange)
    return () => {
      window.removeEventListener('hashchange', handleHashChange)
    }
  }, [])

  return page
}

