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
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import {
  useAIBuildCollection,
  type AIBuildQuestion,
  type AIBuildResult,
} from '@/hooks/useAI'
import { FIELD_TYPE_LABELS } from '@/lib/constants'
import type { FieldType } from '@/lib/types'

interface Props {
  onApply: (result: AIBuildResult) => void
}

export default function AIBuildDialog({ onApply }: Props) {
  const aiAvailable = useAIAvailable()
  const [open, setOpen] = useState(false)
  const [description, setDescription] = useState('')
  const [result, setResult] = useState<AIBuildResult | null>(null)
  const [questions, setQuestions] = useState<AIBuildQuestion[] | null>(null)
  const [answers, setAnswers] = useState<Record<string, string[]>>({})
  const [customTexts, setCustomTexts] = useState<Record<string, string>>({})

  const buildMutation = useAIBuildCollection()

  function handleGenerate() {
    if (!description.trim()) return
    setResult(null)
    setQuestions(null)
    setAnswers({})
    setCustomTexts({})
    buildMutation.mutate(
      { description },
      {
        onSuccess: (data) => {
          if (data.mode === 'questions' && data.questions?.length) {
            setQuestions(data.questions)
            const initial: Record<string, string[]> = {}
            const initialTexts: Record<string, string> = {}
            for (const q of data.questions) {
              initial[q.id] = []
              initialTexts[q.id] = ''
            }
            setAnswers(initial)
            setCustomTexts(initialTexts)
          } else if (data.schema) {
            setResult(data.schema)
          }
        },
      },
    )
  }

  function buildFlatAnswers(): Record<string, string> {
    const flat: Record<string, string> = {}
    for (const [id, selected] of Object.entries(answers)) {
      const custom = customTexts[id]?.trim()
      const parts = [...selected]
      if (custom) parts.push(custom)
      flat[id] = parts.join(', ')
    }
    return flat
  }

  function handleSubmitAnswers() {
    setQuestions(null)
    buildMutation.mutate(
      { description, answers: buildFlatAnswers() },
      {
        onSuccess: (data) => {
          if (data.schema) {
            setResult(data.schema)
          }
        },
      },
    )
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
    setQuestions(null)
    setAnswers({})
    setCustomTexts({})
    buildMutation.reset()
  }

  function toggleChoice(id: string, choice: string) {
    setAnswers((prev) => {
      const current = prev[id] ?? []
      const next = current.includes(choice)
        ? current.filter((c) => c !== choice)
        : [...current, choice]
      return { ...prev, [id]: next }
    })
  }

  function setCustomText(id: string, value: string) {
    setCustomTexts((prev) => ({ ...prev, [id]: value }))
  }

  const allAnswered = questions
    ? questions.every((q) => {
        const selected = answers[q.id] ?? []
        const custom = customTexts[q.id]?.trim() ?? ''
        return selected.length > 0 || custom.length > 0
      })
    : false

  if (!aiAvailable) return null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger className="inline-flex items-center justify-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground shadow-sm hover:bg-primary/90">
        <Sparkles className="h-4 w-4" />
        AI로 만들기
      </DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>AI 앱 빌더</DialogTitle>
          <DialogDescription>
            어떤 앱을 관리하고 싶은지 설명해 주세요. AI가 적절한 앱 구조를
            제안합니다.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* Description input */}
          <Textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="예: 프로젝트별 인허가 체크리스트를 관리하고 싶습니다. 각 항목에 담당자, 마감일, 진행상태, 첨부파일이 필요합니다."
            rows={4}
            disabled={buildMutation.isPending}
          />

          {/* Generate button — only show when no questions and no result */}
          {!questions && !result && (
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

          {/* Error */}
          {buildMutation.isError && (
            <p className="text-sm text-destructive">
              {buildMutation.error?.message ?? 'AI 요청에 실패했습니다'}
            </p>
          )}

          {/* Questions panel */}
          {questions && (
            <div className="space-y-4 rounded-lg border p-4">
              <div>
                <p className="text-sm font-medium">
                  AI가 더 정확한 앱을 만들기 위해 몇 가지를 확인하고 싶어합니다.
                </p>
              </div>
              {questions.map((q) => (
                <div key={q.id} className="space-y-1.5">
                  <label className="text-sm font-medium">{q.question}</label>
                  {q.choices?.length ? (
                    <div className="space-y-2">
                      <p className="text-xs text-muted-foreground">
                        복수 선택 가능
                      </p>
                      <div className="flex flex-wrap gap-2">
                        {q.choices.map((choice) => (
                          <Badge
                            key={choice}
                            variant={
                              (answers[q.id] ?? []).includes(choice)
                                ? 'default'
                                : 'outline'
                            }
                            className="cursor-pointer px-3 py-1 text-sm"
                            onClick={() => toggleChoice(q.id, choice)}
                          >
                            {choice}
                          </Badge>
                        ))}
                      </div>
                      <Input
                        value={customTexts[q.id] ?? ''}
                        onChange={(e) => setCustomText(q.id, e.target.value)}
                        placeholder={q.placeholder || '직접 입력'}
                        className="h-8 text-sm"
                      />
                    </div>
                  ) : (
                    <Input
                      value={customTexts[q.id] ?? ''}
                      onChange={(e) => setCustomText(q.id, e.target.value)}
                      placeholder={q.placeholder || '답변을 입력하세요'}
                      className="h-8 text-sm"
                    />
                  )}
                </div>
              ))}
              <div className="flex justify-end gap-2 pt-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={buildMutation.isPending}
                  onClick={() => {
                    setQuestions(null)
                    buildMutation.mutate(
                      { description, answers: { _skip: 'true' } },
                      {
                        onSuccess: (data) => {
                          if (data.schema) setResult(data.schema)
                        },
                      },
                    )
                  }}
                >
                  건너뛰고 생성
                </Button>
                <Button
                  size="sm"
                  onClick={handleSubmitAnswers}
                  disabled={!allAnswered || buildMutation.isPending}
                >
                  {buildMutation.isPending ? (
                    <>
                      <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
                      생성 중...
                    </>
                  ) : (
                    '답변 후 생성'
                  )}
                </Button>
              </div>
            </div>
          )}

          {/* Result preview */}
          {result && (
            <div className="space-y-3 rounded-lg border p-4">
              <div>
                <h4 className="text-sm font-semibold">{result.label}</h4>
                <p className="text-xs text-muted-foreground">{result.slug}</p>
                {result.description && (
                  <p className="mt-1 text-sm text-muted-foreground">
                    {result.description}
                  </p>
                )}
              </div>

              <div className="space-y-1">
                <p className="text-xs font-medium text-muted-foreground">
                  항목 {result.fields.length}개
                </p>
                <div className="max-h-60 space-y-1 overflow-y-auto">
                  {result.fields.map((f, i) => (
                    <div
                      key={i}
                      className="flex items-center justify-between rounded border px-3 py-1.5 text-sm"
                    >
                      <span>
                        {f.label}
                        <span className="ml-1.5 text-xs text-muted-foreground">
                          ({f.slug})
                        </span>
                      </span>
                      <span className="rounded bg-muted px-1.5 py-0.5 text-xs">
                        {FIELD_TYPE_LABELS[f.field_type as FieldType] ??
                          f.field_type}
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
            <Button
              variant="outline"
              onClick={() => {
                setResult(null)
                setQuestions(null)
              }}
            >
              다시 생성
            </Button>
            <Button onClick={handleApply}>적용하기</Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}
