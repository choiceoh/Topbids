import { useCallback, useEffect, useRef, useState } from 'react'
import { Loader2, Sparkles } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import { useAIBuildFormula } from '@/hooks/useAI'
import { api } from '@/lib/api'
import type { Field } from '@/lib/types'

interface FormulaPreviewResult {
  valid: boolean
  error: string
  sql: string
  refs: string[]
  samples: unknown[]
}

interface Props {
  expression: string
  resultType: string
  precision?: number
  slug: string | undefined
  fields: Field[]
  onChange: (expression: string) => void
  onResultTypeChange: (resultType: string) => void
  onPrecisionChange: (precision: number | undefined) => void
}

const RESULT_TYPES = [
  { value: 'number', label: '숫자' },
  { value: 'integer', label: '정수' },
  { value: 'text', label: '텍스트' },
  { value: 'boolean', label: '체크박스' },
  { value: 'date', label: '날짜' },
]

const FORMULA_HELP = [
  { label: '산술', examples: '+ - * / %' },
  { label: '함수', examples: 'SUM, AVG, MIN, MAX, COUNT, ROUND, ABS, COALESCE' },
  { label: '조건', examples: 'IF(조건, 참, 거짓)' },
  { label: '참조', examples: 'LOOKUP(연결항목, 대상항목)' },
  { label: '요약', examples: 'SUMREL(연결항목, 대상항목), AVGREL, COUNTREL' },
]

export default function FormulaEditor({
  expression,
  resultType,
  precision,
  slug,
  fields,
  onChange,
  onResultTypeChange,
  onPrecisionChange,
}: Props) {
  const [preview, setPreview] = useState<FormulaPreviewResult | null>(null)
  const [loading, setLoading] = useState(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(null)
  const [aiMode, setAiMode] = useState(false)
  const [aiPrompt, setAiPrompt] = useState('')
  const aiAvailable = useAIAvailable()
  const buildFormula = useAIBuildFormula(slug)

  // Available field slugs for autocomplete hint.
  const dataFields = fields.filter(
    (f) => !['label', 'line', 'spacer', 'formula'].includes(f.field_type),
  )
  const relationFields = fields.filter((f) => f.field_type === 'relation')

  const fetchPreview = useCallback(
    async (expr: string) => {
      if (!slug || !expr.trim()) {
        setPreview(null)
        return
      }
      setLoading(true)
      try {
        const result = await api.post<FormulaPreviewResult>(
          `/data/${slug}/formula-preview`,
          { expression: expr, result_type: resultType },
          { raw: true },
        )
        setPreview(result)
      } catch {
        setPreview(null)
      } finally {
        setLoading(false)
      }
    },
    [slug, resultType],
  )

  // Debounced preview on expression change.
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => fetchPreview(expression), 500)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [expression, fetchPreview])

  return (
    <div className="space-y-3">
      {/* Expression input */}
      <div className="space-y-1">
        <div className="flex items-center justify-between">
          <Label className="text-xs font-semibold text-muted-foreground">수식</Label>
          {aiAvailable && slug && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-6 gap-1 px-2 text-xs"
              onClick={() => {
                setAiMode(!aiMode)
                setAiPrompt('')
              }}
            >
              <Sparkles className="h-3 w-3" />
              {aiMode ? '직접 입력' : 'AI 생성'}
            </Button>
          )}
        </div>
        {aiMode ? (
          <div className="flex gap-1">
            <Input
              value={aiPrompt}
              onChange={(e) => setAiPrompt(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && aiPrompt.trim() && !buildFormula.isPending) {
                  buildFormula.mutate(aiPrompt.trim(), {
                    onSuccess: (res) => {
                      onChange(res.expression)
                      setAiMode(false)
                      setAiPrompt('')
                    },
                  })
                }
              }}
              placeholder="예: 단가 곱하기 수량에서 할인 빼기"
              className="text-sm"
              disabled={buildFormula.isPending}
            />
            <Button
              type="button"
              size="sm"
              className="h-8 shrink-0"
              disabled={!aiPrompt.trim() || buildFormula.isPending}
              onClick={() => {
                buildFormula.mutate(aiPrompt.trim(), {
                  onSuccess: (res) => {
                    onChange(res.expression)
                    setAiMode(false)
                    setAiPrompt('')
                  },
                })
              }}
            >
              {buildFormula.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : '생성'}
            </Button>
          </div>
        ) : (
          <Input
            value={expression}
            onChange={(e) => onChange(e.target.value)}
            placeholder="=price * quantity"
            className="font-mono text-sm"
          />
        )}
        {buildFormula.isError && (
          <p className="text-xs text-destructive">AI 수식 생성에 실패했습니다</p>
        )}
      </div>

      {/* Result type */}
      <div className="space-y-1">
        <Label className="text-xs font-semibold text-muted-foreground">결과 유형</Label>
        <Select value={resultType || 'number'} onValueChange={(v) => v && onResultTypeChange(v)}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RESULT_TYPES.map((t) => (
              <SelectItem key={t.value} value={t.value}>{t.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Precision (for number result type) */}
      {(resultType === 'number') && (
        <div className="space-y-1">
          <Label className="text-xs font-semibold text-muted-foreground">소수점 자릿수</Label>
          <Input
            type="number"
            min={0}
            max={10}
            value={precision ?? ''}
            onChange={(e) => onPrecisionChange(e.target.value ? Number(e.target.value) : undefined)}
            placeholder="자동"
          />
        </div>
      )}

      {/* Available fields */}
      <div className="space-y-1">
        <Label className="text-xs font-semibold text-muted-foreground">사용 가능한 항목</Label>
        <div className="flex flex-wrap gap-1">
          {dataFields.map((f) => (
            <button
              key={f.id}
              type="button"
              className="rounded border px-1.5 py-0.5 text-xs font-mono hover:bg-accent"
              onClick={() => {
                const pos = expression.length
                const before = pos > 0 && expression[pos - 1] !== ' ' ? ' ' : ''
                onChange(expression + before + f.slug)
              }}
            >
              {f.slug}
            </button>
          ))}
        </div>
      </div>

      {/* Relation fields for cross-collection formulas */}
      {relationFields.length > 0 && (
        <div className="space-y-1">
          <Label className="text-xs font-semibold text-muted-foreground">연결 항목 (앱 간 참조)</Label>
          <div className="flex flex-wrap gap-1">
            {relationFields.map((f) => (
              <button
                key={f.id}
                type="button"
                className="rounded border border-dashed px-1.5 py-0.5 text-xs font-mono hover:bg-accent"
                onClick={() => {
                  const before = expression.length > 0 && expression[expression.length - 1] !== '(' ? '' : ''
                  onChange(expression + before + `LOOKUP(${f.slug}, )`)
                }}
                title={`LOOKUP(${f.slug}, 대상항목) 또는 SUMREL(${f.slug}, 대상항목)`}
              >
                {f.slug}
              </button>
            ))}
          </div>
          <p className="text-[10px] text-muted-foreground">
            클릭하면 LOOKUP 함수가 삽입됩니다. SUMREL/AVGREL/COUNTREL도 사용 가능
          </p>
        </div>
      )}

      {/* Preview result */}
      {preview && (
        <div className="rounded-md border bg-muted/30 p-2 space-y-1">
          <div className="flex items-center gap-2">
            <Badge variant={preview.valid ? 'default' : 'destructive'} className="text-[10px]">
              {preview.valid ? '유효' : '오류'}
            </Badge>
            {loading && <span className="text-xs text-muted-foreground">검증 중...</span>}
          </div>
          {preview.error && (
            <p className="text-xs text-destructive">{preview.error}</p>
          )}
          {preview.valid && preview.samples && preview.samples.length > 0 && (
            <div className="space-y-0.5">
              <p className="text-xs text-muted-foreground">미리보기 (처음 5건)</p>
              <div className="flex gap-2">
                {preview.samples.map((s, i) => (
                  <span key={i} className="rounded bg-background px-1.5 py-0.5 text-xs font-mono border">
                    {s == null ? 'NULL' : String(s)}
                  </span>
                ))}
              </div>
            </div>
          )}
          {preview.valid && preview.sql && (
            <p className="text-[10px] text-muted-foreground font-mono break-all">
              SQL: {preview.sql}
            </p>
          )}
        </div>
      )}

      {/* Help */}
      <div className="rounded-md border p-2 space-y-1">
        <p className="text-xs font-semibold text-muted-foreground">수식 도움말</p>
        {FORMULA_HELP.map((h) => (
          <p key={h.label} className="text-[10px] text-muted-foreground">
            <span className="font-medium">{h.label}:</span> {h.examples}
          </p>
        ))}
      </div>
    </div>
  )
}
