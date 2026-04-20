import { useState } from 'react'
import { toast } from 'sonner'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  useDepartments,
  useCreateDepartment,
  useUpdateDepartment,
  useDeleteDepartment,
} from '@/hooks/useDepartments'
import { useSubsidiaries } from '@/hooks/useSubsidiaries'
import { formatError } from '@/lib/api'
import type { Department } from '@/lib/types'

const NONE = '__none__'

export default function DepartmentPanel() {
  const { data: departments } = useDepartments()
  const { data: subsidiaries } = useSubsidiaries()
  const createDept = useCreateDepartment()
  const updateDept = useUpdateDepartment()
  const deleteDept = useDeleteDepartment()

  const [newName, setNewName] = useState('')
  const [newParent, setNewParent] = useState(NONE)
  const [newSubsidiary, setNewSubsidiary] = useState(NONE)
  const [editing, setEditing] = useState<Department | null>(null)
  const [editName, setEditName] = useState('')
  const [editParent, setEditParent] = useState(NONE)
  const [editSubsidiary, setEditSubsidiary] = useState(NONE)
  const [deleting, setDeleting] = useState<Department | null>(null)

  function handleCreate() {
    if (!newName.trim()) return
    createDept.mutate(
      {
        name: newName.trim(),
        parent_id: newParent === NONE ? null : newParent,
        subsidiary_id: newSubsidiary === NONE ? null : newSubsidiary,
      },
      {
        onSuccess: () => {
          toast.success('부서가 생성되었습니다')
          setNewName('')
          setNewParent(NONE)
          setNewSubsidiary(NONE)
        },
        onError: (err) => toast.error(formatError(err)),
      },
    )
  }

  function handleUpdate() {
    if (!editing || !editName.trim()) return
    updateDept.mutate(
      {
        id: editing.id,
        name: editName.trim(),
        parent_id: editParent === NONE ? '' : editParent,
        subsidiary_id: editSubsidiary === NONE ? '' : editSubsidiary,
      },
      {
        onSuccess: () => {
          toast.success('부서가 수정되었습니다')
          setEditing(null)
        },
        onError: (err) => toast.error(formatError(err)),
      },
    )
  }

  function startEdit(d: Department) {
    setEditing(d)
    setEditName(d.name)
    setEditParent(d.parent_id ?? NONE)
    setEditSubsidiary(d.subsidiary_id ?? NONE)
  }

  // Build indented list: depth-first walk from roots.
  const deptList = departments ?? []
  const childrenOf = new Map<string, Department[]>()
  const roots: Department[] = []
  for (const d of deptList) {
    if (d.parent_id) {
      const arr = childrenOf.get(d.parent_id) ?? []
      arr.push(d)
      childrenOf.set(d.parent_id, arr)
    } else {
      roots.push(d)
    }
  }

  interface FlatNode { dept: Department; depth: number }
  const flat: FlatNode[] = []
  function walk(dept: Department, depth: number) {
    flat.push({ dept, depth })
    const children = childrenOf.get(dept.id) ?? []
    children.sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name))
    for (const c of children) walk(c, depth + 1)
  }
  roots.sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name))
  for (const r of roots) walk(r, 0)

  return (
    <div className="space-y-4 rounded-lg border border-stone-200 bg-white p-4">
      <h2 className="text-sm font-semibold text-stone-900">부서 관리</h2>

      {/* Create form */}
      <div className="space-y-2">
        <Input
          placeholder="부서명"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
        />
        <Select value={newSubsidiary} onValueChange={(v) => setNewSubsidiary(v ?? NONE)}>
          <SelectTrigger className="text-xs">
            <SelectValue placeholder="소속 계열사" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE}>미지정</SelectItem>
            {(subsidiaries ?? []).map((s) => (
              <SelectItem key={s.id} value={s.id}>
                {s.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={newParent} onValueChange={(v) => setNewParent(v ?? NONE)}>
          <SelectTrigger className="text-xs">
            <SelectValue placeholder="상위 부서" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE}>최상위</SelectItem>
            {deptList.map((d) => (
              <SelectItem key={d.id} value={d.id}>
                {d.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button size="sm" onClick={handleCreate} disabled={createDept.isPending} className="w-full">
          부서 추가
        </Button>
      </div>

      {/* Department tree list */}
      <div className="space-y-1">
        {flat.map(({ dept, depth }) => (
          <div
            key={dept.id}
            className="group flex items-center gap-1 rounded px-2 py-1 text-sm hover:bg-stone-50"
            style={{ paddingLeft: `${depth * 16 + 8}px` }}
          >
            {editing?.id === dept.id ? (
              <div className="flex flex-1 items-center gap-1">
                <Input
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  className="h-7 text-xs"
                  onKeyDown={(e) => e.key === 'Enter' && handleUpdate()}
                />
                <Select value={editParent} onValueChange={(v) => setEditParent(v ?? NONE)}>
                  <SelectTrigger className="h-7 w-24 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NONE}>최상위</SelectItem>
                    {deptList
                      .filter((d) => d.id !== dept.id)
                      .map((d) => (
                        <SelectItem key={d.id} value={d.id}>
                          {d.name}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
                <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={handleUpdate}>
                  저장
                </Button>
                <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={() => setEditing(null)}>
                  취소
                </Button>
              </div>
            ) : (
              <>
                <span className="flex-1 truncate">{dept.name}</span>
                <Button
                  size="sm"
                  variant="ghost"
                  className="hidden h-6 px-1.5 text-xs group-hover:inline-flex"
                  onClick={() => startEdit(dept)}
                >
                  편집
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  className="hidden h-6 px-1.5 text-xs text-destructive group-hover:inline-flex"
                  onClick={() => setDeleting(dept)}
                >
                  삭제
                </Button>
              </>
            )}
          </div>
        ))}
        {flat.length === 0 && (
          <p className="py-4 text-center text-xs text-muted-foreground">부서가 없습니다</p>
        )}
      </div>

      {deleting && (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setDeleting(null)}
          title="부서 삭제"
          description={`"${deleting.name}" 부서를 삭제하시겠습니까? 하위 부서와 소속 사용자의 부서 배정이 해제됩니다.`}
          variant="destructive"
          confirmLabel="삭제"
          loading={deleteDept.isPending}
          onConfirm={() =>
            deleteDept.mutate(deleting.id, {
              onSuccess: () => {
                toast.success('부서가 삭제되었습니다')
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
