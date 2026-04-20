import { useCallback, useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'

interface Step {
  title: string
  description: string
  target: string // CSS selector for the target element
}

interface Props {
  storageKey: string
  steps: Step[]
}

export default function CoachMark({ storageKey, steps }: Props) {
  const [current, setCurrent] = useState(-1)
  const [rect, setRect] = useState<DOMRect | null>(null)

  useEffect(() => {
    try {
      if (localStorage.getItem(storageKey)) return
    } catch { /* ignore */ }
    const timer = setTimeout(() => setCurrent(0), 600)
    return () => clearTimeout(timer)
  }, [storageKey])

  const updateRect = useCallback(() => {
    if (current < 0 || current >= steps.length) return
    const el = document.querySelector(steps[current].target)
    if (el) setRect(el.getBoundingClientRect())
  }, [current, steps])

  useEffect(() => {
    // Defer initial measurement to avoid synchronous setState in effect
    const frame = requestAnimationFrame(() => updateRect())
    window.addEventListener('resize', updateRect)
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('resize', updateRect)
    }
  }, [updateRect])

  function dismiss() {
    setCurrent(-1)
    try { localStorage.setItem(storageKey, '1') } catch { /* ignore */ }
  }

  function next() {
    if (current + 1 >= steps.length) {
      dismiss()
    } else {
      setCurrent(current + 1)
    }
  }

  if (current < 0 || !rect) return null

  const step = steps[current]
  const pad = 6

  return (
    <div className="fixed inset-0 z-50" onClick={dismiss}>
      {/* Backdrop with cutout */}
      <svg className="absolute inset-0 h-full w-full">
        <defs>
          <mask id="coach-mask">
            <rect width="100%" height="100%" fill="white" />
            <rect
              x={rect.left - pad}
              y={rect.top - pad}
              width={rect.width + pad * 2}
              height={rect.height + pad * 2}
              rx="8"
              fill="black"
            />
          </mask>
        </defs>
        <rect width="100%" height="100%" fill="rgba(0,0,0,0.4)" mask="url(#coach-mask)" />
      </svg>

      {/* Highlight border */}
      <div
        className="absolute rounded-lg ring-2 ring-primary ring-offset-2"
        style={{
          left: rect.left - pad,
          top: rect.top - pad,
          width: rect.width + pad * 2,
          height: rect.height + pad * 2,
          pointerEvents: 'none',
        }}
      />

      {/* Tooltip */}
      <div
        className="absolute animate-fade-in-up rounded-lg border bg-background p-4 shadow-lg"
        style={{
          left: Math.min(rect.right + 12, window.innerWidth - 300),
          top: rect.top,
          width: 260,
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <p className="text-sm font-semibold">{step.title}</p>
        <p className="mt-1 text-xs text-muted-foreground">{step.description}</p>
        <div className="mt-3 flex items-center justify-between">
          <span className="text-xs text-muted-foreground">
            {current + 1} / {steps.length}
          </span>
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={dismiss}>
              건너뛰기
            </Button>
            <Button size="sm" onClick={next}>
              {current + 1 < steps.length ? '다음' : '완료'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
