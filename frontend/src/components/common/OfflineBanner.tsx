import { useEffect, useState } from 'react'
import { WifiOff } from 'lucide-react'

import { queryClient } from '@/lib/queryClient'

export function OfflineBanner() {
  const [offline, setOffline] = useState(!navigator.onLine)

  useEffect(() => {
    const goOffline = () => setOffline(true)
    const goOnline = () => {
      setOffline(false)
      queryClient.invalidateQueries()
    }
    window.addEventListener('offline', goOffline)
    window.addEventListener('online', goOnline)
    return () => {
      window.removeEventListener('offline', goOffline)
      window.removeEventListener('online', goOnline)
    }
  }, [])

  if (!offline) return null

  return (
    <div className="fixed top-0 inset-x-0 z-50 flex items-center justify-center gap-2 bg-neutral-900 px-4 py-2 text-sm text-white">
      <WifiOff className="h-4 w-4" />
      <span>네트워크 연결이 끊겼습니다. 연결을 확인해 주세요.</span>
    </div>
  )
}
