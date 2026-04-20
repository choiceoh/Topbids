import { useMutation } from '@tanstack/react-query'

import { api } from '@/lib/api'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  images?: string[] // data-URLs for multimodal messages
}

interface ChatRequest {
  message: string
  history: ChatMessage[]
  images?: string[]
}

interface ChatResponse {
  reply: string
}

const AI_TIMEOUT = 120_000

export function useAIChat() {
  return useMutation({
    mutationFn: (req: ChatRequest) =>
      api.post<ChatResponse>('/ai/chat', req, { timeout: AI_TIMEOUT }),
  })
}
