import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Monitor } from 'lucide-react'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { canManageCollection, useCurrentUser } from '@/hooks/useAuth'
import IconPicker from '@/components/works/IconPicker'
import { useIsMobile } from '@/hooks/useIsMobile'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useCollection,
  useDeleteCollection,
  useUpdateCollection,
} from '@/hooks/useCollections'
import { useMembers, useAddMember, useRemoveMember } from '@/hooks/useMembers'
import { formatError } from '@/lib/api'
import { isComputedType, isLayoutType } from '@/lib/constants'
import type { RLSFilter } from '@/lib/types'

export default function AppSettingsPage() {
  const { appId } = useParams()
  const navigate = useNavigate()
  const { data: collection, isLoading, isError, error, refetch } = useCollection(appId)

  const deleteCollection = useDeleteCollection()
  const updateCollection = useUpdateCollection(appId ?? '')
  const { data: members } = useMembers(appId)
  const addMember = useAddMember(appId ?? '')
  const removeMember = useRemoveMember(appId ?? '')
  const { data: currentUser } = useCurrentUser()
  const canManage = canManageCollection(currentUser, collection?.created_by)

  const [newMemberUserId, setNewMemberUserId] = useState('')
  const [newMemberRole, setNewMemberRole] = useState('viewer')
  const [confirmDeleteCollection, setConfirmDeleteCollection] = useState(false)
  const isMobile = useIsMobile()

  if (isMobile) {
    return (
      <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 px-6 text-center">
        <Monitor className="h-12 w-12 text-muted-foreground" />
        <div>
          <h2 className="text-lg font-semibold">PC에서 이용해 주세요</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            앱 설정은 넓은 화면에서 사용할 수 있습니다.
          </p>
        </div>
        <Button variant="outline" onClick={() => navigate('/apps')}>
          앱 목록으로 돌아가기
        </Button>
      </div>
    )
  }

  if (isLoading) return <LoadingState />
  if (isError) return <ErrorState error={error} onRetry={() => refetch()} />
  if (!collection) return null

  function handleDeleteCollection() {
    deleteCollection.mutate(
      { id: collection!.id, confirm: true },
      {
        onSuccess: () => {
          toast.success('앱이 삭제되었습니다')
          navigate('/apps')
        },
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
          { label: '설정' },
        ]}
        title="설정"
        description={`/${collection.slug} 앱의 항목 및 설정`}
      />

      <div className="space-y-6">
        {canManage && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-4">
            <Card className="p-4">
              <h2 className="mb-1 text-sm font-semibold">앱 수정</h2>
              <p className="text-sm text-muted-foreground">
                앱의 항목(필드) 구조를 추가하거나 변경합니다.
              </p>
              <Link to={`/apps/${collection.id}/edit`}>
                <Button variant="outline" size="sm" className="mt-3">
                  앱 수정
                </Button>
              </Link>
            </Card>

            <Card className="p-4">
              <h2 className="mb-3 text-sm font-semibold">기본 정보</h2>
              <div className="flex items-center gap-3">
                <div>
                  <Label>아이콘</Label>
                  <IconPicker
                    value={collection.icon}
                    onChange={(icon) =>
                      updateCollection.mutate(
                        { icon },
                        { onError: (err) => toast.error(formatError(err)) },
                      )
                    }
                  />
                </div>
              </div>
            </Card>

            <Card className="p-4">
              <h2 className="mb-1 text-sm font-semibold">프로세스</h2>
              <p className="text-sm text-muted-foreground">
                항목의 상태 흐름(워크플로우)을 정의합니다.
              </p>
              <Link to={`/apps/${collection.id}/process`}>
                <Button variant="outline" size="sm" className="mt-3">
                  프로세스 설정
                </Button>
              </Link>
            </Card>

            <Card className="p-4">
              <h2 className="mb-1 text-sm font-semibold">자동화</h2>
              <p className="text-sm text-muted-foreground">
                데이터 변경 시 자동 알림, 항목 업데이트, 외부 연결을 실행합니다.
              </p>
              <Link to={`/apps/${collection.id}/automations`}>
                <Button variant="outline" size="sm" className="mt-3">
                  자동화 설정
                </Button>
              </Link>
            </Card>
          </div>
        )}

        {canManage && (
          <section>
            <h2 className="mb-3 text-lg font-semibold">멤버 ({members?.length ?? 0})</h2>
            <div className="space-y-2">
              {members?.map((m) => (
                <Card key={m.id} className="flex items-center justify-between p-3">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{m.user_name || m.user_email}</span>
                    <Badge variant="secondary">{m.role}</Badge>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      removeMember.mutate(m.user_id, {
                        onSuccess: () => toast.success('멤버가 제거되었습니다'),
                        onError: (err) => toast.error(formatError(err)),
                      })
                    }}
                  >
                    제거
                  </Button>
                </Card>
              ))}
            </div>
            <Card className="mt-3 p-4">
              <div className="flex items-end gap-2">
                <div className="flex-1">
                  <Label>사용자 ID</Label>
                  <Input
                    value={newMemberUserId}
                    onChange={(e) => setNewMemberUserId(e.target.value)}
                    placeholder="UUID"
                  />
                </div>
                <div>
                  <Label>역할</Label>
                  <Select value={newMemberRole} onValueChange={(v) => v && setNewMemberRole(v)}>
                    <SelectTrigger className="w-28">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="owner">소유자</SelectItem>
                      <SelectItem value="editor">편집자</SelectItem>
                      <SelectItem value="viewer">열람자</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <Button
                  size="sm"
                  disabled={!newMemberUserId.trim() || addMember.isPending}
                  onClick={() => {
                    addMember.mutate(
                      { user_id: newMemberUserId.trim(), role: newMemberRole },
                      {
                        onSuccess: () => {
                          toast.success('멤버가 추가되었습니다')
                          setNewMemberUserId('')
                        },
                        onError: (err) => toast.error(formatError(err)),
                      },
                    )
                  }}
                >
                  추가
                </Button>
              </div>
            </Card>
          </section>
        )}

        {canManage && (
          <section>
            <h2 className="mb-1 text-lg font-semibold">데이터 열람 범위</h2>
            <p className="mb-3 text-sm text-muted-foreground">
              열람자(viewer) 역할의 멤버가 볼 수 있는 데이터 범위를 제한합니다.
            </p>
            <Select
              value={collection.access_config?.rls_mode || 'none'}
              onValueChange={(v) => {
                const mode = v === 'none' ? '' : v
                updateCollection.mutate(
                  {
                    access_config: {
                      ...collection.access_config,
                      rls_mode: mode as '' | 'none' | 'creator' | 'department' | 'subsidiary' | 'filter',
                    },
                  },
                  {
                    onSuccess: () => toast.success('열람 범위 설정이 저장되었습니다'),
                    onError: (err) => toast.error(formatError(err)),
                  },
                )
              }}
            >
              <SelectTrigger className="w-64">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">제한 없음 (모든 행 열람)</SelectItem>
                <SelectItem value="creator">본인 작성 행만 열람</SelectItem>
                <SelectItem value="department">같은 부서 행만 열람</SelectItem>
                <SelectItem value="subsidiary">같은 법인 행만 열람</SelectItem>
                <SelectItem value="filter">커스텀 필터 규칙</SelectItem>
              </SelectContent>
            </Select>

            {collection.access_config?.rls_mode === 'filter' && (
              <RLSFilterEditor
                filters={collection.access_config?.rls_filters ?? []}
                fields={(collection.fields ?? []).map((f) => ({ slug: f.slug, label: f.label }))}
                onSave={(filters) => {
                  updateCollection.mutate(
                    {
                      access_config: {
                        ...collection.access_config,
                        rls_filters: filters,
                      },
                    },
                    {
                      onSuccess: () => toast.success('RLS 필터 규칙이 저장되었습니다'),
                      onError: (err) => toast.error(formatError(err)),
                    },
                  )
                }}
              />
            )}
          </section>
        )}

        {canManage && (
          <ListSettingsSection collection={collection} updateCollection={updateCollection} />
        )}

        {canManage && (
          <section className="rounded-lg border border-destructive/30 bg-destructive/5 p-4">
            <h2 className="text-base font-semibold text-destructive">위험 영역</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              앱을 삭제하면 모든 항목과 변경 이력이 제거됩니다. 되돌리기 전에는
              데이터가 영구히 사라집니다.
            </p>
            <Button
              variant="destructive"
              size="sm"
              className="mt-3"
              onClick={() => setConfirmDeleteCollection(true)}
            >
              앱 삭제
            </Button>
          </section>
        )}
      </div>

      <ConfirmDialog
        open={confirmDeleteCollection}
        onOpenChange={setConfirmDeleteCollection}
        title="앱을 삭제하시겠습니까?"
        description={`"${collection.label}" 앱과 모든 데이터가 영구 삭제됩니다.`}
        variant="destructive"
        confirmLabel="삭제"
        onConfirm={handleDeleteCollection}
        loading={deleteCollection.isPending}
      />
    </div>
  )
}

function ListSettingsSection({
  collection,
  updateCollection,
}: {
  collection: NonNullable<ReturnType<typeof useCollection>['data']>
  updateCollection: ReturnType<typeof useUpdateCollection>
}) {
  const fields = collection.fields ?? []

  const titleCandidates = useMemo(
    () => fields.filter((f) => !isLayoutType(f.field_type) && !isComputedType(f.field_type)),
    [fields],
  )

  const sortCandidates = useMemo(
    () => [
      { slug: 'created_at', label: '생성일' },
      { slug: 'updated_at', label: '수정일' },
      ...fields
        .filter((f) => !isLayoutType(f.field_type) && !isComputedType(f.field_type))
        .map((f) => ({ slug: f.slug, label: f.label })),
    ],
    [fields],
  )

  return (
    <section>
      <h2 className="mb-1 text-lg font-semibold">목록 표시 설정</h2>
      <p className="mb-3 text-sm text-muted-foreground">
        데이터 목록의 제목열과 기본 정렬 순서를 설정합니다.
      </p>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div>
          <Label>제목열</Label>
          <p className="mb-1.5 text-xs text-muted-foreground">캘린더, 간트 등에서 레코드 이름으로 표시됩니다.</p>
          <Select
            value={collection.title_field_id || '_auto'}
            onValueChange={(v) => {
              updateCollection.mutate(
                { title_field_id: v === '_auto' || v === null ? '' : v },
                {
                  onSuccess: () => toast.success('제목열이 변경되었습니다'),
                  onError: (err) => toast.error(formatError(err)),
                },
              )
            }}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="_auto">자동 (첫 번째 텍스트 필드)</SelectItem>
              {titleCandidates.map((f) => (
                <SelectItem key={f.id} value={f.id}>{f.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div>
          <Label>기본 정렬 항목</Label>
          <p className="mb-1.5 text-xs text-muted-foreground">목록 진입 시 기본으로 적용되는 정렬 기준입니다.</p>
          <Select
            value={collection.default_sort_field || '_default'}
            onValueChange={(v) => {
              updateCollection.mutate(
                { default_sort_field: v === '_default' || v === null ? '' : v },
                {
                  onSuccess: () => toast.success('기본 정렬이 변경되었습니다'),
                  onError: (err) => toast.error(formatError(err)),
                },
              )
            }}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="_default">기본 (생성일 내림차순)</SelectItem>
              {sortCandidates.map((f) => (
                <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div>
          <Label>정렬 순서</Label>
          <p className="mb-1.5 text-xs text-muted-foreground">오름차순(A-Z, 1-9) 또는 내림차순(Z-A, 9-1).</p>
          <Select
            value={collection.default_sort_order || 'desc'}
            onValueChange={(v) => {
              updateCollection.mutate(
                { default_sort_order: v as 'asc' | 'desc' },
                {
                  onSuccess: () => toast.success('정렬 순서가 변경되었습니다'),
                  onError: (err) => toast.error(formatError(err)),
                },
              )
            }}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="asc">오름차순</SelectItem>
              <SelectItem value="desc">내림차순</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
    </section>
  )
}

const RLS_OPS = [
  { value: 'eq', label: '같음 (=)' },
  { value: 'neq', label: '다름 (!=)' },
  { value: 'in', label: '포함 (IN)' },
  { value: 'contains', label: '텍스트 포함' },
] as const

const RLS_USER_VARS = [
  { value: '$user.id', label: '현재 사용자 ID' },
  { value: '$user.department_id', label: '현재 사용자 부서' },
  { value: '$user.subsidiary_id', label: '현재 사용자 법인' },
  { value: '$user.email', label: '현재 사용자 이메일' },
]

function RLSFilterEditor({
  filters,
  fields,
  onSave,
}: {
  filters: RLSFilter[]
  fields: { slug: string; label: string }[]
  onSave: (filters: RLSFilter[]) => void
}) {
  const [draft, setDraft] = useState<RLSFilter[]>(filters.length > 0 ? filters : [{ field: '', op: 'eq', value: '' }])

  const addRule = () => setDraft((prev) => [...prev, { field: '', op: 'eq', value: '' }])
  const removeRule = (i: number) => setDraft((prev) => prev.filter((_, idx) => idx !== i))
  const updateRule = (i: number, patch: Partial<RLSFilter>) =>
    setDraft((prev) => prev.map((r, idx) => (idx === i ? { ...r, ...patch } : r)))

  const canSave = draft.every((r) => r.field && r.op && r.value)

  return (
    <div className="mt-4 space-y-3">
      <p className="text-sm text-muted-foreground">
        필터 규칙을 정의하면 열람자는 모든 조건을 만족하는 행만 볼 수 있습니다.
        값에 <code className="rounded bg-muted px-1 text-xs">$user.department_id</code> 등을 사용하면 현재 사용자 속성으로 자동 치환됩니다.
      </p>
      {draft.map((rule, i) => (
        <div key={i} className="flex items-center gap-2">
          <Select value={rule.field || undefined} onValueChange={(v) => updateRule(i, { field: v ?? '' })}>
            <SelectTrigger className="w-40"><SelectValue placeholder="필드" /></SelectTrigger>
            <SelectContent>
              {fields.map((f) => (
                <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
              ))}
              <SelectItem value="created_by">작성자 ID</SelectItem>
            </SelectContent>
          </Select>
          <Select value={rule.op} onValueChange={(v) => updateRule(i, { op: (v ?? 'eq') as RLSFilter['op'] })}>
            <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
            <SelectContent>
              {RLS_OPS.map((op) => (
                <SelectItem key={op.value} value={op.value}>{op.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex-1">
            <Select
              value={rule.value.startsWith('$user.') ? rule.value : '__custom__'}
              onValueChange={(v) => {
                if (!v || v === '__custom__') updateRule(i, { value: '' })
                else updateRule(i, { value: v })
              }}
            >
              <SelectTrigger className="w-full"><SelectValue placeholder="값 선택" /></SelectTrigger>
              <SelectContent>
                {RLS_USER_VARS.map((uv) => (
                  <SelectItem key={uv.value} value={uv.value}>{uv.label}</SelectItem>
                ))}
                <SelectItem value="__custom__">직접 입력</SelectItem>
              </SelectContent>
            </Select>
            {!rule.value.startsWith('$user.') && (
              <Input
                className="mt-1"
                placeholder="직접 입력 값"
                value={rule.value}
                onChange={(e) => updateRule(i, { value: e.target.value })}
              />
            )}
          </div>
          <Button variant="ghost" size="sm" onClick={() => removeRule(i)} disabled={draft.length <= 1}>
            &times;
          </Button>
        </div>
      ))}
      <div className="flex gap-2">
        <Button variant="outline" size="sm" onClick={addRule}>+ 규칙 추가</Button>
        <Button size="sm" disabled={!canSave} onClick={() => onSave(draft.filter((r) => r.field && r.value))}>
          저장
        </Button>
      </div>
    </div>
  )
}
