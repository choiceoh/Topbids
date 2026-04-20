import { Link } from 'react-router'
import { Zap } from 'lucide-react'

import LoadingState from '@/components/common/LoadingState'
import ErrorState from '@/components/common/ErrorState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { useCollections } from '@/hooks/useCollections'

export default function GlobalAutomationsPage() {
  const { data: collections, isLoading, isError, error } = useCollections()

  if (isLoading) return <LoadingState variant="table" />
  if (isError) return <ErrorState error={error} />

  const apps = collections ?? []

  return (
    <div>
      <PageHeader
        title="자동화"
        description="전체 컬렉션의 자동화 규칙을 관리합니다"
      />

      {apps.length === 0 ? (
        <div className="py-16 text-center text-muted-foreground">
          <Zap className="mx-auto mb-3 h-10 w-10" />
          <p>아직 컬렉션이 없습니다</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {apps.map((app) => (
            <Link key={app.id} to={`/apps/${app.id}/automations`}>
              <Card className="flex items-center justify-between p-4 hover:bg-accent/50 transition-colors">
                <div className="flex items-center gap-3 min-w-0">
                  <Zap className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <div className="min-w-0">
                    <span className="block truncate font-medium">{app.label}</span>
                    <span className="text-xs text-muted-foreground">/{app.slug}</span>
                  </div>
                </div>
                <Badge variant="secondary" className="shrink-0 ml-2">자동화 관리</Badge>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
