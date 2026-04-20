import { useCallback, useEffect, useRef } from 'react'
import { toast } from 'sonner'

interface UndoEntry {
  id: string
  toastId: string | number
  undo: () => void
  redo: () => void
  timer: ReturnType<typeof setTimeout>
}

const UNDO_DURATION = 5000

/**
 * Provides undo-able toast functionality with Cmd+Z / Cmd+Shift+Z support.
 * Call `push(message, undoFn, redoFn?)` to show a toast with an "되돌리기" action.
 * If the user clicks undo within 5s or presses Cmd+Z, `undoFn` is called.
 * Cmd+Shift+Z triggers redo on the most recently undone action.
 */
export function useUndoToast() {
  const stack = useRef<UndoEntry[]>([])
  const redoStack = useRef<Array<{ redo: () => void }>>([])

  const performUndo = useCallback(() => {
    const entry = stack.current.pop()
    if (!entry) return
    clearTimeout(entry.timer)
    toast.dismiss(entry.toastId)
    entry.undo()
    redoStack.current.push({ redo: entry.redo })
    toast('되돌렸습니다', { duration: 2000 })
  }, [])

  const performRedo = useCallback(() => {
    const entry = redoStack.current.pop()
    if (!entry) return
    entry.redo()
    toast('다시 실행했습니다', { duration: 2000 })
  }, [])

  // Global Cmd+Z / Cmd+Shift+Z listener
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      if (!(e.metaKey || e.ctrlKey)) return
      if (e.key.toLowerCase() !== 'z') return

      // Don't intercept when editing text in inputs
      const el = document.activeElement
      if (el) {
        const tag = el.tagName.toLowerCase()
        if (tag === 'input' || tag === 'textarea') return
        if ((el as HTMLElement).contentEditable === 'true') return
      }

      e.preventDefault()
      if (e.shiftKey) {
        performRedo()
      } else {
        performUndo()
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [performUndo, performRedo])

  const push = useCallback((message: string, undoFn: () => void, redoFn?: () => void) => {
    const id = `undo_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`

    // Clear redo stack on new action
    redoStack.current = []

    const toastId = toast(message, {
      duration: UNDO_DURATION,
      action: {
        label: '되돌리기',
        onClick: () => {
          const idx = stack.current.findIndex((e) => e.id === id)
          if (idx !== -1) {
            const entry = stack.current.splice(idx, 1)[0]
            clearTimeout(entry.timer)
            entry.undo()
            redoStack.current.push({ redo: entry.redo })
          }
        },
      },
    })

    const timer = setTimeout(() => {
      const idx = stack.current.findIndex((e) => e.id === id)
      if (idx !== -1) stack.current.splice(idx, 1)
    }, UNDO_DURATION + 500)

    // The redo function defaults to the reverse: re-apply the original mutation
    const entry: UndoEntry = {
      id,
      toastId,
      undo: undoFn,
      redo: redoFn ?? undoFn,
      timer,
    }
    stack.current.push(entry)
  }, [])

  return { push }
}
