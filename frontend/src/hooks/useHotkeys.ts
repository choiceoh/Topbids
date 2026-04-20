import { useEffect } from 'react'

type KeyCombo = string // e.g. 'mod+s', 'mod+n', '?', 'escape'

interface HotkeyConfig {
  key: KeyCombo
  handler: (e: KeyboardEvent) => void
  enabled?: boolean
}

function matchesCombo(e: KeyboardEvent, combo: string): boolean {
  const parts = combo.toLowerCase().split('+')
  const key = parts.pop()!
  const needsMod = parts.includes('mod')
  const needsShift = parts.includes('shift')
  const needsAlt = parts.includes('alt')

  const hasMod = e.metaKey || e.ctrlKey
  const hasShift = e.shiftKey
  const hasAlt = e.altKey

  if (needsMod !== hasMod) return false
  if (needsShift !== hasShift) return false
  if (needsAlt !== hasAlt) return false

  const eventKey = e.key.toLowerCase()
  if (key === 'escape') return eventKey === 'escape'
  return eventKey === key
}

function isInputFocused(): boolean {
  const el = document.activeElement
  if (!el) return false
  const tag = el.tagName.toLowerCase()
  if (tag === 'input' || tag === 'textarea' || tag === 'select') return true
  if ((el as HTMLElement).contentEditable === 'true') return true
  return false
}

export function useHotkeys(configs: HotkeyConfig[]) {
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      for (const config of configs) {
        if (config.enabled === false) continue
        if (matchesCombo(e, config.key)) {
          // Allow mod+key combos even in inputs, but block plain keys
          const hasMod = config.key.includes('mod') || config.key.includes('shift+') || config.key.includes('alt+')
          if (!hasMod && isInputFocused()) continue

          e.preventDefault()
          config.handler(e)
          return
        }
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [configs])
}
