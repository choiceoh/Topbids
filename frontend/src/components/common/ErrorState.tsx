import { Button } from '@/components/ui/button'
import { formatError } from '@/lib/api'

interface Props {
  error: unknown
  onRetry?: () => void
  title?: string
}

export default function ErrorState({ error, onRetry, title = '불러오지 못했습니다' }: Props) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-destructive/30 bg-destructive/5 py-12 px-6 text-center">
      <h3 className="text-base font-medium text-destructive">{title}</h3>
      <p className="max-w-md text-sm text-muted-foreground">{formatError(error)}</p>
      {onRetry && (
        <Button onClick={onRetry} variant="outline" size="sm">
          다시 시도
        </Button>
      )}
    </div>
  )
}
