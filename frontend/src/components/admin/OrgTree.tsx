import { useState, useMemo } from 'react'
import { ChevronDown, ChevronRight, Building, Building2, User as UserIcon } from 'lucide-react'

import type { Department, Subsidiary, User } from '@/lib/types'

interface OrgTreeProps {
  subsidiaries?: Subsidiary[]
  departments: Department[]
  users: User[]
  onSelectUser?: (user: User) => void
  onSelectDepartment?: (dept: Department) => void
  onSelectSubsidiary?: (sub: Subsidiary) => void
  selectedUserId?: string
  selectedDeptId?: string
  selectedSubId?: string
  showUsers?: boolean
}

interface DeptNode {
  dept: Department
  children: DeptNode[]
  users: User[]
}

interface SubNode {
  sub: Subsidiary
  deptRoots: DeptNode[]
  userCount: number
}

export default function OrgTree({
  subsidiaries = [],
  departments,
  users,
  onSelectUser,
  onSelectDepartment,
  onSelectSubsidiary,
  selectedUserId,
  selectedDeptId,
  selectedSubId,
  showUsers = true,
}: OrgTreeProps) {
  const { subNodes, unassignedDepts, unassignedUsers } = useMemo(
    () => buildOrgTree(subsidiaries, departments, showUsers ? users : []),
    [subsidiaries, departments, users, showUsers],
  )

  return (
    <div className="space-y-0.5 text-sm">
      {subNodes.map((sn) => (
        <SubBranch
          key={sn.sub.id}
          node={sn}
          onSelectUser={onSelectUser}
          onSelectDepartment={onSelectDepartment}
          onSelectSubsidiary={onSelectSubsidiary}
          selectedUserId={selectedUserId}
          selectedDeptId={selectedDeptId}
          selectedSubId={selectedSubId}
        />
      ))}
      {unassignedDepts.length > 0 && (
        <div className="mt-2">
          <p className="px-2 py-1 text-xs font-medium text-muted-foreground">미배정 부서</p>
          {unassignedDepts.map((node) => (
            <DeptBranch
              key={node.dept.id}
              node={node}
              depth={1}
              onSelectUser={onSelectUser}
              onSelectDepartment={onSelectDepartment}
              selectedUserId={selectedUserId}
              selectedDeptId={selectedDeptId}
            />
          ))}
        </div>
      )}
      {unassignedUsers.length > 0 && (
        <div className="mt-2">
          <p className="px-2 py-1 text-xs font-medium text-muted-foreground">미배정</p>
          {unassignedUsers.map((u) => (
            <UserLeaf
              key={u.id}
              user={u}
              depth={1}
              selected={u.id === selectedUserId}
              onClick={onSelectUser}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function SubBranch({
  node,
  onSelectUser,
  onSelectDepartment,
  onSelectSubsidiary,
  selectedUserId,
  selectedDeptId,
  selectedSubId,
}: {
  node: SubNode
  onSelectUser?: (user: User) => void
  onSelectDepartment?: (dept: Department) => void
  onSelectSubsidiary?: (sub: Subsidiary) => void
  selectedUserId?: string
  selectedDeptId?: string
  selectedSubId?: string
}) {
  const [expanded, setExpanded] = useState(true)
  const hasChildren = node.deptRoots.length > 0
  const isSelected = node.sub.id === selectedSubId

  return (
    <div>
      <button
        type="button"
        className={`flex w-full items-center gap-1.5 rounded px-2 py-1 text-left hover:bg-stone-100 ${
          isSelected ? 'bg-stone-100 font-medium' : ''
        }`}
        style={{ paddingLeft: '8px' }}
        onClick={() => {
          if (onSelectSubsidiary) {
            onSelectSubsidiary(node.sub)
          }
          setExpanded(!expanded)
        }}
      >
        {hasChildren ? (
          expanded ? (
            <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          )
        ) : (
          <span className="w-3.5" />
        )}
        <Building className="h-3.5 w-3.5 shrink-0 text-stone-600" />
        <span className="truncate font-medium" title={node.sub.name}>{node.sub.name}</span>
        {node.userCount > 0 && (
          <span className="ml-auto text-xs text-muted-foreground">{node.userCount}</span>
        )}
      </button>
      {expanded && node.deptRoots.map((dn) => (
        <DeptBranch
          key={dn.dept.id}
          node={dn}
          depth={1}
          onSelectUser={onSelectUser}
          onSelectDepartment={onSelectDepartment}
          selectedUserId={selectedUserId}
          selectedDeptId={selectedDeptId}
        />
      ))}
    </div>
  )
}

function DeptBranch({
  node,
  depth,
  onSelectUser,
  onSelectDepartment,
  selectedUserId,
  selectedDeptId,
}: {
  node: DeptNode
  depth: number
  onSelectUser?: (user: User) => void
  onSelectDepartment?: (dept: Department) => void
  selectedUserId?: string
  selectedDeptId?: string
}) {
  const [expanded, setExpanded] = useState(depth < 2)
  const hasChildren = node.children.length > 0 || node.users.length > 0
  const isSelected = node.dept.id === selectedDeptId

  return (
    <div>
      <button
        type="button"
        className={`flex w-full items-center gap-1.5 rounded px-2 py-1 text-left hover:bg-stone-100 ${
          isSelected ? 'bg-stone-100 font-medium' : ''
        }`}
        style={{ paddingLeft: `${depth * 16 + 8}px` }}
        onClick={() => {
          if (onSelectDepartment) {
            onSelectDepartment(node.dept)
          } else {
            setExpanded(!expanded)
          }
        }}
      >
        {hasChildren ? (
          expanded ? (
            <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          )
        ) : (
          <span className="w-3.5" />
        )}
        <Building2 className="h-3.5 w-3.5 shrink-0 text-stone-500" />
        <span className="truncate" title={node.dept.name}>{node.dept.name}</span>
        {node.users.length > 0 && (
          <span className="ml-auto text-xs text-muted-foreground">{node.users.length}</span>
        )}
      </button>
      {expanded && (
        <>
          {node.children.map((child) => (
            <DeptBranch
              key={child.dept.id}
              node={child}
              depth={depth + 1}
              onSelectUser={onSelectUser}
              onSelectDepartment={onSelectDepartment}
              selectedUserId={selectedUserId}
              selectedDeptId={selectedDeptId}
            />
          ))}
          {node.users.map((u) => (
            <UserLeaf
              key={u.id}
              user={u}
              depth={depth + 1}
              selected={u.id === selectedUserId}
              onClick={onSelectUser}
            />
          ))}
        </>
      )}
    </div>
  )
}

function UserLeaf({
  user,
  depth,
  selected,
  onClick,
}: {
  user: User
  depth: number
  selected: boolean
  onClick?: (user: User) => void
}) {
  return (
    <button
      type="button"
      className={`flex w-full items-center gap-1.5 rounded px-2 py-1 text-left hover:bg-stone-100 ${
        selected ? 'bg-stone-100 font-medium' : ''
      } ${!user.is_active ? 'opacity-50' : ''}`}
      style={{ paddingLeft: `${depth * 16 + 8}px` }}
      onClick={() => onClick?.(user)}
      disabled={!onClick}
    >
      <span className="w-3.5" />
      <UserIcon className="h-3.5 w-3.5 shrink-0 text-stone-400" />
      <span className="truncate" title={user.name}>{user.name}</span>
      {user.position && (
        <span className="ml-1 text-xs text-muted-foreground">{user.position}</span>
      )}
    </button>
  )
}

function buildOrgTree(
  subsidiaries: Subsidiary[],
  departments: Department[],
  users: User[],
): { subNodes: SubNode[]; unassignedDepts: DeptNode[]; unassignedUsers: User[] } {
  // Build department nodes.
  const nodeMap = new Map<string, DeptNode>()
  for (const d of departments) {
    nodeMap.set(d.id, { dept: d, children: [], users: [] })
  }

  // Assign users to their department.
  for (const u of users) {
    if (u.department_id) {
      const node = nodeMap.get(u.department_id)
      if (node) node.users.push(u)
    }
  }

  // Build parent-child within departments.
  const deptRootsMap = new Map<string | null, DeptNode[]>() // subsidiary_id -> root dept nodes
  for (const node of nodeMap.values()) {
    if (node.dept.parent_id) {
      const parent = nodeMap.get(node.dept.parent_id)
      if (parent) {
        parent.children.push(node)
        continue
      }
    }
    // This is a root department — group by subsidiary_id.
    const subId = node.dept.subsidiary_id ?? null
    const arr = deptRootsMap.get(subId) ?? []
    arr.push(node)
    deptRootsMap.set(subId, arr)
  }

  const sortNodes = (nodes: DeptNode[]) => {
    nodes.sort((a, b) => a.dept.sort_order - b.dept.sort_order || a.dept.name.localeCompare(b.dept.name))
    for (const n of nodes) sortNodes(n.children)
  }

  // Count users recursively.
  const countUsers = (node: DeptNode): number => {
    let c = node.users.length
    for (const ch of node.children) c += countUsers(ch)
    return c
  }

  // Build subsidiary nodes.
  const subNodes: SubNode[] = []
  const activeSubs = subsidiaries.filter((s) => s.is_active)
  activeSubs.sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name))

  for (const sub of activeSubs) {
    const roots = deptRootsMap.get(sub.id) ?? []
    sortNodes(roots)
    let userCount = 0
    for (const r of roots) userCount += countUsers(r)
    subNodes.push({ sub, deptRoots: roots, userCount })
    deptRootsMap.delete(sub.id)
  }

  // Departments without subsidiary assignment.
  const unassignedDepts = deptRootsMap.get(null) ?? []
  sortNodes(unassignedDepts)
  // Also gather any remaining orphans (subsidiary deleted but dept still references it).
  for (const [key, nodes] of deptRootsMap) {
    if (key !== null) unassignedDepts.push(...nodes)
  }

  // Users without department assignment.
  const unassignedUsers = users.filter((u) => !u.department_id)

  return { subNodes, unassignedDepts, unassignedUsers }
}
