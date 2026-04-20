import EmptyState from '@/components/common/EmptyState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import OrgTree from '@/components/admin/OrgTree'
import { useDepartments } from '@/hooks/useDepartments'
import { useSubsidiaries } from '@/hooks/useSubsidiaries'
import { useUsers } from '@/hooks/useUsers'
import { ROLE_LABELS } from '@/lib/constants'
import type { Department, Subsidiary, User } from '@/lib/types'
import { useState } from 'react'

type Selection =
  | { kind: 'user'; user: User }
  | { kind: 'department'; dept: Department }
  | { kind: 'subsidiary'; sub: Subsidiary }
  | null

export default function OrgChartPage() {
  const { data: subsidiaries, isLoading: sLoading } = useSubsidiaries()
  const { data: departments, isLoading: dLoading } = useDepartments()
  const { data: users, isLoading: uLoading } = useUsers()
  const [selected, setSelected] = useState<Selection>(null)

  if (sLoading || dLoading || uLoading) return <LoadingState />

  return (
    <div className="space-y-6">
      <PageHeader title="조직도" description="계열사 · 부서 · 구성원 현황" />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[320px_1fr]">
        <div className="rounded-lg border border-stone-200 bg-white p-4">
          <OrgTree
            subsidiaries={subsidiaries ?? []}
            departments={departments ?? []}
            users={(users ?? []).filter((u) => u.is_active)}
            onSelectUser={(u) => setSelected({ kind: 'user', user: u })}
            onSelectDepartment={(d) => setSelected({ kind: 'department', dept: d })}
            onSelectSubsidiary={(s) => setSelected({ kind: 'subsidiary', sub: s })}
            selectedUserId={selected?.kind === 'user' ? selected.user.id : undefined}
            selectedDeptId={selected?.kind === 'department' ? selected.dept.id : undefined}
            selectedSubId={selected?.kind === 'subsidiary' ? selected.sub.id : undefined}
          />
        </div>
        <div className="rounded-lg border border-stone-200 bg-white p-6">
          {selected?.kind === 'user' && (
            <UserDetail user={selected.user} departments={departments ?? []} subsidiaries={subsidiaries ?? []} />
          )}
          {selected?.kind === 'department' && (
            <DepartmentDetail dept={selected.dept} subsidiaries={subsidiaries ?? []} users={users ?? []} />
          )}
          {selected?.kind === 'subsidiary' && (
            <SubsidiaryDetail sub={selected.sub} departments={departments ?? []} users={users ?? []} />
          )}
          {!selected && (
            <EmptyState
              compact
              title="좌측 트리에서 항목을 선택하세요"
              description="계열사, 부서 또는 구성원을 선택하면 상세 정보가 표시됩니다"
            />
          )}
        </div>
      </div>
    </div>
  )
}

function UserDetail({ user, departments, subsidiaries }: { user: User; departments: Department[]; subsidiaries: Subsidiary[] }) {
  const dept = departments.find((d) => d.id === user.department_id)
  const sub = subsidiaries.find((s) => s.id === user.subsidiary_id)
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <div className="flex h-12 w-12 items-center justify-center rounded-full bg-stone-100 text-lg font-bold text-stone-600">
          {user.name.charAt(0)}
        </div>
        <div>
          <p className="text-lg font-semibold">{user.name}</p>
          <p className="text-sm text-muted-foreground">
            {ROLE_LABELS[user.role] ?? user.role}
            {user.position && ` / ${user.position}`}
          </p>
        </div>
      </div>
      <div className="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
        <Info label="이메일" value={user.email} />
        <Info label="전화번호" value={user.phone} />
        <Info label="직책" value={user.title} />
        <Info label="입사일" value={user.joined_at ?? undefined} />
        <Info label="계열사" value={sub?.name} />
        <Info label="부서" value={dept?.name} />
      </div>
    </div>
  )
}

function DepartmentDetail({ dept, subsidiaries, users }: { dept: Department; subsidiaries: Subsidiary[]; users: User[] }) {
  const sub = subsidiaries.find((s) => s.id === dept.subsidiary_id)
  const memberCount = users.filter((u) => u.department_id === dept.id && u.is_active).length
  return (
    <div className="space-y-4">
      <p className="text-lg font-semibold">{dept.name}</p>
      <div className="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
        <Info label="소속 계열사" value={sub?.name ?? '미배정'} />
        <Info label="소속 인원" value={`${memberCount}명`} />
      </div>
    </div>
  )
}

function SubsidiaryDetail({ sub, departments, users }: { sub: Subsidiary; departments: Department[]; users: User[] }) {
  const deptCount = departments.filter((d) => d.subsidiary_id === sub.id).length
  const memberCount = users.filter((u) => u.subsidiary_id === sub.id && u.is_active).length
  return (
    <div className="space-y-4">
      <p className="text-lg font-semibold">{sub.name}</p>
      <div className="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
        <Info label="부서 수" value={`${deptCount}개`} />
        <Info label="소속 인원" value={`${memberCount}명`} />
      </div>
    </div>
  )
}

function Info({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <div>
      <span className="text-muted-foreground">{label}</span>
      <p className="font-medium">{value}</p>
    </div>
  )
}
