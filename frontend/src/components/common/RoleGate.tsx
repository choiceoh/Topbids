import type { ReactNode } from 'react'

import { hasRole, useCurrentUser } from '@/hooks/useAuth'
import type { User } from '@/lib/types'

interface Props {
  roles: User['role'][]
  children: ReactNode
  fallback?: ReactNode
}

// RoleGate hides its children when the current user is not in `roles`.
// Use this for buttons/links that should never be visible to lower roles.
//
// Note: this is *display* gating, not security. The backend enforces RBAC.
export default function RoleGate({ roles, children, fallback = null }: Props) {
  const { data: user } = useCurrentUser()
  if (!hasRole(user, roles)) return <>{fallback}</>
  return <>{children}</>
}
