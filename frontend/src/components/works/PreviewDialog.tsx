import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { Preview, SafetyLevel } from '@/lib/types'

interface Props {
  open: boolean
  preview: Preview | null
  onConfirm: () => void
  onCancel: () => void
  confirming?: boolean
}

const SAFETY_VARIANT: Record<SafetyLevel, 'secondary' | 'outline' | 'destructive'> = {
  SAFE: 'secondary',
  CAUTIOUS: 'outline',
  DANGEROUS: 'destructive',
}

const SAFETY_LABEL: Record<SafetyLevel, string> = {
  SAFE: '안전',
  CAUTIOUS: '주의',
  DANGEROUS: '위험',
}

export default function PreviewDialog({ open, preview, onConfirm, onCancel, confirming }: Props) {
  if (!preview) return null

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onCancel()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <span>작업 확인</span>
            <Badge variant={SAFETY_VARIANT[preview.safety_level]}>
              {SAFETY_LABEL[preview.safety_level]}
            </Badge>
          </DialogTitle>
          <DialogDescription>{preview.description}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 text-sm">
          <div className="flex gap-6">
            <div>
              <p className="text-xs text-muted-foreground">영향 받는 행</p>
              <p className="text-lg font-semibold">{preview.affected_rows.toLocaleString()}</p>
            </div>
            {preview.incompatible_rows !== undefined && preview.incompatible_rows > 0 && (
              <div>
                <p className="text-xs text-muted-foreground">비호환 행</p>
                <p className="text-lg font-semibold text-destructive">
                  {preview.incompatible_rows.toLocaleString()}
                </p>
              </div>
            )}
          </div>

          {preview.warnings && preview.warnings.length > 0 && (
            <div className="rounded-md border border-amber-300 bg-amber-50 p-3">
              <p className="mb-1 text-xs font-semibold text-amber-700">경고</p>
              <ul className="list-disc space-y-1 pl-4 text-xs text-amber-900">
                {preview.warnings.map((w, i) => (
                  <li key={i}>{w}</li>
                ))}
              </ul>
            </div>
          )}

          {preview.incompatible_sample && preview.incompatible_sample.length > 0 && (
            <div>
              <p className="mb-1 text-xs font-semibold text-muted-foreground">
                비호환 데이터 샘플 (최대 5건)
              </p>
              <pre className="overflow-x-auto rounded bg-muted p-2 text-xs">
                {JSON.stringify(preview.incompatible_sample, null, 2)}
              </pre>
            </div>
          )}

          <div>
            <p className="mb-1 text-xs font-semibold text-muted-foreground">실행 DDL</p>
            <pre className="overflow-x-auto rounded bg-muted p-2 text-xs">{preview.ddl_up}</pre>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onCancel} disabled={confirming}>
            취소
          </Button>
          <Button
            variant={preview.safety_level === 'DANGEROUS' ? 'destructive' : 'default'}
            onClick={onConfirm}
            disabled={confirming}
          >
            {confirming ? '실행 중...' : '확인하고 실행'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
