import { Link } from 'react-router'
import { BarChart3 } from 'lucide-react'

import EmptyState from '@/components/common/EmptyState'
import LoadingState from '@/components/common/LoadingState'
import ErrorState from '@/components/common/ErrorState'
import PageHeader from '@/components/common/PageHeader'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useCollections } from '@/hooks/useCollections'

export default function GlobalDashboardPage() {
  const { data: collections, isLoading, isError, error } = useCollections()

  if (isLoading) return <LoadingState variant="summary" />
  if (isError) return <ErrorState error={error} />

  const apps = collections ?? []

  return (
    <div>
      <PageHeader
        title="대시보드"
        description="전체 컬렉션 현황을 한눈에 확인합니다"
      />

      {apps.length === 0 ? (
        <EmptyState
          title="아직 컬렉션이 없습니다"
          description="컬렉션을 만들면 여기에 요약이 표시됩니다"
          icon={<BarChart3 className="h-10 w-10" />}
          action={
            <Link to="/apps/new">
              <Button>새 앱 만들기</Button>
            </Link>
          }
        />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {apps.map((app) => (
            <Link key={app.id} to={`/apps/${app.id}?tab=chart`}>
              <Card className="p-4 hover:bg-accent/50 transition-colors">
                <div className="mb-1 font-medium">{app.label}</div>
                <div className="text-xs text-muted-foreground">
                  /{app.slug} &middot; 필드 {app.fields?.length ?? 0}개
                </div>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
