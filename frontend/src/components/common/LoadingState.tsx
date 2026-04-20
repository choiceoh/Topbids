import { Skeleton } from '@/components/ui/skeleton'

interface Props {
  label?: string
  variant?: 'spinner' | 'card-grid' | 'table' | 'summary' | 'kanban' | 'calendar' | 'gallery'
  count?: number
}

export default function LoadingState({
  label = '로딩 중...',
  variant = 'spinner',
  count,
}: Props) {
  if (variant === 'card-grid') return <CardGridSkeleton count={count ?? 6} />
  if (variant === 'table') return <TableSkeleton count={count ?? 5} />
  if (variant === 'summary') return <SummarySkeleton />
  if (variant === 'kanban') return <KanbanSkeleton count={count ?? 4} />
  if (variant === 'calendar') return <CalendarSkeleton />
  if (variant === 'gallery') return <GallerySkeleton count={count ?? 6} />

  return (
    <div className="flex items-center justify-center gap-3 py-10 text-sm text-muted-foreground sm:py-16 animate-fade-in">
      <div className="h-5 w-5 animate-spin rounded-full border-2 border-stone-200 border-t-stone-500" />
      <span>{label}</span>
    </div>
  )
}

function CardGridSkeleton({ count }: { count: number }) {
  return (
    <div className="grid justify-center gap-4 grid-cols-[repeat(auto-fill,minmax(280px,340px))]">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="rounded-lg border bg-card p-4 space-y-3">
          <div className="flex items-center gap-2">
            <Skeleton className="h-6 w-6 rounded" />
            <Skeleton className="h-4 w-28" />
          </div>
          <Skeleton className="h-3 w-full" />
          <Skeleton className="h-3 w-2/3" />
          <div className="flex items-center gap-2 pt-1">
            <Skeleton className="h-3 w-16" />
            <Skeleton className="h-3 w-16" />
            <Skeleton className="h-3 w-20" />
          </div>
        </div>
      ))}
    </div>
  )
}

function TableSkeleton({ count }: { count: number }) {
  return (
    <div className="rounded-lg border">
      {/* Header */}
      <div className="flex items-center gap-4 border-b px-4 py-3">
        <Skeleton className="h-4 w-4" />
        <Skeleton className="h-3 w-24" />
        <Skeleton className="h-3 w-32" />
        <Skeleton className="h-3 w-20" />
        <Skeleton className="h-3 w-28" />
        <Skeleton className="h-3 w-16" />
      </div>
      {/* Rows */}
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="flex items-center gap-4 border-b px-4 py-3 last:border-b-0">
          <Skeleton className="h-4 w-4" />
          <Skeleton className="h-3 w-28" />
          <Skeleton className="h-3 w-36" />
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-3 w-20" />
        </div>
      ))}
    </div>
  )
}

function SummarySkeleton() {
  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
      {Array.from({ length: 4 }, (_, i) => (
        <div key={i} className="rounded-lg border bg-card p-4 space-y-2">
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-7 w-24" />
        </div>
      ))}
    </div>
  )
}

function KanbanSkeleton({ count }: { count: number }) {
  return (
    <div className="flex gap-4 overflow-hidden">
      {Array.from({ length: count }, (_, ci) => (
        <div key={ci} className="min-w-[240px] flex-shrink-0 space-y-2">
          <div className="flex items-center gap-2 mb-2">
            <Skeleton className="h-5 w-16 rounded-full" />
            <Skeleton className="h-3 w-4" />
          </div>
          <div className="space-y-2 rounded-lg border-2 border-transparent p-1">
            {Array.from({ length: 3 - (ci % 2) }, (_, ri) => (
              <div key={ri} className="rounded-lg border bg-card p-3 space-y-2">
                <Skeleton className="h-3.5 w-3/4" />
                <Skeleton className="h-3 w-16" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function CalendarSkeleton() {
  const dayNames = ['일', '월', '화', '수', '목', '금', '토']
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Skeleton className="h-8 w-8 rounded-md" />
          <Skeleton className="h-5 w-28" />
          <Skeleton className="h-8 w-8 rounded-md" />
        </div>
        <Skeleton className="h-8 w-12 rounded-md" />
      </div>
      <div className="rounded-md border">
        <div className="grid grid-cols-7">
          {dayNames.map((name) => (
            <div
              key={name}
              className="border-b px-2 py-1.5 text-center text-xs font-medium text-muted-foreground"
            >
              {name}
            </div>
          ))}
        </div>
        {Array.from({ length: 5 }, (_, wi) => (
          <div key={wi} className="grid grid-cols-7">
            {Array.from({ length: 7 }, (_, di) => (
              <div key={di} className="min-h-[100px] border-b border-r p-1">
                <Skeleton className="h-3 w-4 mb-1" />
                {wi === 1 && di === 2 && <Skeleton className="h-4 w-full rounded" />}
                {wi === 2 && di === 4 && (
                  <div className="space-y-0.5">
                    <Skeleton className="h-4 w-full rounded" />
                    <Skeleton className="h-4 w-full rounded" />
                  </div>
                )}
                {wi === 3 && di === 0 && <Skeleton className="h-4 w-full rounded" />}
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

function GallerySkeleton({ count }: { count: number }) {
  return (
    <div className="grid gap-4 grid-cols-[repeat(auto-fill,minmax(220px,1fr))]">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="rounded-lg border bg-card overflow-hidden">
          <Skeleton className="h-40 w-full rounded-none" />
          <div className="p-3 space-y-2">
            <Skeleton className="h-3.5 w-3/4" />
            <Skeleton className="h-3 w-1/2" />
          </div>
        </div>
      ))}
    </div>
  )
}
