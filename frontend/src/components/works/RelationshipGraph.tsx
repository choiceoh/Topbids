import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router'

import type { GraphEdge, GraphNode } from '@/hooks/useRelationshipGraph'

const NODE_W = 160
const NODE_H = 56
const PADDING = 80

interface NodePosition {
  id: string
  x: number
  y: number
}

function relationLabel(type: string): string {
  switch (type) {
    case 'one_to_one': return '1:1'
    case 'one_to_many': return '1:N'
    case 'many_to_many': return 'N:M'
    default: return type
  }
}

// Simple circular layout, then optional drag to reposition
function computeLayout(nodes: GraphNode[]): NodePosition[] {
  if (nodes.length === 0) return []
  if (nodes.length === 1) {
    return [{ id: nodes[0].id, x: 300, y: 200 }]
  }

  const cx = 350
  const cy = 280
  const radius = Math.max(180, nodes.length * 45)

  return nodes.map((node, i) => {
    const angle = (2 * Math.PI * i) / nodes.length - Math.PI / 2
    return {
      id: node.id,
      x: cx + radius * Math.cos(angle),
      y: cy + radius * Math.sin(angle),
    }
  })
}

function computeEdgePath(
  source: NodePosition,
  target: NodePosition,
): string {
  const sx = source.x + NODE_W / 2
  const sy = source.y + NODE_H / 2
  const tx = target.x + NODE_W / 2
  const ty = target.y + NODE_H / 2

  const dx = tx - sx
  const dy = ty - sy
  const dist = Math.sqrt(dx * dx + dy * dy)

  // Shorten line to stop at node border
  const ratio = NODE_W / 2 / dist
  const startX = sx + dx * ratio
  const startY = sy + dy * ratio
  const endX = tx - dx * ratio
  const endY = ty - dy * ratio

  // Curved path
  const midX = (startX + endX) / 2
  const midY = (startY + endY) / 2
  const perpX = -(endY - startY) * 0.15
  const perpY = (endX - startX) * 0.15

  return `M${startX},${startY} Q${midX + perpX},${midY + perpY} ${endX},${endY}`
}

interface Props {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export default function RelationshipGraph({ nodes, edges }: Props) {
  const navigate = useNavigate()
  const svgRef = useRef<SVGSVGElement>(null)
  const [positions, setPositions] = useState<NodePosition[]>([])
  const [hoveredNode, setHoveredNode] = useState<string | null>(null)
  const [dragState, setDragState] = useState<{
    nodeId: string
    startX: number
    startY: number
    origX: number
    origY: number
  } | null>(null)

  // Compute initial layout
  useEffect(() => {
    setPositions(computeLayout(nodes))
  }, [nodes])

  const posMap = useMemo(
    () => new Map(positions.map((p) => [p.id, p])),
    [positions],
  )

  // Compute SVG viewBox to fit all nodes
  const viewBox = useMemo(() => {
    if (positions.length === 0) return `0 0 800 600`
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    for (const p of positions) {
      if (p.x < minX) minX = p.x
      if (p.y < minY) minY = p.y
      if (p.x + NODE_W > maxX) maxX = p.x + NODE_W
      if (p.y + NODE_H > maxY) maxY = p.y + NODE_H
    }
    return `${minX - PADDING} ${minY - PADDING} ${maxX - minX + PADDING * 2} ${maxY - minY + PADDING * 2}`
  }, [positions])

  // Connected edges for highlighting
  const connectedEdges = useMemo(() => {
    if (!hoveredNode) return new Set<string>()
    return new Set(
      edges
        .filter((e) => e.sourceId === hoveredNode || e.targetId === hoveredNode)
        .map((e) => e.id),
    )
  }, [hoveredNode, edges])

  const connectedNodes = useMemo(() => {
    if (!hoveredNode) return new Set<string>()
    const s = new Set<string>()
    s.add(hoveredNode)
    for (const e of edges) {
      if (e.sourceId === hoveredNode) s.add(e.targetId)
      if (e.targetId === hoveredNode) s.add(e.sourceId)
    }
    return s
  }, [hoveredNode, edges])

  // Drag handling
  const handleMouseDown = useCallback(
    (e: React.MouseEvent, nodeId: string) => {
      e.preventDefault()
      const pos = posMap.get(nodeId)
      if (!pos) return
      setDragState({
        nodeId,
        startX: e.clientX,
        startY: e.clientY,
        origX: pos.x,
        origY: pos.y,
      })
    },
    [posMap],
  )

  useEffect(() => {
    if (!dragState) return
    function handleMouseMove(e: MouseEvent) {
      if (!dragState) return
      const dx = e.clientX - dragState.startX
      const dy = e.clientY - dragState.startY
      setPositions((prev) =>
        prev.map((p) =>
          p.id === dragState.nodeId
            ? { ...p, x: dragState.origX + dx, y: dragState.origY + dy }
            : p,
        ),
      )
    }
    function handleMouseUp() {
      setDragState(null)
    }
    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', handleMouseUp)
    return () => {
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', handleMouseUp)
    }
  }, [dragState])

  if (nodes.length === 0) {
    return null
  }

  return (
    <svg
      ref={svgRef}
      viewBox={viewBox}
      className="h-full w-full"
      style={{ minHeight: 500 }}
    >
      <defs>
        <marker
          id="rel-arrow"
          markerWidth="8"
          markerHeight="8"
          refX="8"
          refY="4"
          orient="auto"
        >
          <path
            d="M0,1 L8,4 L0,7 Z"
            className="fill-muted-foreground/60"
          />
        </marker>
      </defs>

      {/* Edges */}
      {edges.map((edge) => {
        const source = posMap.get(edge.sourceId)
        const target = posMap.get(edge.targetId)
        if (!source || !target) return null

        const path = computeEdgePath(source, target)
        const isHighlighted = hoveredNode
          ? connectedEdges.has(edge.id)
          : true
        const dimmed = hoveredNode && !isHighlighted

        // Label position (midpoint of the curve)
        const sx = source.x + NODE_W / 2
        const sy = source.y + NODE_H / 2
        const tx = target.x + NODE_W / 2
        const ty = target.y + NODE_H / 2
        const midX = (sx + tx) / 2 + (-(ty - sy) * 0.15)
        const midY = (sy + ty) / 2 + ((tx - sx) * 0.15)

        return (
          <g key={edge.id}>
            <path
              d={path}
              fill="none"
              strokeWidth={isHighlighted && hoveredNode ? 2 : 1.5}
              className={dimmed ? 'stroke-muted-foreground/15' : 'stroke-muted-foreground/50'}
              markerEnd="url(#rel-arrow)"
            />
            <g
              className={dimmed ? 'opacity-20' : ''}
              style={{ transition: 'opacity 150ms' }}
            >
              <rect
                x={midX - 28}
                y={midY - 10}
                width={56}
                height={20}
                rx={4}
                className="fill-background stroke-border"
                strokeWidth={0.5}
              />
              <text
                x={midX}
                y={midY + 4}
                textAnchor="middle"
                className="fill-muted-foreground text-[10px]"
              >
                {edge.label} ({relationLabel(edge.relationType)})
              </text>
            </g>
          </g>
        )
      })}

      {/* Nodes */}
      {positions.map((pos) => {
        const node = nodes.find((n) => n.id === pos.id)
        if (!node) return null
        const isHighlighted = hoveredNode ? connectedNodes.has(node.id) : true
        const dimmed = hoveredNode && !isHighlighted

        return (
          <g
            key={node.id}
            transform={`translate(${pos.x}, ${pos.y})`}
            className={`cursor-grab ${dimmed ? 'opacity-20' : ''}`}
            style={{ transition: 'opacity 150ms' }}
            onMouseEnter={() => setHoveredNode(node.id)}
            onMouseLeave={() => setHoveredNode(null)}
            onMouseDown={(e) => handleMouseDown(e, node.id)}
            onDoubleClick={() => navigate(`/apps/${node.id}`)}
          >
            <rect
              width={NODE_W}
              height={NODE_H}
              rx={8}
              className={`fill-card stroke-border ${
                hoveredNode === node.id ? 'stroke-primary stroke-[1.5]' : ''
              }`}
              strokeWidth={hoveredNode === node.id ? 1.5 : 1}
            />
            {node.icon && (
              <text
                x={14}
                y={NODE_H / 2 + 5}
                className="text-base"
              >
                {node.icon}
              </text>
            )}
            <text
              x={node.icon ? 34 : 14}
              y={NODE_H / 2 - 3}
              className="fill-foreground text-xs font-medium"
            >
              {node.label.length > 14 ? node.label.slice(0, 14) + '...' : node.label}
            </text>
            <text
              x={node.icon ? 34 : 14}
              y={NODE_H / 2 + 12}
              className="fill-muted-foreground text-[10px]"
            >
              {node.fieldCount}개 필드
            </text>
          </g>
        )
      })}
    </svg>
  )
}
