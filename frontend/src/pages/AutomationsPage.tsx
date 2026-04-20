import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router'
import { toast } from 'sonner'
import { Plus, Trash2, Zap } from 'lucide-react'

import UserCombobox from '@/components/common/UserCombobox'
import AIAutomationDialog from '@/components/works/AIAutomationDialog'
import SchedulePicker, { isValidCron } from '@/components/works/SchedulePicker'
import ConfirmDialog from '@/components/common/ConfirmDialog'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useCollection } from '@/hooks/useCollections'
import {
  useAutomations,
  useCreateAutomation,
  useDeleteAutomation,
  useUpdateAutomation,
} from '@/hooks/useAutomations'
import { useProcess } from '@/hooks/useProcess'
import { useUnsavedChanges } from '@/hooks/useUnsavedChanges'
import { formatError } from '@/lib/api'
import type {
  ActionType,
  ConditionOperator,
  CreateAutomationReq,
  TriggerType,
} from '@/lib/types'

const TRIGGER_LABELS: Record<TriggerType, string> = {
  record_created: '데이터 생성',
  record_updated: '데이터 수정',
  record_deleted: '데이터 삭제',
  status_change: '상태 변경',
  schedule: '정해진 시간에 반복',
  form_submit: '폼 제출',
}

const ACTION_LABELS: Record<ActionType, string> = {
  send_notification: '알림 보내기',
  update_field: '항목 값 자동 변경',
  call_webhook: '외부 서비스에 알리기',
}

const OPERATOR_LABELS: Record<ConditionOperator, string> = {
  equals: '과(와) 같으면',
  not_equals: '과(와) 다르면',
  contains: '을(를) 포함하면',
  gt: '보다 크면',
  lt: '보다 작으면',
  is_empty: '비어있으면',
  is_not_empty: '값이 있으면',
}

interface ConditionDraft {
  field_slug: string
  operator: ConditionOperator
  value: string
}

interface ActionDraft {
  action_type: ActionType
  action_config: Record<string, unknown>
}

export default function AutomationsPage() {
  const { appId } = useParams()
  const { data: collection, isLoading: colLoading } = useCollection(appId)
  const { data: automations, isLoading: autoLoading, isError, error, refetch } = useAutomations(appId)
  const { data: process } = useProcess(appId)
  const createAutomation = useCreateAutomation(appId ?? '')
  const updateAutomation = useUpdateAutomation(appId ?? '')
  const deleteAutomation = useDeleteAutomation(appId ?? '')

  const [formOpen, setFormOpen] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [isEnabled, setIsEnabled] = useState(true)
  const [triggerType, setTriggerType] = useState<TriggerType>('record_created')
  const [fromStatus, setFromStatus] = useState('')
  const [toStatus, setToStatus] = useState('')
  const [cronExpr, setCronExpr] = useState('')
  const [cronTimezone, setCronTimezone] = useState('Asia/Seoul')
  const [formSlug, setFormSlug] = useState('')
  const [conditions, setConditions] = useState<ConditionDraft[]>([])
  const [actions, setActions] = useState<ActionDraft[]>([])

  const fields = collection?.fields?.filter((f) => !f.is_layout) ?? []
  const statuses = process?.statuses ?? []

  const isDirty = useMemo(
    () => formOpen && (name.trim() !== '' || conditions.length > 0 || actions.length > 0),
    [formOpen, name, conditions, actions],
  )
  const blocker = useUnsavedChanges(isDirty)

  function handleAIApply(result: CreateAutomationReq) {
    setEditingId(null)
    setName(result.name)
    setIsEnabled(result.is_enabled)
    setTriggerType(result.trigger_type)
    const tc = result.trigger_config ?? {}
    setFromStatus((tc.from_status as string) ?? '')
    setToStatus((tc.to_status as string) ?? '')
    setConditions(
      result.conditions.map((c) => ({
        field_slug: c.field_slug,
        operator: c.operator,
        value: c.value,
      })),
    )
    setActions(
      result.actions.map((a) => ({
        action_type: a.action_type,
        action_config: a.action_config,
      })),
    )
    setFormOpen(true)
  }

  if (colLoading || autoLoading) return <LoadingState variant="table" />
  if (isError) return <ErrorState error={error} onRetry={() => refetch()} />
  if (!collection) return null

  function resetForm() {
    setFormOpen(false)
    setEditingId(null)
    setName('')
    setIsEnabled(true)
    setTriggerType('record_created')
    setFromStatus('')
    setToStatus('')
    setCronExpr('')
    setCronTimezone('Asia/Seoul')
    setFormSlug('')
    setConditions([])
    setActions([])
  }

  function handleEdit(id: string) {
    const a = automations?.find((x) => x.id === id)
    if (!a) return

    // Need to fetch the full automation with conditions/actions
    setEditingId(id)
    setName(a.name)
    setIsEnabled(a.is_enabled)
    setTriggerType(a.trigger_type)
    const tc = a.trigger_config ?? {}
    setFromStatus((tc.from_status as string) ?? '')
    setToStatus((tc.to_status as string) ?? '')
    setCronExpr((tc.cron as string) ?? '')
    setCronTimezone((tc.timezone as string) ?? 'Asia/Seoul')
    setFormSlug((tc.form_slug as string) ?? '')
    setConditions(
      a.conditions?.map((c) => ({
        field_slug: c.field_slug,
        operator: c.operator,
        value: c.value,
      })) ?? [],
    )
    setActions(
      a.actions?.map((act) => ({
        action_type: act.action_type,
        action_config: act.action_config,
      })) ?? [],
    )
    setFormOpen(true)
  }

  function handleSave() {
    if (!name.trim()) {
      toast.error('이름을 입력해주세요')
      return
    }
    if (actions.length === 0) {
      toast.error('최소 하나의 액션을 추가해주세요')
      return
    }

    if (triggerType === 'schedule' && !isValidCron(cronExpr)) {
      toast.error('유효하지 않은 크론 표현식입니다')
      return
    }

    const triggerConfig: Record<string, unknown> = {}
    if (triggerType === 'status_change') {
      if (fromStatus) triggerConfig.from_status = fromStatus
      if (toStatus) triggerConfig.to_status = toStatus
    }
    if (triggerType === 'schedule') {
      triggerConfig.cron = cronExpr
      triggerConfig.timezone = cronTimezone
    }
    if (triggerType === 'form_submit') {
      if (formSlug) triggerConfig.form_slug = formSlug
    }

    const payload: CreateAutomationReq = {
      name: name.trim(),
      is_enabled: isEnabled,
      trigger_type: triggerType,
      trigger_config: triggerConfig,
      conditions: conditions.map((c) => ({
        field_slug: c.field_slug,
        operator: c.operator,
        value: c.value,
      })),
      actions: actions.map((a) => ({
        action_type: a.action_type,
        action_config: a.action_config,
      })),
    }

    if (editingId) {
      updateAutomation.mutate(
        { id: editingId, ...payload },
        {
          onSuccess: () => {
            toast.success('자동화가 수정되었습니다')
            resetForm()
          },
          onError: (err) => toast.error(formatError(err)),
        },
      )
    } else {
      createAutomation.mutate(payload, {
        onSuccess: () => {
          toast.success('자동화가 생성되었습니다')
          resetForm()
        },
        onError: (err) => toast.error(formatError(err)),
      })
    }
  }

  function handleDelete(id: string) {
    deleteAutomation.mutate(id, {
      onSuccess: () => toast.success('자동화가 삭제되었습니다'),
      onError: (err) => toast.error(formatError(err)),
    })
  }

  // --- Condition helpers ---
  function addCondition() {
    if (fields.length === 0) return
    setConditions([...conditions, { field_slug: fields[0].slug, operator: 'equals', value: '' }])
  }

  function removeCondition(idx: number) {
    setConditions(conditions.filter((_, i) => i !== idx))
  }

  function updateCondition(idx: number, patch: Partial<ConditionDraft>) {
    setConditions(conditions.map((c, i) => (i === idx ? { ...c, ...patch } : c)))
  }

  // --- Action helpers ---
  function addAction() {
    setActions([...actions, { action_type: 'send_notification', action_config: {} }])
  }

  function removeAction(idx: number) {
    setActions(actions.filter((_, i) => i !== idx))
  }

  function updateAction(idx: number, patch: Partial<ActionDraft>) {
    setActions(actions.map((a, i) => (i === idx ? { ...a, ...patch } : a)))
  }

  function updateActionConfig(idx: number, key: string, value: unknown) {
    setActions(
      actions.map((a, i) =>
        i === idx ? { ...a, action_config: { ...a.action_config, [key]: value } } : a,
      ),
    )
  }

  return (
    <div>
      <PageHeader
        breadcrumb={[
          { label: '앱 목록', href: '/apps' },
          { label: collection.label, href: `/apps/${collection.id}` },
          { label: '설정', href: `/apps/${collection.id}/settings` },
          { label: '자동화' },
        ]}
        title="자동화"
        actions={
          <div className="flex gap-2">
            {!formOpen && (
              <>
                <AIAutomationDialog
                  collectionId={collection.id}
                  onApply={handleAIApply}
                />
                <Button size="sm" onClick={() => setFormOpen(true)}>
                  <Plus className="mr-1 h-4 w-4" />
                  새 자동화
                </Button>
              </>
            )}
            <Link to={`/apps/${collection.id}/settings`}>
              <Button variant="outline" size="sm">관리 홈</Button>
            </Link>
          </div>
        }
      />

      <div className="space-y-4">
        {/* Form */}
        {formOpen && (
          <Card className="p-4 space-y-4">
            <h3 className="font-semibold">{editingId ? '자동화 수정' : '새 자동화'}</h3>

            {/* Name + enabled */}
            <div className="flex items-end gap-2">
              <div className="flex-1">
                <Label>이름</Label>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="예: 승인 시 알림 발송"
                />
              </div>
              <label className="flex items-center gap-2 pb-2 text-sm">
                <Checkbox
                  checked={isEnabled}
                  onCheckedChange={(c) => setIsEnabled(!!c)}
                />
                활성화
              </label>
            </div>

            {/* Trigger */}
            <div>
              <Label>트리거</Label>
              <Select value={triggerType} onValueChange={(v) => v && setTriggerType(v as TriggerType)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(Object.keys(TRIGGER_LABELS) as TriggerType[]).map((t) => (
                    <SelectItem key={t} value={t}>{TRIGGER_LABELS[t]}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Status change config */}
            {triggerType === 'status_change' && statuses.length > 0 && (
              <div className="flex gap-2">
                <div className="flex-1">
                  <Label>이전 상태 (선택)</Label>
                  <Select value={fromStatus} onValueChange={(v) => setFromStatus(v ?? '')}>
                    <SelectTrigger>
                      <SelectValue placeholder="모든 상태" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="">모든 상태</SelectItem>
                      {statuses.map((s) => (
                        <SelectItem key={s.id} value={s.name}>{s.name}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex items-end pb-2 text-muted-foreground">→</div>
                <div className="flex-1">
                  <Label>새 상태 (선택)</Label>
                  <Select value={toStatus} onValueChange={(v) => setToStatus(v ?? '')}>
                    <SelectTrigger>
                      <SelectValue placeholder="모든 상태" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="">모든 상태</SelectItem>
                      {statuses.map((s) => (
                        <SelectItem key={s.id} value={s.name}>{s.name}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            )}

            {/* Schedule config */}
            {triggerType === 'schedule' && (
              <div className="space-y-3">
                <SchedulePicker
                  value={cronExpr}
                  onChange={setCronExpr}
                />
                <div>
                  <Label>타임존</Label>
                  <Select value={cronTimezone} onValueChange={(v) => v && setCronTimezone(v)}>
                    <SelectTrigger className="w-48">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="Asia/Seoul">한국 표준시 (KST)</SelectItem>
                      <SelectItem value="UTC">세계 표준시 (UTC)</SelectItem>
                      <SelectItem value="Asia/Tokyo">일본 표준시 (JST)</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
            )}

            {/* Form submit config */}
            {triggerType === 'form_submit' && (
              <div>
                <Label>폼 슬러그 (선택)</Label>
                <Input
                  value={formSlug}
                  onChange={(e) => setFormSlug(e.target.value)}
                  placeholder="비워두면 모든 폼 제출 시 실행"
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  특정 외부 폼에서 제출된 경우만 실행하려면 폼 슬러그를 입력하세요.
                </p>
              </div>
            )}

            {/* Conditions */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <Label>조건 ({conditions.length})</Label>
                <Button variant="outline" size="sm" onClick={addCondition}>
                  + 조건 추가
                </Button>
              </div>
              {conditions.length > 0 && (
                <div className="space-y-2">
                  {conditions.map((c, idx) => (
                    <div key={idx} className="flex items-center gap-2">
                      <Select
                        value={c.field_slug}
                        onValueChange={(v) => v && updateCondition(idx, { field_slug: v })}
                      >
                        <SelectTrigger className="w-36">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {fields.map((f) => (
                            <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Select
                        value={c.operator}
                        onValueChange={(v) => v && updateCondition(idx, { operator: v as ConditionOperator })}
                      >
                        <SelectTrigger className="w-32">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {(Object.keys(OPERATOR_LABELS) as ConditionOperator[]).map((op) => (
                            <SelectItem key={op} value={op}>{OPERATOR_LABELS[op]}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {c.operator !== 'is_empty' && c.operator !== 'is_not_empty' && (
                        <Input
                          className="flex-1"
                          value={c.value}
                          onChange={(e) => updateCondition(idx, { value: e.target.value })}
                          placeholder="비교 값"
                        />
                      )}
                      <Button variant="ghost" size="sm" onClick={() => removeCondition(idx)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Actions */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <Label>액션 ({actions.length})</Label>
                <Button variant="outline" size="sm" onClick={addAction}>
                  + 액션 추가
                </Button>
              </div>
              {actions.length > 0 && (
                <div className="space-y-3">
                  {actions.map((a, idx) => (
                    <Card key={idx} className="p-3 space-y-2">
                      <div className="flex items-center gap-2">
                        <Select
                          value={a.action_type}
                          onValueChange={(v) => v && updateAction(idx, { action_type: v as ActionType, action_config: {} })}
                        >
                          <SelectTrigger className="w-48">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {(Object.keys(ACTION_LABELS) as ActionType[]).map((at) => (
                              <SelectItem key={at} value={at}>{ACTION_LABELS[at]}</SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        <div className="flex-1" />
                        <Button variant="ghost" size="sm" onClick={() => removeAction(idx)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>

                      {/* Action-specific config */}
                      {a.action_type === 'send_notification' && (
                        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                          <div>
                            <Label className="text-xs font-semibold text-muted-foreground">누구에게 보낼까요?</Label>
                            <Select
                              value={(a.action_config.recipient as string) ?? 'record_creator'}
                              onValueChange={(v) => updateActionConfig(idx, 'recipient', v)}
                            >
                              <SelectTrigger>
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="record_creator">데이터 작성자</SelectItem>
                                <SelectItem value="specific_user">지정한 사용자</SelectItem>
                                <SelectItem value="field_ref">담당자 항목에서 가져오기</SelectItem>
                              </SelectContent>
                            </Select>
                          </div>
                          {a.action_config.recipient === 'specific_user' && (
                            <div>
                              <Label className="text-xs font-semibold text-muted-foreground">받을 사용자</Label>
                              <UserCombobox
                                value={(a.action_config.user_id as string) ?? ''}
                                onChange={(v) => updateActionConfig(idx, 'user_id', v ?? '')}
                              />
                            </div>
                          )}
                          {a.action_config.recipient === 'field_ref' && (
                            <div>
                              <Label className="text-xs font-semibold text-muted-foreground">어떤 항목의 사용자에게?</Label>
                              <Select
                                value={(a.action_config.field_slug as string) ?? ''}
                                onValueChange={(v) => updateActionConfig(idx, 'field_slug', v)}
                              >
                                <SelectTrigger>
                                  <SelectValue placeholder="항목 선택" />
                                </SelectTrigger>
                                <SelectContent>
                                  {fields.filter((f) => f.field_type === 'user').map((f) => (
                                    <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                            </div>
                          )}
                          <div>
                            <Label className="text-xs font-semibold text-muted-foreground">알림 제목</Label>
                            <Input
                              value={(a.action_config.title as string) ?? ''}
                              onChange={(e) => updateActionConfig(idx, 'title', e.target.value)}
                              placeholder="예: 새 요청이 등록되었습니다"
                            />
                          </div>
                          <div>
                            <Label className="text-xs font-semibold text-muted-foreground">알림 내용</Label>
                            <Input
                              value={(a.action_config.body as string) ?? ''}
                              onChange={(e) => updateActionConfig(idx, 'body', e.target.value)}
                              placeholder="예: 확인 후 처리해주세요"
                            />
                          </div>
                        </div>
                      )}

                      {a.action_type === 'update_field' && (
                        <div className="grid grid-cols-2 gap-2">
                          <div>
                            <Label className="text-xs font-semibold text-muted-foreground">변경할 항목</Label>
                            <Select
                              value={(a.action_config.field_slug as string) ?? ''}
                              onValueChange={(v) => updateActionConfig(idx, 'field_slug', v)}
                            >
                              <SelectTrigger>
                                <SelectValue placeholder="항목 선택" />
                              </SelectTrigger>
                              <SelectContent>
                                {fields.map((f) => (
                                  <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                          <div>
                            <Label className="text-xs font-semibold text-muted-foreground">변경할 값</Label>
                            <Input
                              value={(a.action_config.value as string) ?? ''}
                              onChange={(e) => updateActionConfig(idx, 'value', e.target.value)}
                              placeholder="예: 완료"
                            />
                          </div>
                        </div>
                      )}

                      {a.action_type === 'call_webhook' && (
                        <div>
                          <Label className="text-xs font-semibold text-muted-foreground">연결할 주소 (URL)</Label>
                          <Input
                            value={(a.action_config.url as string) ?? ''}
                            onChange={(e) => updateActionConfig(idx, 'url', e.target.value)}
                            placeholder="https://..."
                          />
                        </div>
                      )}
                    </Card>
                  ))}
                </div>
              )}
            </div>

            {/* Form actions */}
            <div className="flex gap-2 border-t pt-3">
              <Button onClick={handleSave} disabled={createAutomation.isPending || updateAutomation.isPending}>
                {createAutomation.isPending || updateAutomation.isPending ? '저장 중...' : '저장'}
              </Button>
              <Button variant="outline" onClick={resetForm}>취소</Button>
            </div>
          </Card>
        )}

        {/* List */}
        {(!automations || automations.length === 0) && !formOpen && (
          <div className="py-12 text-center text-muted-foreground">
            <Zap className="mx-auto mb-2 h-8 w-8" />
            <p>아직 자동화가 없습니다</p>
            <p className="text-sm">데이터 생성/수정/삭제 시 자동으로 알림을 보내거나 항목을 업데이트할 수 있습니다.</p>
            <Button size="sm" className="mt-4" onClick={() => setFormOpen(true)}>
              <Plus className="mr-1 h-4 w-4" />
              새 자동화
            </Button>
          </div>
        )}

        {automations && automations.length > 0 && (
          <div className="space-y-2">
            {automations.map((a) => (
              <Card
                key={a.id}
                className="flex items-center justify-between p-3 cursor-pointer hover:bg-accent/50"
                onClick={() => handleEdit(a.id)}
              >
                <div className="flex items-center gap-2">
                  <Zap className="h-4 w-4 text-muted-foreground" />
                  <div>
                    <span className="font-medium">{a.name}</span>
                    <div className="flex items-center gap-2 mt-0.5">
                      <Badge variant="secondary">{TRIGGER_LABELS[a.trigger_type]}</Badge>
                      <span className="text-xs text-muted-foreground">
                        {a.action_count ?? 0}개 액션
                      </span>
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {!a.is_enabled && <Badge variant="outline">비활성</Badge>}
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={(e) => {
                      e.stopPropagation()
                      handleDelete(a.id)
                    }}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </Card>
            ))}
          </div>
        )}
      </div>

      <ConfirmDialog
        open={blocker.state === 'blocked'}
        onOpenChange={(open) => { if (!open) blocker.reset?.() }}
        title="저장하지 않고 나가시겠습니까?"
        description="작성 중인 자동화가 저장되지 않습니다."
        confirmLabel="나가기"
        cancelLabel="계속 작성"
        onConfirm={() => blocker.proceed?.()}
      />
    </div>
  )
}
