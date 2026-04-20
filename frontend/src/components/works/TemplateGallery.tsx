import { useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useCreateCollection } from '@/hooks/useCollections'
import { formatError } from '@/lib/api'
import { FIELD_TYPE_LABELS } from '@/lib/constants'
import {
  TEMPLATE_CATEGORIES,
  TEMPLATES,
  type Template,
  type TemplateCategory,
} from '@/lib/templates'

export default function TemplateGallery() {
  const navigate = useNavigate()
  const createCollection = useCreateCollection()
  const [selectedCategory, setSelectedCategory] = useState<TemplateCategory | null>(null)
  const [preview, setPreview] = useState<Template | null>(null)

  const filtered = selectedCategory
    ? TEMPLATES.filter((t) => t.category === selectedCategory)
    : TEMPLATES

  function handleUseTemplate() {
    if (!preview) return
    createCollection.mutate(preview.collection, {
      onSuccess: (created) => {
        toast.success(`${created.label} 앱이 생성되었습니다`)
        navigate(`/apps/${created.id}`)
      },
      onError: (err) => {
        toast.error(formatError(err))
      },
    })
  }

  return (
    <>
      <div className="space-y-4">
        {/* Category filter */}
        <div className="flex items-center gap-2">
          <Button
            variant={selectedCategory === null ? 'default' : 'outline'}
            onClick={() => setSelectedCategory(null)}
          >
            전체
          </Button>
          {TEMPLATE_CATEGORIES.map((cat) => (
            <Button
              key={cat}
              variant={selectedCategory === cat ? 'default' : 'outline'}
              onClick={() => setSelectedCategory(cat)}
            >
              {cat}
            </Button>
          ))}
        </div>

        {/* Template cards */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
          {filtered.map((tpl, i) => (
            <Card
              key={tpl.id}
              className={`cursor-pointer p-4 transition-all duration-200 hover:bg-accent hover:-translate-y-0.5 hover:shadow-md animate-scale-in stagger-${Math.min(i + 1, 12)}`}
              onClick={() => setPreview(tpl)}
            >
              <div className="mb-1 text-xs font-medium text-muted-foreground">
                {tpl.category}
              </div>
              <h4 className="text-sm font-semibold">{tpl.label}</h4>
              <p className="mt-1 text-xs text-muted-foreground line-clamp-2">
                {tpl.description}
              </p>
              <p className="mt-2 text-[11px] text-muted-foreground">
                {tpl.collection.fields?.length ?? 0}개 항목
              </p>
            </Card>
          ))}
        </div>
      </div>

      {/* Preview dialog */}
      <Dialog open={!!preview} onOpenChange={(open) => !open && setPreview(null)}>
        {preview && (
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>{preview.label}</DialogTitle>
              <DialogDescription>{preview.description}</DialogDescription>
            </DialogHeader>
            <div className="space-y-3">
              <div className="text-sm font-medium">포함된 항목</div>
              <div className="max-h-[300px] space-y-1 overflow-y-auto">
                {preview.collection.fields?.map((f) => (
                  <div
                    key={f.slug}
                    className="flex items-center justify-between rounded border px-3 py-1.5 text-sm"
                  >
                    <span>
                      {f.label}
                      {f.is_required && (
                        <span className="ml-1 text-xs text-destructive">*</span>
                      )}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {FIELD_TYPE_LABELS[f.field_type]}
                    </span>
                  </div>
                ))}
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setPreview(null)}>
                취소
              </Button>
              <Button
                onClick={handleUseTemplate}
                disabled={createCollection.isPending}
              >
                {createCollection.isPending ? <><Loader2 className="mr-1.5 h-4 w-4 animate-spin" />생성 중...</> : '이 템플릿 사용'}
              </Button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>
    </>
  )
}
