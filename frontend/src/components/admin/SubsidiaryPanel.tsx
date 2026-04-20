import { useState } from 'react'
import { toast } from 'sonner'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  useSubsidiaries,
  useCreateSubsidiary,
  useUpdateSubsidiary,
  useDeleteSubsidiary,
} from '@/hooks/useSubsidiaries'
import { formatError } from '@/lib/api'
import type { Subsidiary } from '@/lib/types'

export default function SubsidiaryPanel() {
  const { data: subsidiaries } = useSubsidiaries()
  const createSub = useCreateSubsidiary()
  const updateSub = useUpdateSubsidiary()
  const deleteSub = useDeleteSubsidiary()

  const [newName, setNewName] = useState('')
  const [editing, setEditing] = useState<Subsidiary | null>(null)
  const [editName, setEditName] = useState('')
  const [deleting, setDeleting] = useState<Subsidiary | null>(null)

  function handleCreate() {
    if (!newName.trim()) return
    createSub.mutate(
      { name: newName.trim() },
      {
        onSuccess: () => {
          toast.success('계열사가 생성되었습니다')
          setNewName('')
        },
        onError: (err) => toast.error(formatError(err)),
      },
    )
  }

  function handleUpdate() {
    if (!editing || !editName.trim()) return
    updateSub.mutate(
      { id: editing.id, name: editName.trim() },
      {
        onSuccess: () => {
          toast.success('계열사가 수정되었습니다')
          setEditing(null)
        },
        onError: (err) => toast.error(formatError(err)),
      },
    )
  }

  function startEdit(s: Subsidiary) {
    setEditing(s)
    setEditName(s.name)
  }

  const subList = (subsidiaries ?? []).slice().sort(
    (a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name),
  )

  return (
    <div className="space-y-4 rounded-lg border border-stone-200 bg-white p-4">
      <h2 className="text-sm font-semibold text-stone-900">계열사 관리</h2>

      <div className="space-y-2">
        <Input
          placeholder="계열사명"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
        />
        <Button size="sm" onClick={handleCreate} disabled={createSub.isPending} className="w-full">
          계열사 추가
        </Button>
      </div>

      <div className="space-y-1">
        {subList.map((s) => (
          <div
            key={s.id}
            className="group flex items-center gap-1 rounded px-2 py-1 text-sm hover:bg-stone-50"
          >
            {editing?.id === s.id ? (
              <div className="flex flex-1 items-center gap-1">
                <Input
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  className="h-7 text-xs"
                  onKeyDown={(e) => e.key === 'Enter' && handleUpdate()}
                />
                <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={handleUpdate}>
                  저장
                </Button>
                <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={() => setEditing(null)}>
                  취소
                </Button>
              </div>
            ) : (
              <>
                <span className="flex-1 truncate">{s.name}</span>
                <Button
                  size="sm"
                  variant="ghost"
                  className="hidden h-6 px-1.5 text-xs group-hover:inline-flex"
                  onClick={() => startEdit(s)}
                >
                  편집
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  className="hidden h-6 px-1.5 text-xs text-destructive group-hover:inline-flex"
                  onClick={() => setDeleting(s)}
                >
                  삭제
                </Button>
              </>
            )}
          </div>
        ))}
        {subList.length === 0 && (
          <p className="py-4 text-center text-xs text-muted-foreground">계열사가 없습니다</p>
        )}
      </div>

      {deleting && (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setDeleting(null)}
          title="계열사 삭제"
          description={`"${deleting.name}" 계열사를 삭제하시겠습니까? 소속 부서와 사용자의 계열사 배정이 해제됩니다.`}
          variant="destructive"
          confirmLabel="삭제"
          loading={deleteSub.isPending}
          onConfirm={() =>
            deleteSub.mutate(deleting.id, {
              onSuccess: () => {
                toast.success('계열사가 삭제되었습니다')
                setDeleting(null)
              },
              onError: (err) => toast.error(formatError(err)),
            })
          }
        />
      )}
    </div>
  )
}
