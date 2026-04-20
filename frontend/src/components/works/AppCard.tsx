import {
  Clipboard, FileText, Wrench, Calendar, BarChart3, CheckSquare,
  Users, ShoppingCart, Mail, Building2, FolderOpen, Briefcase,
  BookOpen, Globe, Heart, Star, Zap, Shield, Bell, Tag,
  Layers, Package, Truck, CreditCard, Settings, Database,
} from 'lucide-react'
import { Link } from 'react-router'

import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { TERM } from '@/lib/constants'
import type { Collection } from '@/lib/types'

export const APP_ICONS: Record<string, React.ComponentType<{ className?: string }>> = {
  clipboard: Clipboard,
  document: FileText,
  tool: Wrench,
  calendar: Calendar,
  chart: BarChart3,
  check: CheckSquare,
  users: Users,
  cart: ShoppingCart,
  mail: Mail,
  building: Building2,
  folder: FolderOpen,
  briefcase: Briefcase,
  book: BookOpen,
  globe: Globe,
  heart: Heart,
  star: Star,
  zap: Zap,
  shield: Shield,
  bell: Bell,
  tag: Tag,
  layers: Layers,
  package: Package,
  truck: Truck,
  card: CreditCard,
  settings: Settings,
  database: Database,
}

export const DEFAULT_ICON = 'clipboard'

export default function AppCard({ collection, count }: { collection: Collection, count?: number }) {

  // How long ago the collection was last updated.
  const updatedAt = collection.updated_at ? new Date(collection.updated_at) : null
  const timeSince = updatedAt ? formatTimeSince(updatedAt) : null

  return (
    <Link to={`/apps/${collection.id}`}>
      <Card className="group flex h-full flex-col p-4 shadow-premium transition-all duration-300 ease-out hover:-translate-y-1 hover:shadow-premium-hover">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-accent text-muted-foreground transition-colors duration-300 group-hover:bg-foreground group-hover:text-white">
              <AppIcon name={collection.icon} className="h-4.5 w-4.5" />
            </div>
            <h3 className="font-semibold tracking-tight text-foreground">{collection.label}</h3>
          </div>
          <div className="flex items-center gap-1.5">
            {collection.is_system && <Badge variant="secondary">시스템</Badge>}
          </div>
        </div>
        <p className="mt-2.5 min-h-[2lh] flex-1 line-clamp-2 break-words text-sm leading-relaxed text-muted-foreground">{collection.description || '\u00A0'}</p>
        <div className="mt-3.5 flex items-center gap-3 text-xs text-muted-foreground/80">
          <span>{collection.fields?.length || 0}개 {TERM.field}</span>
          {count != null && (
            <>
              <span className="h-0.5 w-0.5 rounded-full bg-current opacity-40" />
              <span>{count.toLocaleString('ko')}건 {TERM.record}</span>
            </>
          )}
          {timeSince && (
            <>
              <span className="h-0.5 w-0.5 rounded-full bg-current opacity-40" />
              <span>최근 {timeSince}</span>
            </>
          )}
        </div>
      </Card>
    </Link>
  )
}

export function AppIcon({ name, className }: { name?: string, className?: string }) {
  const Icon = APP_ICONS[name || DEFAULT_ICON] || APP_ICONS[DEFAULT_ICON]
  return <Icon className={className} />
}

function formatTimeSince(date: Date): string {
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMin = Math.floor(diffMs / 60_000)
  if (diffMin < 1) return '방금'
  if (diffMin < 60) return `${diffMin}분 전`
  const diffHrs = Math.floor(diffMin / 60)
  if (diffHrs < 24) return `${diffHrs}시간 전`
  const diffDays = Math.floor(diffHrs / 24)
  if (diffDays < 30) return `${diffDays}일 전`
  return date.toLocaleDateString('ko')
}
