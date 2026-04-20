import { useNavigate, useParams } from 'react-router'
import { Monitor } from 'lucide-react'

import AppBuilder from '@/components/works/AppBuilder'
import { Button } from '@/components/ui/button'
import { useIsMobile } from '@/hooks/useIsMobile'

export default function AppBuilderPage() {
  const { appId } = useParams()
  const isMobile = useIsMobile()
  const navigate = useNavigate()

  if (isMobile) {
    return (
      <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 px-6 text-center">
        <Monitor className="h-12 w-12 text-muted-foreground" />
        <div>
          <h2 className="text-lg font-semibold">PC에서 이용해 주세요</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            앱 빌더는 넓은 화면에서 사용할 수 있습니다.
          </p>
        </div>
        <Button variant="outline" onClick={() => navigate('/apps')}>
          앱 목록으로 돌아가기
        </Button>
      </div>
    )
  }

  return (
    <div>
      <AppBuilder appId={appId} />
    </div>
  )
}
