import { useMemo } from 'react'
import { Link } from 'react-router'
import { ClipboardCheck } from 'lucide-react'

import EmptyState from '@/components/common/EmptyState'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { useMyTasks, type MyTaskItem } from '@/hooks/useEntries'

export default function MyTasksPage() {
  const { data: tasks, isLoading, isError, error } = useMyTasks()

  const grouped = useMemo(() => {
    if (!tasks) return []
    const map = new Map<string, { label: string, slug: string, icon?: string, items: MyTaskItem[] }>()
    for (const t of tasks) {
      let group = map.get(t.collectionId)
      if (!group) {
        group = { label: t.collectionLabel, slug: t.collectionSlug, icon: t.collectionIcon, items: [] }
        map.set(t.collectionId, group)
      }
      group.items.push(t)
    }
    return Array.from(map.values())
  }, [tasks])

  if (isLoading) return <LoadingState />
  if (isError) return <ErrorState error={error} />

  return (
    <div>
      <PageHeader
        title="내 업무"
        description="현재 내 차례에 있는 프로세스 항목입니다"
      />

      {grouped.length === 0 ? (
        <EmptyState
          title="처리할 업무가 없습니다"
          description="프로세스에서 내 차례에 해당하는 항목이 없습니다"
          icon={<ClipboardCheck className="h-10 w-10" />}
        />
      ) : (
        <div className="space-y-8">
          {grouped.map((group) => (
            <section key={group.slug}>
              <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-foreground">
                {group.icon && <span className="text-base">{group.icon}</span>}
                {group.label}
                <Badge variant="secondary" className="text-xs font-normal">
                  {group.items.length}
                </Badge>
              </h2>
              <div className="divide-y divide-border rounded-lg border border-border">
                {group.items.map((item) => (
                  <Link
                    key={item.id}
                    to={`/apps/${item.collectionSlug}?entry=${item.id}`}
                    className="flex items-center justify-between px-4 py-3 transition-colors hover:bg-accent/50"
                    viewTransition
                  >
                    <span className="truncate text-sm font-medium text-foreground">
                      {item.label}
                    </span>
                    <Badge variant="outline" className="ml-3 shrink-0 text-xs">
                      {item.status}
                    </Badge>
                  </Link>
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
