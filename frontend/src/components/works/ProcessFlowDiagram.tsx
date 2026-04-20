import { useCallback, useMemo, useRef, useState } from 'react'
import { Plus, Trash2, X } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'

// --- Types ---

export interface StatusDraft {
  name: string
  color: string
  sort_order: number
  is_initial: boolean
}

export interface TransitionDraft {
  from_index: number
  to_index: number
  label: string
  allowed_roles: string[]
  allowed_user_ids: string[]
}

interface Props {
  statuses: StatusDraft[]
  transitions: TransitionDraft[]
  users?: { id: string; name: string }[]
  onAddTransition: (from: number, to: number) => void
  onRemoveTransition: (index: number) => void
  onUpdateTransition: (index: number, patch: Partial<TransitionDraft>) => void
}

const ALL_ROLES = [
  { value: 'director', label: '관리자' },
  { value: 'pm', label: '운영자' },
  { value: 'engineer', label: '담당자' },
  { value: 'viewer', label: '열람자' },
] as const

const ROLE_LABELS: Record<string, string> = {
  director: '관리자',
  pm: '운영자',
  engineer: '담당자',
  viewer: '열람자',
}

// --- Layout constants ---
const NODE_W = 120
const NODE_H = 40
const NODE_RX = 8
const GAP_X = 160
const START_R = 20
const PADDING = 50
const ARROW_SIZE = 7

// --- Component ---

export default function ProcessFlowDiagram({
  statuses,
  transitions,
  users = [],
  onAddTransition,
  onRemoveTransition,
  onUpdateTransition,
}: Props) {
  const svgRef = useRef<SVGSVGElement>(null)

  // Node positions (draggable)
  const [positions, setPositions] = useState<{ x: number; y: number }[]>([])
  const [dragging, setDragging] = useState<{ index: number; offsetX: number; offsetY: number } | null>(null)

  // Connection mode: click source node, then target node to create transition
  const [connectFrom, setConnectFrom] = useState<number | null>(null)
  const [mousePos, setMousePos] = useState<{ x: number; y: number } | null>(null)

  // Selected transition for editing
  const [selectedTransition, setSelectedTransition] = useState<number | null>(null)

  // Initialize positions when statuses change
  const [prevStatusCount, setPrevStatusCount] = useState(statuses.length)
  if (statuses.length !== prevStatusCount) {
    setPrevStatusCount(statuses.length)
    const centerY = PADDING + 80
    setPositions(
      statuses.map((_, i) => ({
        x: PADDING + START_R * 2 + GAP_X + i * (NODE_W + GAP_X),
        y: centerY,
      })),
    )
    setConnectFrom(null)
    setSelectedTransition(null)
  }

  // SVG dimensions
  const svgW = useMemo(() => {
    if (positions.length === 0) return 400
    const maxX = Math.max(...positions.map((p) => p.x))
    return Math.max(maxX + NODE_W + PADDING * 2, 400)
  }, [positions])

  const svgH = useMemo(() => {
    if (positions.length === 0) return 250
    const maxY = Math.max(...positions.map((p) => p.y))
    const minY = Math.min(...positions.map((p) => p.y))
    return Math.max(maxY + NODE_H + PADDING * 2, minY > 0 ? maxY - minY + NODE_H + PADDING * 4 : 250, 250)
  }, [positions])

  const startX = PADDING + START_R
  const startY = useMemo(() => {
    if (positions.length === 0) return PADDING + 80 + NODE_H / 2
    return positions[0]?.y + NODE_H / 2
  }, [positions])

  // --- Drag handlers ---
  const handleMouseDown = useCallback(
    (index: number, e: React.MouseEvent) => {
      if (connectFrom !== null) return // In connect mode, don't drag
      e.stopPropagation()
      const pos = positions[index]
      if (!pos) return
      const svg = svgRef.current
      if (!svg) return
      const pt = svg.createSVGPoint()
      pt.x = e.clientX
      pt.y = e.clientY
      const svgPt = pt.matrixTransform(svg.getScreenCTM()!.inverse())
      setDragging({ index, offsetX: svgPt.x - pos.x, offsetY: svgPt.y - pos.y })
    },
    [connectFrom, positions],
  )

  const handleMouseMove = useCallback(
    (e: React.MouseEvent) => {
      const svg = svgRef.current
      if (!svg) return
      const pt = svg.createSVGPoint()
      pt.x = e.clientX
      pt.y = e.clientY
      const svgPt = pt.matrixTransform(svg.getScreenCTM()!.inverse())

      if (dragging) {
        setPositions((prev) =>
          prev.map((p, i) =>
            i === dragging.index
              ? { x: Math.max(PADDING, svgPt.x - dragging.offsetX), y: Math.max(PADDING, svgPt.y - dragging.offsetY) }
              : p,
          ),
        )
      }

      if (connectFrom !== null) {
        setMousePos({ x: svgPt.x, y: svgPt.y })
      }
    },
    [dragging, connectFrom],
  )

  const handleMouseUp = useCallback(() => {
    setDragging(null)
  }, [])

  // --- Node click handler ---
  const handleNodeClick = useCallback(
    (index: number, e: React.MouseEvent) => {
      e.stopPropagation()
      if (dragging) return

      if (connectFrom === null) {
        // Start connecting
        setConnectFrom(index)
        setSelectedTransition(null)
      } else if (connectFrom === index) {
        // Cancel
        setConnectFrom(null)
        setMousePos(null)
      } else {
        // Create transition
        onAddTransition(connectFrom, index)
        setConnectFrom(null)
        setMousePos(null)
      }
    },
    [connectFrom, dragging, onAddTransition],
  )

  // --- Transition click handler ---
  const handleTransitionClick = useCallback(
    (index: number, e: React.MouseEvent) => {
      e.stopPropagation()
      setSelectedTransition((prev) => (prev === index ? null : index))
      setConnectFrom(null)
    },
    [],
  )

  // Cancel modes on background click
  const handleBgClick = useCallback(() => {
    setConnectFrom(null)
    setMousePos(null)
    setSelectedTransition(null)
  }, [])

  if (statuses.length === 0 || positions.length === 0) return null

  // --- Render helpers ---
  function arrowHead(ex: number, ey: number, angle: number) {
    const a1 = angle + Math.PI * 0.8
    const a2 = angle - Math.PI * 0.8
    return `M${ex + ARROW_SIZE * Math.cos(a1)},${ey + ARROW_SIZE * Math.sin(a1)} L${ex},${ey} L${ex + ARROW_SIZE * Math.cos(a2)},${ey + ARROW_SIZE * Math.sin(a2)}`
  }

  function getEdgePath(fromIdx: number, toIdx: number, transIdx: number) {
    const from = positions[fromIdx]
    const to = positions[toIdx]
    if (!from || !to) return { path: '', midX: 0, midY: 0, endAngle: 0, labelX: 0, labelY: 0 }

    const fcx = from.x + NODE_W / 2
    const fcy = from.y + NODE_H / 2
    const tcx = to.x + NODE_W / 2
    const tcy = to.y + NODE_H / 2

    if (fromIdx === toIdx) {
      // Self-loop
      const topY = from.y - 4
      const r = 16
      return {
        path: `M${fcx - 10},${topY} A${r},${r} 0 1,1 ${fcx + 10},${topY}`,
        midX: fcx,
        midY: topY - r - 8,
        endAngle: Math.PI / 3,
        labelX: fcx,
        labelY: topY - r - 14,
      }
    }

    // Determine if there are parallel edges (same pair, any direction)
    const parallelEdges = transitions.filter(
      (t) =>
        (t.from_index === fromIdx && t.to_index === toIdx) ||
        (t.from_index === toIdx && t.to_index === fromIdx),
    )
    const parallelCount = parallelEdges.length

    // Calculate curve offset for parallel edges
    const dx = tcx - fcx
    const dy = tcy - fcy
    const dist = Math.sqrt(dx * dx + dy * dy)
    const nx = -dy / dist // normal
    const ny = dx / dist

    let curveOffset = 0
    if (parallelCount > 1) {
      const idx = transitions
        .filter(
          (t) =>
            (t.from_index === fromIdx && t.to_index === toIdx) ||
            (t.from_index === toIdx && t.to_index === fromIdx),
        )
        .indexOf(transitions[transIdx])
      curveOffset = (idx - (parallelCount - 1) / 2) * 30
    } else {
      // If nodes are at similar Y, use straight line; otherwise use slight curve
      if (Math.abs(fcy - tcy) < 10) {
        curveOffset = fromIdx > toIdx ? -30 : 0
      }
    }

    // Edge from border of from-node to border of to-node
    const angle = Math.atan2(tcy - fcy, tcx - fcx)
    const fx = fcx + (NODE_W / 2) * Math.cos(angle)
    const fy = fcy + (NODE_H / 2) * Math.sin(angle)
    const tx = tcx - (NODE_W / 2 + 4) * Math.cos(angle)
    const ty = tcy - (NODE_H / 2 + 4) * Math.sin(angle)

    const midX = (fx + tx) / 2 + nx * curveOffset
    const midY = (fy + ty) / 2 + ny * curveOffset

    const path = `M${fx},${fy} Q${midX},${midY} ${tx},${ty}`
    const endAngle = Math.atan2(ty - midY, tx - midX)

    return {
      path,
      midX,
      midY,
      endAngle,
      labelX: midX,
      labelY: midY - 10,
    }
  }

  const selectedTrans = selectedTransition !== null ? transitions[selectedTransition] : null

  return (
    <div className="space-y-3">
      {/* Toolbar */}
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant={connectFrom !== null ? 'default' : 'outline'}
          size="sm"
          onClick={() => {
            if (connectFrom !== null) {
              setConnectFrom(null)
              setMousePos(null)
            } else {
              setConnectFrom(-1) // Activate mode, waiting for first click
              setSelectedTransition(null)
            }
          }}
        >
          <Plus className="mr-1 h-3.5 w-3.5" />
          {connectFrom !== null ? '연결 취소' : '전이 추가'}
        </Button>
        {connectFrom !== null && connectFrom >= 0 && (
          <span className="text-xs text-muted-foreground">
            <Badge variant="secondary">{statuses[connectFrom]?.name}</Badge> → 대상 노드를 클릭하세요
          </span>
        )}
        {connectFrom === -1 && (
          <span className="text-xs text-muted-foreground">출발 노드를 클릭하세요</span>
        )}
      </div>

      {/* SVG Diagram */}
      <div className="overflow-x-auto rounded-lg border bg-white">
        <svg
          ref={svgRef}
          width={svgW}
          height={svgH}
          className="block select-none"
          onMouseMove={handleMouseMove}
          onMouseUp={handleMouseUp}
          onMouseLeave={handleMouseUp}
          onClick={handleBgClick}
        >
          <defs>
            <marker id="ah-gray" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
              <path d="M1,1 L7,4 L1,7" fill="none" stroke="#9ca3af" strokeWidth="1.5" />
            </marker>
            <marker id="ah-blue" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
              <path d="M1,1 L7,4 L1,7" fill="none" stroke="#3b82f6" strokeWidth="1.5" />
            </marker>
          </defs>

          {/* START circle → first status */}
          <circle
            cx={startX}
            cy={startY}
            r={START_R}
            fill="none"
            stroke="#1f2937"
            strokeWidth="2"
          />
          <text
            x={startX}
            y={startY + 4}
            textAnchor="middle"
            fontSize="10"
            fontWeight="bold"
            fill="#1f2937"
          >
            START
          </text>
          {positions.length > 0 && (
            <line
              x1={startX + START_R}
              y1={startY}
              x2={positions[0].x - 2}
              y2={positions[0].y + NODE_H / 2}
              stroke="#9ca3af"
              strokeWidth="1.5"
              markerEnd="url(#ah-gray)"
            />
          )}

          {/* Transition edges */}
          {transitions.map((t, i) => {
            const { path, endAngle, labelX, labelY } = getEdgePath(t.from_index, t.to_index, i)
            if (!path) return null
            const isSelected = selectedTransition === i
            const isBackward = t.from_index > t.to_index
            const strokeColor = isSelected ? '#3b82f6' : isBackward ? '#ef4444' : '#9ca3af'
            const strokeWidth = isSelected ? 2.5 : 1.5
            const roleStr =
              t.allowed_roles.length > 0
                ? t.allowed_roles.map((r) => ROLE_LABELS[r] ?? r).join(', ')
                : ''

            return (
              <g key={`t-${i}`}>
                {/* Invisible wider path for easier clicking */}
                <path
                  d={path}
                  fill="none"
                  stroke="transparent"
                  strokeWidth="16"
                  className="cursor-pointer"
                  onClick={(e) => handleTransitionClick(i, e)}
                />
                <path
                  d={path}
                  fill="none"
                  stroke={strokeColor}
                  strokeWidth={strokeWidth}
                  strokeDasharray={t.from_index === t.to_index || isBackward ? '5,3' : undefined}
                  className="pointer-events-none"
                />
                {t.from_index !== t.to_index && (
                  <path
                    d={arrowHead(
                      // Compute end point from path
                      positions[t.to_index]
                        ? positions[t.to_index].x +
                            NODE_W / 2 -
                            (NODE_W / 2 + 4) *
                              Math.cos(
                                Math.atan2(
                                  positions[t.to_index].y - positions[t.from_index].y,
                                  positions[t.to_index].x - positions[t.from_index].x,
                                ),
                              )
                        : 0,
                      positions[t.to_index]
                        ? positions[t.to_index].y +
                            NODE_H / 2 -
                            (NODE_H / 2 + 4) *
                              Math.sin(
                                Math.atan2(
                                  positions[t.to_index].y - positions[t.from_index].y,
                                  positions[t.to_index].x - positions[t.from_index].x,
                                ),
                              )
                        : 0,
                      endAngle,
                    )}
                    fill="none"
                    stroke={strokeColor}
                    strokeWidth={strokeWidth}
                    className="pointer-events-none"
                  />
                )}
                {/* Label */}
                {t.label && (
                  <text
                    x={labelX}
                    y={labelY}
                    textAnchor="middle"
                    fontSize="10"
                    fontWeight={isSelected ? '600' : '400'}
                    fill={isSelected ? '#3b82f6' : '#374151'}
                    className="pointer-events-none"
                  >
                    {t.label}
                  </text>
                )}
                {/* Role label */}
                {roleStr && (
                  <text
                    x={labelX}
                    y={labelY - 13}
                    textAnchor="middle"
                    fontSize="9"
                    fill="#9ca3af"
                    className="pointer-events-none"
                  >
                    {roleStr}
                  </text>
                )}
              </g>
            )
          })}

          {/* Connecting line (while creating a new transition) */}
          {connectFrom !== null && connectFrom >= 0 && mousePos && positions[connectFrom] && (
            <line
              x1={positions[connectFrom].x + NODE_W / 2}
              y1={positions[connectFrom].y + NODE_H / 2}
              x2={mousePos.x}
              y2={mousePos.y}
              stroke="#3b82f6"
              strokeWidth="2"
              strokeDasharray="6,3"
              className="pointer-events-none"
            />
          )}

          {/* Status nodes */}
          {statuses.map((s, i) => {
            const pos = positions[i]
            if (!pos) return null
            const isSource = connectFrom === i
            const isConnecting = connectFrom !== null && connectFrom >= 0 && connectFrom !== i
            const isDraggable = connectFrom === null

            return (
              <g
                key={i}
                className={isDraggable ? 'cursor-grab' : 'cursor-pointer'}
                onMouseDown={(e) => handleMouseDown(i, e)}
                onClick={(e) => {
                  if (connectFrom === -1) {
                    // First click in connect mode — set source
                    e.stopPropagation()
                    setConnectFrom(i)
                  } else {
                    handleNodeClick(i, e)
                  }
                }}
              >
                {/* Hover/selection highlight */}
                <rect
                  x={pos.x - 3}
                  y={pos.y - 3}
                  width={NODE_W + 6}
                  height={NODE_H + 6}
                  rx={NODE_RX + 2}
                  fill="none"
                  stroke={isSource ? '#3b82f6' : isConnecting ? '#3b82f6' : 'transparent'}
                  strokeWidth="2"
                  strokeDasharray={isConnecting ? '4,2' : undefined}
                  opacity={isConnecting ? 0.5 : 1}
                />
                {/* Node */}
                <rect
                  x={pos.x}
                  y={pos.y}
                  width={NODE_W}
                  height={NODE_H}
                  rx={NODE_RX}
                  fill={s.color}
                  className="transition-opacity hover:opacity-90"
                />
                <text
                  x={pos.x + NODE_W / 2}
                  y={pos.y + NODE_H / 2 + 5}
                  textAnchor="middle"
                  fontSize="12"
                  fontWeight="500"
                  fill="white"
                  className="pointer-events-none"
                >
                  {s.name}
                </text>
                {s.is_initial && (
                  <text
                    x={pos.x + NODE_W / 2}
                    y={pos.y + NODE_H + 14}
                    textAnchor="middle"
                    fontSize="9"
                    fill="#6b7280"
                    className="pointer-events-none"
                  >
                    초기
                  </text>
                )}
              </g>
            )
          })}
        </svg>
      </div>

      {/* Transition editor panel */}
      {selectedTrans && selectedTransition !== null && (
        <div className="rounded-lg border bg-muted/30 p-3 space-y-2">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-sm font-medium">
              <Badge style={{ backgroundColor: statuses[selectedTrans.from_index]?.color }} className="text-white">
                {statuses[selectedTrans.from_index]?.name}
              </Badge>
              <span className="text-muted-foreground">→</span>
              <Badge style={{ backgroundColor: statuses[selectedTrans.to_index]?.color }} className="text-white">
                {statuses[selectedTrans.to_index]?.name}
              </Badge>
            </div>
            <div className="flex items-center gap-1">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => {
                  onRemoveTransition(selectedTransition)
                  setSelectedTransition(null)
                }}
              >
                <Trash2 className="h-3.5 w-3.5 text-destructive" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setSelectedTransition(null)}
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <label className="text-xs text-muted-foreground whitespace-nowrap">라벨:</label>
            <Input
              placeholder="이동 이름 (예: 진행하기)"
              value={selectedTrans.label}
              onChange={(e) => onUpdateTransition(selectedTransition, { label: e.target.value })}
              className="h-8 text-sm"
            />
          </div>
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-xs text-muted-foreground whitespace-nowrap">허용 역할:</span>
            {ALL_ROLES.map((role) => (
              <label key={role.value} className="flex items-center gap-1 text-xs">
                <Checkbox
                  checked={selectedTrans.allowed_roles.includes(role.value)}
                  onCheckedChange={(checked) => {
                    const next = checked
                      ? [...selectedTrans.allowed_roles, role.value]
                      : selectedTrans.allowed_roles.filter((r) => r !== role.value)
                    onUpdateTransition(selectedTransition, { allowed_roles: next })
                  }}
                />
                {role.label}
              </label>
            ))}
          </div>
          {users.length > 0 && (
            <div className="flex items-center gap-2 flex-wrap">
              <span className="text-xs text-muted-foreground whitespace-nowrap">허용 사용자:</span>
              {users.map((u) => (
                <label key={u.id} className="flex items-center gap-1 text-xs">
                  <Checkbox
                    checked={selectedTrans.allowed_user_ids.includes(u.id)}
                    onCheckedChange={(checked) => {
                      const next = checked
                        ? [...selectedTrans.allowed_user_ids, u.id]
                        : selectedTrans.allowed_user_ids.filter((id) => id !== u.id)
                      onUpdateTransition(selectedTransition, { allowed_user_ids: next })
                    }}
                  />
                  {u.name}
                </label>
              ))}
            </div>
          )}
          {selectedTrans.allowed_roles.length === 0 && selectedTrans.allowed_user_ids.length === 0 && (
            <p className="text-xs text-muted-foreground">역할과 사용자가 모두 비어있으면 누구나 전환 가능</p>
          )}
        </div>
      )}
    </div>
  )
}
