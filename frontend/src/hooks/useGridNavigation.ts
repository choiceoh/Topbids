import { useCallback, useEffect, useRef, useState } from 'react'

export interface CellPosition {
  row: number
  col: number
}

export interface SelectionRange {
  startRow: number
  startCol: number
  endRow: number
  endCol: number
}

// Normalize selection range so start <= end.
function normalize(range: SelectionRange): SelectionRange {
  return {
    startRow: Math.min(range.startRow, range.endRow),
    startCol: Math.min(range.startCol, range.endCol),
    endRow: Math.max(range.startRow, range.endRow),
    endCol: Math.max(range.startCol, range.endCol),
  }
}

export function isCellInRange(row: number, col: number, range: SelectionRange | null): boolean {
  if (!range) return false
  const n = normalize(range)
  return row >= n.startRow && row <= n.endRow && col >= n.startCol && col <= n.endCol
}

interface UseGridNavigationOptions {
  rowCount: number
  colCount: number
  /** Column indices that should be skipped during navigation (e.g. _actions) */
  skipColumns?: number[]
}

export function useGridNavigation({
  rowCount,
  colCount,
  skipColumns = [],
}: UseGridNavigationOptions) {
  const [activeCell, setActiveCell] = useState<CellPosition | null>(null)
  const [selection, setSelection] = useState<SelectionRange | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Anchor for shift-extend selection.
  const anchorRef = useRef<CellPosition | null>(null)

  const clampRow = useCallback((r: number) => Math.max(0, Math.min(r, rowCount - 1)), [rowCount])
  const clampCol = useCallback((c: number) => Math.max(0, Math.min(c, colCount - 1)), [colCount])

  const nextCol = useCallback(
    (c: number, dir: 1 | -1): number => {
      let next = c + dir
      while (next >= 0 && next < colCount && skipColumns.includes(next)) {
        next += dir
      }
      return Math.max(0, Math.min(next, colCount - 1))
    },
    [colCount, skipColumns],
  )

  const moveTo = useCallback(
    (row: number, col: number, extend: boolean) => {
      const r = clampRow(row)
      const c = clampCol(col)
      setActiveCell({ row: r, col: c })

      if (extend) {
        const anchor = anchorRef.current ?? { row: r, col: c }
        setSelection({ startRow: anchor.row, startCol: anchor.col, endRow: r, endCol: c })
      } else {
        anchorRef.current = { row: r, col: c }
        setSelection(null)
      }
    },
    [clampRow, clampCol],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (!activeCell) return

      const { row, col } = activeCell
      const shift = e.shiftKey

      switch (e.key) {
        case 'ArrowUp':
          e.preventDefault()
          moveTo(row - 1, col, shift)
          break
        case 'ArrowDown':
          e.preventDefault()
          moveTo(row + 1, col, shift)
          break
        case 'ArrowLeft':
          e.preventDefault()
          moveTo(row, nextCol(col, -1), shift)
          break
        case 'ArrowRight':
          e.preventDefault()
          moveTo(row, nextCol(col, 1), shift)
          break
        case 'Tab':
          e.preventDefault()
          if (shift) {
            moveTo(row, nextCol(col, -1), false)
          } else {
            moveTo(row, nextCol(col, 1), false)
          }
          break
        case 'Enter':
          e.preventDefault()
          moveTo(shift ? row - 1 : row + 1, col, false)
          break
        case 'Escape':
          e.preventDefault()
          setActiveCell(null)
          setSelection(null)
          break
        case 'Home':
          e.preventDefault()
          moveTo(row, 0, shift)
          break
        case 'End':
          e.preventDefault()
          moveTo(row, colCount - 1, shift)
          break
      }
    },
    [activeCell, moveTo, nextCol, colCount],
  )

  const handleCellClick = useCallback(
    (row: number, col: number, e: React.MouseEvent) => {
      if (e.shiftKey && activeCell) {
        const anchor = anchorRef.current ?? activeCell
        setSelection({ startRow: anchor.row, startCol: anchor.col, endRow: row, endCol: col })
        setActiveCell({ row, col })
      } else {
        anchorRef.current = { row, col }
        setActiveCell({ row, col })
        setSelection(null)
      }
    },
    [activeCell],
  )

  // Select all (Ctrl+A).
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    function onSelectAll(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key === 'a' && rowCount > 0 && colCount > 0) {
        e.preventDefault()
        setSelection({ startRow: 0, startCol: 0, endRow: rowCount - 1, endCol: colCount - 1 })
      }
    }
    el.addEventListener('keydown', onSelectAll)
    return () => el.removeEventListener('keydown', onSelectAll)
  }, [rowCount, colCount])

  return {
    activeCell,
    setActiveCell,
    selection,
    setSelection,
    containerRef,
    handleKeyDown,
    handleCellClick,
    moveTo,
  }
}
