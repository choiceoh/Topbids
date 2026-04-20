import { ChevronDown, ChevronUp, GitBranch, Layers, MousePointerClick, Search, Settings, Sparkles } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link } from 'react-router'

import AppCard from '@/components/works/AppCard'
import EmptyState from '@/components/common/EmptyState'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import TemplateGallery from '@/components/works/TemplateGallery'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import { useCollections, useCollectionCounts } from '@/hooks/useCollections'
import { TERM } from '@/lib/constants'

export default function AppListPage() {
  const { data: collections, isLoading, isError, error, refetch } = useCollections()
  const { data: counts } = useCollectionCounts()
  const aiAvailable = useAIAvailable()
  const [showTemplates, setShowTemplates] = useState(false)
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
      <PageHeader
        title={TERM.collections}
        description="앱을 만들고 데이터를 관리하세요"
        actions={
          <>
            <Link to="/apps/relationships">
              <Button variant="outline" className="gap-1">
                <GitBranch className="h-4 w-4" />
                관계도
              </Button>
            </Link>
            <Button
              variant={showTemplates ? 'secondary' : 'outline'}
              onClick={() => setShowTemplates(!showTemplates)}
              className="gap-1"
            >
              템플릿
              {showTemplates
                ? <ChevronUp className="h-3.5 w-3.5" />
                : <ChevronDown className="h-3.5 w-3.5" />}
            </Button>
            <Link to="/apps/new">
              <Button>{TERM.newCollection}</Button>
            </Link>
          </>
        }
      />

      {/* Template gallery */}
      {showTemplates && (
        <div className="mb-6 animate-slide-down">
          <h3 className="mb-3 text-sm font-medium text-muted-foreground">
            템플릿으로 빠르게 시작하세요
          </h3>
          <TemplateGallery />
        </div>
      )}

      {isLoading && <LoadingState variant="card-grid" />}
      {isError && <ErrorState error={error} onRetry={() => refetch()} />}

      {collections && collections.length === 0 && !showTemplates && (
        <div className="mx-auto max-w-lg mt-8 animate-fade-in-up">
          <EmptyState
            title={TERM.noCollections}
            description={TERM.noCollectionsDesc}
            icon={<Layers className="h-10 w-10" />}
            action={
              <Link to="/apps/new">
                <Button>{TERM.newCollection}</Button>
              </Link>
            }
          />
          {/* AI quick start */}
          {aiAvailable && (
            <div className="mt-6 flex items-center gap-4 rounded-lg border p-4">
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted">
                <Sparkles className="h-5 w-5 text-muted-foreground" />
              </div>
              <div className="flex-1 min-w-0">
                <h4 className="text-sm font-medium">AI로 첫 앱 만들기</h4>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  어떤 데이터를 관리하고 싶은지 설명하면 AI가 앱 구조를 제안합니다
                </p>
              </div>
              <Link to="/apps/new">
                <Button variant="outline" size="sm" className="gap-1.5 shrink-0">
                  <Sparkles className="h-3.5 w-3.5" />
                  시작하기
                </Button>
              </Link>
            </div>
          )}
          {/* Onboarding guide */}
          <div className="mt-8 space-y-4">
            <h3 className="text-center text-sm font-medium text-muted-foreground">
              시작하기
            </h3>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <OnboardingStep
                icon={<Layers className="h-5 w-5" />}
                step="1"
                title="앱 정의"
                description="관리할 앱을 만들고 필요한 항목을 설정하세요."
              />
              <OnboardingStep
                icon={<MousePointerClick className="h-5 w-5" />}
                step="2"
                title="데이터 입력"
                description="폼으로 데이터를 등록하거나 CSV로 일괄 가져오세요."
              />
              <OnboardingStep
                icon={<Settings className="h-5 w-5" />}
                step="3"
                title="보기 구성"
                description="목록, 보드, 캘린더, 갤러리 등 원하는 형태로 데이터를 확인하세요."
              />
            </div>
          </div>
        </div>
      )}

      {hasCollections && (
        <>
          {/* Search bar */}
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

function OnboardingStep({
  icon,
  step,
  title,
  description,
}: {
  icon: React.ReactNode
  step: string
  title: string
  description: string
}) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-2xl border border-border/60 bg-white p-6 text-center shadow-premium transition-all duration-300 hover:-translate-y-0.5 hover:shadow-premium-hover">
      <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-accent text-muted-foreground">
        {icon}
      </div>
      <div className="text-[10px] font-semibold uppercase tracking-[0.15em] text-muted-foreground/60">STEP {step}</div>
      <h4 className="text-sm font-medium text-foreground">{title}</h4>
      <p className="text-xs leading-relaxed text-muted-foreground">{description}</p>
    </div>
  )
}
