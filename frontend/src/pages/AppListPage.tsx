import { Layers, Search } from 'lucide-react'
import { useMemo, useState } from 'react'

import AppCard from '@/components/works/AppCard'
import EmptyState from '@/components/common/EmptyState'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Input } from '@/components/ui/input'
import { useCollections, useCollectionCounts } from '@/hooks/useCollections'
import { TERM } from '@/lib/constants'

export default function AppListPage() {
  const { data: collections, isLoading, isError, error, refetch } = useCollections()
  const { data: counts } = useCollectionCounts()
  const [search, setSearch] = useState('')

  const filtered = useMemo(() => {
    if (!collections) return []
    if (!search.trim()) return collections
    const q = search.trim().toLowerCase()
    return collections.filter(
      (c) =>
        c.label.toLowerCase().includes(q) ||
        (c.description?.toLowerCase().includes(q)),
    )
  }, [collections, search])

  const hasCollections = collections && collections.length > 0

  return (
    <div>
      <PageHeader title={TERM.collections} description="앱을 선택해 데이터를 확인하세요" />

      {isLoading && <LoadingState variant="card-grid" />}
      {isError && <ErrorState error={error} onRetry={() => refetch()} />}

      {collections && collections.length === 0 && (
        <div className="mx-auto max-w-lg mt-8 animate-fade-in-up">
          <EmptyState
            title="앱이 아직 없습니다"
            description="관리자에게 앱 시드 실행을 요청하세요."
            icon={<Layers className="h-10 w-10" />}
          />
        </div>
      )}

      {hasCollections && (
        <>
          <div className="relative mb-4 max-w-xs">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="앱 검색…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-8"
            />
          </div>

          {filtered.length === 0 ? (
            <p className="py-8 text-center text-sm text-muted-foreground">
              '{search}'에 해당하는 앱이 없습니다
            </p>
          ) : (
            <div className="grid justify-center gap-4 grid-cols-[repeat(auto-fill,minmax(280px,340px))]">
              {filtered.map((c, i) => (
                <div key={c.id} className={`animate-scale-in stagger-${Math.min(i + 1, 12)}`}>
                  <AppCard collection={c} count={counts?.[c.slug]} />
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
