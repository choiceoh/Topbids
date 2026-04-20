import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Loader2 } from 'lucide-react'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import ProcessFlowDiagram from '@/components/works/ProcessFlowDiagram'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useCollection } from '@/hooks/useCollections'
import { useProcess, useSaveProcess } from '@/hooks/useProcess'
import { useUnsavedChanges } from '@/hooks/useUnsavedChanges'
import { useUsers } from '@/hooks/useUsers'
import { formatError } from '@/lib/api'

const STATUS_COLORS = [
  { label: '회색', value: '#6b7280' },
  { label: '파랑', value: '#3b82f6' },
  { label: '초록', value: '#22c55e' },
  { label: '노랑', value: '#eab308' },
  { label: '주황', value: '#f97316' },
  { label: '빨강', value: '#ef4444' },
  { label: '보라', value: '#a855f7' },
  { label: '청록', value: '#06b6d4' },
]

interface StatusDraft {
  name: string
  color: string
  sort_order: number
  is_initial: boolean
}

interface TransitionDraft {
  from_index: number
  to_index: number
  label: string
  allowed_roles: string[]
  allowed_user_ids: string[]
}

export default function ProcessPage() {
  const { appId } = useParams()
  const navigate = useNavigate()
  const { data: collection, isLoading: colLoading } = useCollection(appId)
  const { data: process, isLoading: procLoading, isError, error, refetch } = useProcess(appId)
  const saveProcess = useSaveProcess(appId ?? '')
  const { data: allUsers } = useUsers()

  const [isEnabled, setIsEnabled] = useState(false)
  const [statuses, setStatuses] = useState<StatusDraft[]>([])
  const [transitions, setTransitions] = useState<TransitionDraft[]>([])
  const [newStatusName, setNewStatusName] = useState('')

  // Sync server state to local state.
  const [syncedProcessId, setSyncedProcessId] = useState<string | null>(null)
  if (process && process.id !== syncedProcessId) {
    setSyncedProcessId(process.id)
    setIsEnabled(process.is_enabled)
    if (process.statuses?.length) {
      setStatuses(
        process.statuses.map((s) => ({
          name: s.name,
          color: s.color,
          sort_order: s.sort_order,
          is_initial: s.is_initial,
        })),
      )
      if (process.transitions?.length) {
        const idToIndex = new Map(process.statuses.map((s, i) => [s.id, i]))
        setTransitions(
          process.transitions
            .filter((t) => idToIndex.has(t.from_status_id) && idToIndex.has(t.to_status_id))
            .map((t) => ({
              from_index: idToIndex.get(t.from_status_id)!,
              to_index: idToIndex.get(t.to_status_id)!,
              label: t.label,
              allowed_roles: t.allowed_roles ?? [],
              allowed_user_ids: t.allowed_user_ids ?? [],
            })),
        )
      } else {
        setTransitions([])
      }
    } else {
      setStatuses([])
      setTransitions([])
    }
  }

  const isDirty = useMemo(() => {
    if (!process) return false
    if (isEnabled !== process.is_enabled) return true
    const serverStatuses = (process.statuses ?? []).map((s) => ({
      name: s.name,
      color: s.color,
      sort_order: s.sort_order,
      is_initial: s.is_initial,
    }))
    if (JSON.stringify(statuses) !== JSON.stringify(serverStatuses)) return true
    return transitions.length !== (process.transitions ?? []).length
  }, [process, isEnabled, statuses, transitions])

  const blocker = useUnsavedChanges(isDirty)

  if (colLoading || procLoading) return <LoadingState />
  if (isError) return <ErrorState error={error} onRetry={() => refetch()} />
  if (!collection) return null

  function addStatus() {
    const name = newStatusName.trim()
    if (!name) return
    const isFirst = statuses.length === 0
    setStatuses([
      ...statuses,
      {
        name,
        color: STATUS_COLORS[statuses.length % STATUS_COLORS.length].value,
        sort_order: statuses.length,
        is_initial: isFirst,
      },
    ])
    setNewStatusName('')
  }

  function removeStatus(idx: number) {
    const removed = statuses[idx]
    const next = statuses.filter((_, i) => i !== idx)
    // If removed was initial, set first remaining as initial.
    if (removed.is_initial && next.length > 0) {
      next[0] = { ...next[0], is_initial: true }
    }
    // Fix sort_order.
    const reordered = next.map((s, i) => ({ ...s, sort_order: i }))
    setStatuses(reordered)
    // Remove transitions referencing the removed index and adjust indices.
    setTransitions(
      transitions
        .filter((t) => t.from_index !== idx && t.to_index !== idx)
        .map((t) => ({
          ...t,
          from_index: t.from_index > idx ? t.from_index - 1 : t.from_index,
          to_index: t.to_index > idx ? t.to_index - 1 : t.to_index,
        })),
    )
  }

  function setInitial(idx: number) {
    setStatuses(statuses.map((s, i) => ({ ...s, is_initial: i === idx })))
  }

  function removeTransition(idx: number) {
    setTransitions(transitions.filter((_, i) => i !== idx))
  }

  function updateTransition(idx: number, patch: Partial<TransitionDraft>) {
    setTransitions(transitions.map((t, i) => (i === idx ? { ...t, ...patch } : t)))
  }

  function handleSave() {
    saveProcess.mutate(
      { is_enabled: isEnabled, statuses, transitions },
      {
        onSuccess: () => toast.success('프로세스가 저장되었습니다'),
        onError: (err) => toast.error(formatError(err)),
      },
    )
  }

  return (
    <div>
      <PageHeader
        breadcrumb={[
          { label: '앱 목록', href: '/apps' },
          { label: collection.label, href: `/apps/${collection.id}` },
          { label: '설정', href: `/apps/${collection.id}/settings` },
          { label: '프로세스' },
        ]}
        title="프로세스 관리"
      />

      {/* Action buttons */}
      <div className="mb-6 flex items-center gap-2">
        <Button onClick={handleSave} disabled={saveProcess.isPending}>
          {saveProcess.isPending ? <><Loader2 className="mr-1.5 h-4 w-4 animate-spin" />저장 중...</> : '저장'}
        </Button>
        <Button variant="outline" onClick={() => refetch()}>
          취소
        </Button>
        <Link to={`/apps/${collection.id}/settings`}>
          <Button variant="outline">관리 홈으로 이동</Button>
        </Link>
        <Button variant="outline" onClick={() => navigate(`/apps/${collection.id}`)}>
          해당 앱으로 이동
        </Button>
      </div>

      <div className="space-y-6">
        {/* Toggle */}
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={!isEnabled}
            onCheckedChange={(c) => setIsEnabled(!c)}
          />
          이 앱에서는 상태를 사용하지 않겠습니다.
        </label>

        {isEnabled && (
          <>
            {/* Add status */}
            <section>
              <div className="flex items-end gap-2">
                <div className="flex-1">
                  <Input
                    placeholder="상태 이름 입력"
                    value={newStatusName}
                    onChange={(e) => setNewStatusName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && addStatus()}
                  />
                </div>
                <Button onClick={addStatus}>+</Button>
              </div>
            </section>

            {/* Status list */}
            {statuses.length > 0 && (
              <section>
                <h2 className="mb-2 text-sm font-semibold">상태 ({statuses.length})</h2>
                <div className="space-y-2">
                  {statuses.map((s, idx) => (
                    <Card key={idx} className="flex items-center gap-2 p-3">
                      <div
                        className="h-4 w-4 rounded-full flex-shrink-0"
                        style={{ backgroundColor: s.color }}
                      />
                      <span className="flex-1 font-medium">{s.name}</span>
                      <Select
                        value={s.color}
                        onValueChange={(v) => {
                          if (v) setStatuses(statuses.map((st, i) => (i === idx ? { ...st, color: v } : st)))
                        }}
                      >
                        <SelectTrigger className="w-24">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {STATUS_COLORS.map((c) => (
                            <SelectItem key={c.value} value={c.value}>
                              <div className="flex items-center gap-2">
                                <div
                                  className="h-3 w-3 rounded-full"
                                  style={{ backgroundColor: c.value }}
                                />
                                {c.label}
                              </div>
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <label className="flex items-center gap-1 text-xs">
                        <input
                          type="radio"
                          name="initial"
                          checked={s.is_initial}
                          onChange={() => setInitial(idx)}
                        />
                        초기
                      </label>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => removeStatus(idx)}
                      >
                        삭제
                      </Button>
                    </Card>
                  ))}
                </div>
              </section>
            )}

            {/* Interactive flow diagram + transition editor */}
            {statuses.length > 0 && (
              <section>
                <h2 className="mb-3 text-sm font-semibold">
                  플로우 다이어그램 ({transitions.length}개 전이)
                </h2>
                <ProcessFlowDiagram
                  statuses={statuses}
                  transitions={transitions}
                  users={allUsers?.map((u) => ({ id: u.id, name: u.name })) ?? []}
                  onAddTransition={(from, to) => {
                    const fromName = statuses[from]?.name ?? ''
                    const toName = statuses[to]?.name ?? ''
                    setTransitions([
                      ...transitions,
                      { from_index: from, to_index: to, label: `${fromName} → ${toName}`, allowed_roles: [], allowed_user_ids: [] },
                    ])
                  }}
                  onRemoveTransition={removeTransition}
                  onUpdateTransition={updateTransition}
                />
                <p className="mt-2 text-xs text-muted-foreground">
                  노드를 드래그하여 위치를 조정하세요. &quot;전이 추가&quot; 버튼을 누른 뒤 출발 → 도착 노드를 클릭하면 전이가 생성됩니다. 화살표를 클릭하면 편집할 수 있습니다.
                </p>
              </section>
            )}
          </>
        )}

      </div>

      <ConfirmDialog
        open={blocker.state === 'blocked'}
        onOpenChange={(open) => { if (!open) blocker.reset?.() }}
        title="저장하지 않고 나가시겠습니까?"
        description="작성 중인 내용이 저장되지 않습니다."
        confirmLabel="나가기"
        cancelLabel="계속 작성"
        onConfirm={() => blocker.proceed?.()}
      />
    </div>
  )
}

