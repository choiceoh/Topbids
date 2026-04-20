import { useState } from 'react'
import { Loader2, Sparkles } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import { useAIBuildAutomation } from '@/hooks/useAIAutomation'
import type { CreateAutomationReq } from '@/lib/types'

const TRIGGER_LABELS: Record<string, string> = {
  record_created: '데이터 생성',
  record_updated: '데이터 수정',
  record_deleted: '데이터 삭제',
  status_change: '상태 변경',
}

const ACTION_LABELS: Record<string, string> = {
  send_notification: '알림 발송',
  update_field: '항목 값 업데이트',
  call_webhook: 'Webhook 호출',
}

const OPERATOR_LABELS: Record<string, string> = {
  equals: '같음',
  not_equals: '같지 않음',
  contains: '포함',
  gt: '초과',
  lt: '미만',
  is_empty: '비어있음',
  is_not_empty: '비어있지 않음',
}

interface Props {
  collectionId: string
  onApply: (result: CreateAutomationReq) => void
}

export default function AIAutomationDialog({ collectionId, onApply }: Props) {
  const aiAvailable = useAIAvailable()
  const [open, setOpen] = useState(false)
  const [description, setDescription] = useState('')
  const [result, setResult] = useState<CreateAutomationReq | null>(null)

  const buildMutation = useAIBuildAutomation(collectionId)

  function handleGenerate() {
    if (!description.trim()) return
    setResult(null)
    buildMutation.mutate(description, {
      onSuccess: (data) => setResult(data),
    })
  }

  function handleApply() {
    if (!result) return
    onApply(result)
    setOpen(false)
    resetState()
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) resetState()
  }

  function resetState() {
    setDescription('')
    setResult(null)
    buildMutation.reset()
  }

  if (!aiAvailable) return null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger className="inline-flex items-center justify-center gap-1.5 rounded-md border border-input bg-background px-3 py-1.5 text-sm font-medium ring-offset-background hover:bg-accent hover:text-accent-foreground">
        <Sparkles className="h-4 w-4" />
        AI로 만들기
      </DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>AI 자동화 빌더</DialogTitle>
          <DialogDescription>
            어떤 자동화를 만들고 싶은지 설명해 주세요. AI가 적절한 자동화 규칙을
            제안합니다.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <Textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="예: 우선순위가 '긴급'인 데이터가 생성되면 담당자에게 알림을 보내주세요"
            rows={3}
            disabled={buildMutation.isPending}
          />

          {!result && (
            <div className="flex justify-end">
              <Button
                onClick={handleGenerate}
                disabled={!description.trim() || buildMutation.isPending}
              >
                {buildMutation.isPending ? (
                  <>
                    <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
                    생성 중...
                  </>
                ) : (
                  <>
                    <Sparkles className="mr-1.5 h-4 w-4" />
                    생성하기
                  </>
                )}
              </Button>
            </div>
          )}

          {buildMutation.isError && (
            <p className="text-sm text-destructive">
              {buildMutation.error?.message ?? 'AI 요청에 실패했습니다'}
            </p>
          )}

          {result && (
            <div className="space-y-3 rounded-lg border p-4">
              {/* Name + trigger */}
              <div>
                <h4 className="text-sm font-semibold">{result.name}</h4>
                <Badge variant="secondary" className="mt-1">
                  {TRIGGER_LABELS[result.trigger_type] ?? result.trigger_type}
                </Badge>
                {result.trigger_type === 'status_change' &&
                  result.trigger_config && (
                    <span className="ml-2 text-xs text-muted-foreground">
                      {(result.trigger_config.from_status as string) || '모든 상태'} →{' '}
                      {(result.trigger_config.to_status as string) || '모든 상태'}
                    </span>
                  )}
              </div>

              {/* Conditions */}
              {result.conditions.length > 0 && (
                <div>
                  <p className="text-xs font-medium text-muted-foreground mb-1">
                    조건 ({result.conditions.length}개)
                  </p>
                  <div className="space-y-1">
                    {result.conditions.map((c, i) => (
                      <div
                        key={i}
                        className="flex items-center gap-1.5 rounded border px-2.5 py-1 text-xs"
                      >
                        <span className="font-medium">{c.field_slug}</span>
                        <Badge variant="outline" className="text-[10px]">
                          {OPERATOR_LABELS[c.operator] ?? c.operator}
                        </Badge>
                        {c.value && (
                          <span className="text-muted-foreground">
                            &quot;{c.value}&quot;
                          </span>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Actions */}
              <div>
                <p className="text-xs font-medium text-muted-foreground mb-1">
                  액션 ({result.actions.length}개)
                </p>
                <div className="space-y-1">
                  {result.actions.map((a, i) => (
                    <div
                      key={i}
                      className="rounded border px-2.5 py-1.5 text-xs"
                    >
                      <Badge variant="secondary" className="text-[10px]">
                        {ACTION_LABELS[a.action_type] ?? a.action_type}
                      </Badge>
                      <span className="ml-2 text-muted-foreground">
                        {summarizeAction(a.action_type, a.action_config)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}
        </div>

        {result && (
          <DialogFooter>
            <Button variant="outline" onClick={() => setResult(null)}>
              다시 생성
            </Button>
            <Button onClick={handleApply}>적용하기</Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}

function summarizeAction(
  type: string,
  config: Record<string, unknown>,
): string {
  switch (type) {
    case 'send_notification':
      return `${(config.title as string) || '알림'} → ${(config.recipient as string) === 'record_creator' ? '작성자' : (config.recipient as string) === 'field_ref' ? `${config.field_slug} 항목` : '지정 사용자'}`
    case 'update_field':
      return `${config.field_slug} = "${config.value}"`
    case 'call_webhook':
      return (config.url as string) || 'URL 미설정'
    default:
      return JSON.stringify(config)
  }
}
