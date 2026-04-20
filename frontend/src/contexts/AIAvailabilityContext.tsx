import { createContext, useContext, type ReactNode } from 'react'

import { useAIHealth } from '@/hooks/useAIHealth'

const AIAvailabilityContext = createContext(false)

export function AIAvailabilityProvider({ children }: { children: ReactNode }) {
  const { data } = useAIHealth()
  const available = data?.available ?? false

  return (
    <AIAvailabilityContext.Provider value={available}>
      {children}
    </AIAvailabilityContext.Provider>
  )
}

export function useAIAvailable() {
  return useContext(AIAvailabilityContext)
}
