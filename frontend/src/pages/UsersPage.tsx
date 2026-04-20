import { type ColumnDef } from '@tanstack/react-table'
import { useState } from 'react'
import { toast } from 'sonner'

import { DataTable } from '@/components/common/DataTable'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useCurrentUser } from '@/hooks/useAuth'
import { useDepartments } from '@/hooks/useDepartments'
import { useSubsidiaries } from '@/hooks/useSubsidiaries'
import { useUsers, useUpdateUser } from '@/hooks/useUsers'
import { formatError } from '@/lib/api'
import { ROLE_LABELS } from '@/lib/constants'
import type { User, Department, Subsidiary } from '@/lib/types'

import UserFormDialog from '@/components/admin/UserFormDialog'
import DepartmentPanel from '@/components/admin/DepartmentPanel'
import SubsidiaryPanel from '@/components/admin/SubsidiaryPanel'

export default function UsersPage() {
  const { data: currentUser } = useCurrentUser()
  const { data: users, isLoading } = useUsers()
  const { data: departments } = useDepartments()
  const { data: subsidiaries } = useSubsidiaries()
  const updateUser = useUpdateUser()

  const [editingUser, setEditingUser] = useState<User | null>(null)
  const [showCreate, setShowCreate] = useState(false)

  const isDirector = currentUser?.role === 'director'

  const deptMap = new Map<string, Department>()
  departments?.forEach((d) => deptMap.set(d.id, d))

  const subMap = new Map<string, Subsidiary>()
  subsidiaries?.forEach((s) => subMap.set(s.id, s))

  const columns: ColumnDef<User, unknown>[] = [
    { accessorKey: 'name', header: '이름' },
    { accessorKey: 'email', header: '이메일' },
    {
      accessorKey: 'role',
      header: '역할',
      cell: ({ getValue }) => ROLE_LABELS[getValue() as string] ?? getValue(),
    },
    {
      accessorKey: 'subsidiary_id',
      header: '계열사',
      cell: ({ getValue }) => {
        const id = getValue() as string | null
        return id ? subMap.get(id)?.name ?? '-' : '-'
      },
    },
    {
      accessorKey: 'department_id',
      header: '부서',
      cell: ({ getValue }) => {
        const id = getValue() as string | null
        return id ? deptMap.get(id)?.name ?? '-' : '-'
      },
    },
    { accessorKey: 'position', header: '직위' },
    { accessorKey: 'title', header: '직책' },
    {
      accessorKey: 'is_active',
      header: '상태',
      cell: ({ getValue }) =>
        getValue() ? (
          <Badge variant="default">활성</Badge>
        ) : (
          <Badge variant="secondary">비활성</Badge>
        ),
    },
  ]

  if (isLoading) return <LoadingState variant="table" />

  return (
    <div className="space-y-6">
      <PageHeader
        title="사용자 관리"
        description="사용자 계정, 계열사, 부서를 관리합니다"
        actions={
          isDirector ? (
            <Button onClick={() => setShowCreate(true)}>사용자 추가</Button>
          ) : undefined
        }
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1fr)_280px]">
        <DataTable
          columns={columns}
          data={users ?? []}
          emptyTitle="사용자가 없습니다"
          onRowClick={(user) => isDirector && setEditingUser(user)}
        />

        {isDirector && (
          <div className="space-y-4">
            <SubsidiaryPanel />
            <DepartmentPanel />
          </div>
        )}
      </div>

      {showCreate && (
        <UserFormDialog
          departments={departments ?? []}
          subsidiaries={subsidiaries ?? []}
          onClose={() => setShowCreate(false)}
        />
      )}

      {editingUser && (
        <UserFormDialog
          user={editingUser}
          departments={departments ?? []}
          subsidiaries={subsidiaries ?? []}
          onClose={() => setEditingUser(null)}
          onToggleActive={() => {
            updateUser.mutate(
              { id: editingUser.id, is_active: !editingUser.is_active },
              {
                onSuccess: () => {
                  toast.success(editingUser.is_active ? '비활성화됨' : '활성화됨')
                  setEditingUser(null)
                },
                onError: (err) => toast.error(formatError(err)),
              },
            )
          }}
        />
      )}
    </div>
  )
}
