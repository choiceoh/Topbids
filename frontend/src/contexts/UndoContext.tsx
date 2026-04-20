import { createContext, useCallback, useContext, useEffect, useRef } from 'react'
import { toast } from 'sonner'

interface UndoEntry {
  id: string
  description: string
  undo: () => void
}

interface UndoContextValue {
  push: (description: string, undoFn: () => void) => void
}

const UndoCtx = createContext<UndoContextValue | null>(null)

const MAX_STACK = 20
const UNDO_DURATION = 5000

export function UndoProvider({ children }: { children: React.ReactNode }) {
  const stack = useRef<UndoEntry[]>([])
  const timers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())

  const executeUndo = useCallback((id: string) => {
    const entry = stack.current.find((e) => e.id === id)
    if (!entry) return
    stack.current = stack.current.filter((e) => e.id !== id)
    const timer = timers.current.get(id)
    if (timer) {
      clearTimeout(timer)
      timers.current.delete(id)
    }
    entry.undo()
    toast.success('되돌렸습니다')
  }, [])

  const push = useCallback((description: string, undoFn: () => void) => {
    const id = `undo_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`
    const entry: UndoEntry = { id, description, undo: undoFn }

    stack.current = [entry, ...stack.current].slice(0, MAX_STACK)

    toast(description, {
      duration: UNDO_DURATION,
      action: {
        label: '되돌리기',
        onClick: () => {
          executeUndo(id)
        },
      },
    })

    // Auto-expire from stack
    const timer = setTimeout(() => {
      stack.current = stack.current.filter((e) => e.id !== id)
      timers.current.delete(id)
    }, UNDO_DURATION + 500)

    timers.current.set(id, timer)
  }, [executeUndo])

  // Global Ctrl+Z handler: undo the most recent action
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key === 'z' && !e.shiftKey) {
        // Don't intercept when user is typing in an input/textarea
        const target = e.target as HTMLElement
        if (
          target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.isContentEditable
        ) {
          return
        }

        if (stack.current.length > 0) {
          e.preventDefault()
          const latest = stack.current[0]
          executeUndo(latest.id)
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [])

  return (
    <UndoCtx.Provider value={{ push }}>
      {children}
    </UndoCtx.Provider>
  )
}

export function useUndo() {
  const ctx = useContext(UndoCtx)
  if (!ctx) throw new Error('useUndo must be used within UndoProvider')
  return ctx
}
