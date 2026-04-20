import { useMutation } from '@tanstack/react-query'

import { api } from '@/lib/api'

const AI_TIMEOUT = 120_000

export interface AIBuildField {
  slug: string
  label: string
  field_type: string
  is_required: boolean
  width: number
  height: number
  options?: Record<string, unknown>
}

export interface AIBuildQuestion {
  id: string
  question: string
  placeholder?: string
  choices?: string[]
}

export interface AIBuildSchema {
  slug: string
  label: string
  description: string
  icon?: string
  fields: AIBuildField[]
}

export interface AIBuildEnvelope {
  mode: 'questions' | 'schema'
  questions?: AIBuildQuestion[]
  schema?: AIBuildSchema
}

/** For backwards compat */
export type AIBuildResult = AIBuildSchema

interface AIBuildInput {
  description: string
  answers?: Record<string, string>
}

export function useAIBuildCollection() {
  return useMutation({
    mutationFn: (input: AIBuildInput) =>
      api.post<AIBuildEnvelope>('/ai/build-collection', input, { timeout: AI_TIMEOUT }),
  })
}

export function useAIGenerateSlug() {
  return useMutation({
    mutationFn: (label: string) =>
      api.post<{ slug: string }>('/ai/generate-slug', { label }, { timeout: AI_TIMEOUT }),
  })
}

export function useAIBuildFormula(slug: string | undefined) {
  return useMutation({
    mutationFn: (description: string) =>
      api.post<{ expression: string }>(`/ai/build-formula/${slug}`, { description }, { timeout: AI_TIMEOUT }),
  })
}

export function useAIBuildFilter(slug: string | undefined) {
  return useMutation({
    mutationFn: (query: string) =>
      api.post<{ field: string, operator: string, value: string }[]>(`/ai/build-filter/${slug}`, { query }, { timeout: AI_TIMEOUT }),
  })
}

export function useAIPrefill(slug: string | undefined) {
  return useMutation({
    mutationFn: (description: string) =>
      api.post<Record<string, unknown>>(`/ai/prefill/${slug}`, { description }, { timeout: AI_TIMEOUT }),
  })
}

export function useAIMapCSVColumns(slug: string | undefined) {
  return useMutation({
    mutationFn: (headers: string[]) =>
      api.post<Record<string, string>>(`/ai/map-csv-columns/${slug}`, { headers }, { timeout: AI_TIMEOUT }),
  })
}

export interface AIChartResult {
  name: string
  chart_type: string
  config: Record<string, unknown>
}

export function useAIBuildChart(collectionId: string | undefined) {
  return useMutation({
    mutationFn: (description: string) =>
      api.post<AIChartResult>(`/ai/build-chart/${collectionId}`, { description }, { timeout: AI_TIMEOUT }),
  })
}
