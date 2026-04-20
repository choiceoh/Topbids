import { useRelationshipGraphAPI } from '@/hooks/useEntries'
import type { GraphNode, GraphEdge } from '@/hooks/useEntries'

export type { GraphNode, GraphEdge }

export function useRelationshipGraph() {
  const { data, isLoading } = useRelationshipGraphAPI()

  const nodes: GraphNode[] = data?.nodes ?? []
  const edges: GraphEdge[] = data?.edges ?? []

  return { nodes, edges, isLoading }
}
