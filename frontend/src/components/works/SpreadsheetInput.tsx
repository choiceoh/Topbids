import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowDownAZ, ArrowUpAZ, Filter, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import type { Field } from '@/lib/types'
import { getFieldOptions } from '@/lib/fieldGuards'

// ── Types ──

interface SpreadsheetCol {
  key: string
  label: string
  type: 'text' | 'number' | 'select'
  choices?: string[]
}

type Row = Record<string, unknown>
type CellPos = { row: number; col: number }

interface ConditionalRule {
  column: string
  operator: 'gt' | 'lt' | 'eq' | 'neq' | 'empty' | 'not_empty' | 'contains'
  value: string
  style: 'red-bg' | 'green-bg' | 'yellow-bg' | 'bold' | 'red-text' | 'green-text'
}

interface MergedCell {
  row: number
  col: number
  rowSpan: number
  colSpan: number
}

// ── Helpers ──

function colLetter(idx: number) {
  let s = ''
  let n = idx
  do {
    s = String.fromCharCode(65 + (n % 26)) + s
    n = Math.floor(n / 26) - 1
  } while (n >= 0)
  return s
}

/** Parse A1-style cell ref → { row, col } (0-based). */
function parseRef(ref: string): CellPos | null {
  const m = ref.match(/^([A-Z]+)(\d+)$/i)
  if (!m) return null
  let col = 0
  for (const ch of m[1].toUpperCase()) col = col * 26 + ch.charCodeAt(0) - 64
  return { row: Number(m[2]) - 1, col: col - 1 }
}

/** Expand range "A1:B3" into list of CellPos. */
function expandRange(range: string): CellPos[] {
  const [startRef, endRef] = range.split(':')
  const start = parseRef(startRef)
  const end = endRef ? parseRef(endRef) : start
  if (!start || !end) return []
  const cells: CellPos[] = []
  const r0 = Math.min(start.row, end.row), r1 = Math.max(start.row, end.row)
  const c0 = Math.min(start.col, end.col), c1 = Math.max(start.col, end.col)
  for (let r = r0; r <= r1; r++)
    for (let c = c0; c <= c1; c++)
      cells.push({ row: r, col: c })
  return cells
}

/** Evaluate simple formula. Supports SUM, AVG, MIN, MAX, COUNT, IF, basic arithmetic. */
function evaluateFormula(
  expr: string,
  rows: Row[],
  columns: SpreadsheetCol[],
  currentRow: number,
  currentCol: number,
): unknown {
  const upper = expr.toUpperCase().trim()

  // Gather numeric values from a range expression
  function getValues(rangeExpr: string): number[] {
    const cells = expandRange(rangeExpr.trim())
    return cells
      .filter((c) => !(c.row === currentRow && c.col === currentCol)) // prevent self-ref
      .map((c) => {
        const colDef = columns[c.col]
        if (!colDef || !rows[c.row]) return NaN
        const raw = rows[c.row][colDef.key]
        return raw == null || raw === '' ? NaN : Number(raw)
      })
      .filter((n) => !isNaN(n))
  }

  // Resolve a single cell ref or literal to a number
  function resolveValue(token: string): number {
    const trimmed = token.trim()
    const n = Number(trimmed)
    if (!isNaN(n)) return n
    const ref = parseRef(trimmed)
    if (ref) {
      const colDef = columns[ref.col]
      if (!colDef || !rows[ref.row]) return 0
      const raw = rows[ref.row][colDef.key]
      return raw == null || raw === '' ? 0 : Number(raw) || 0
    }
    return 0
  }

  // SUM(A1:A5)
  const sumMatch = upper.match(/^SUM\((.+)\)$/)
  if (sumMatch) {
    const vals = getValues(sumMatch[1])
    return vals.reduce((a, b) => a + b, 0)
  }

  // AVG / AVERAGE
  const avgMatch = upper.match(/^(?:AVG|AVERAGE)\((.+)\)$/)
  if (avgMatch) {
    const vals = getValues(avgMatch[1])
    return vals.length ? vals.reduce((a, b) => a + b, 0) / vals.length : 0
  }

  // MIN
  const minMatch = upper.match(/^MIN\((.+)\)$/)
  if (minMatch) {
    const vals = getValues(minMatch[1])
    return vals.length ? Math.min(...vals) : 0
  }

  // MAX
  const maxMatch = upper.match(/^MAX\((.+)\)$/)
  if (maxMatch) {
    const vals = getValues(maxMatch[1])
    return vals.length ? Math.max(...vals) : 0
  }

  // COUNT
  const countMatch = upper.match(/^COUNT\((.+)\)$/)
  if (countMatch) {
    return getValues(countMatch[1]).length
  }

  // IF(condition, trueVal, falseVal) — simple: IF(A1>0, A1, 0)
  const ifMatch = expr.match(/^IF\((.+?)\s*(>|<|>=|<=|=|<>|!=)\s*(.+?)\s*,\s*(.+?)\s*,\s*(.+?)\)$/i)
  if (ifMatch) {
    const left = resolveValue(ifMatch[1])
    const right = resolveValue(ifMatch[3])
    const op = ifMatch[2]
    let cond = false
    if (op === '>') cond = left > right
    else if (op === '<') cond = left < right
    else if (op === '>=' ) cond = left >= right
    else if (op === '<=') cond = left <= right
    else if (op === '=' || op === '==') cond = left === right
    else if (op === '<>' || op === '!=') cond = left !== right
    return cond ? resolveValue(ifMatch[4]) : resolveValue(ifMatch[5])
  }

  // Simple arithmetic: replace cell refs with values, then eval
  try {
    const replaced = expr.replace(/[A-Z]+\d+/gi, (ref) => {
      return String(resolveValue(ref))
    })
    // safe eval — only numbers and operators
    if (/^[\d\s+\-*/().]+$/.test(replaced)) {
      return new Function(`return (${replaced})`)()
    }
  } catch { /* fallthrough */ }

  return '#ERROR'
}

// ── Conditional formatting styles ──

const COND_STYLES: Record<string, string> = {
  'red-bg': 'bg-red-100',
  'green-bg': 'bg-green-100',
  'yellow-bg': 'bg-yellow-100',
  'bold': 'font-bold',
  'red-text': 'text-red-600',
  'green-text': 'text-green-600',
}

const COND_STYLE_LABELS: { value: string; label: string }[] = [
  { value: 'red-bg', label: '빨간 배경' },
  { value: 'green-bg', label: '초록 배경' },
  { value: 'yellow-bg', label: '노란 배경' },
  { value: 'bold', label: '굵게' },
  { value: 'red-text', label: '빨간 글씨' },
  { value: 'green-text', label: '초록 글씨' },
]

const COND_OPERATOR_LABELS: { value: ConditionalRule['operator']; label: string }[] = [
  { value: 'gt', label: '>' },
  { value: 'lt', label: '<' },
  { value: 'eq', label: '=' },
  { value: 'neq', label: '≠' },
  { value: 'contains', label: '포함' },
  { value: 'empty', label: '비어있음' },
  { value: 'not_empty', label: '값 있음' },
]

function matchesRule(cellValue: unknown, rule: ConditionalRule): boolean {
  const s = cellValue == null ? '' : String(cellValue)
  const n = Number(s)
  const rv = Number(rule.value)
  switch (rule.operator) {
    case 'gt': return !isNaN(n) && !isNaN(rv) && n > rv
    case 'lt': return !isNaN(n) && !isNaN(rv) && n < rv
    case 'eq': return s === rule.value
    case 'neq': return s !== rule.value
    case 'contains': return s.includes(rule.value)
    case 'empty': return s === ''
    case 'not_empty': return s !== ''
    default: return false
  }
}

// ── Selection helpers ──

function normalizeSelection(anchor: CellPos, cursor: CellPos) {
  return {
    r0: Math.min(anchor.row, cursor.row),
    r1: Math.max(anchor.row, cursor.row),
    c0: Math.min(anchor.col, cursor.col),
    c1: Math.max(anchor.col, cursor.col),
  }
}

function inSelection(row: number, col: number, sel: { r0: number; r1: number; c0: number; c1: number } | null) {
  if (!sel) return false
  return row >= sel.r0 && row <= sel.r1 && col >= sel.c0 && col <= sel.c1
}

// ── Component ──

export default function SpreadsheetInput({
  field,
  value,
  onChange,
}: {
  field: Field
  value: unknown
  onChange: (v: unknown) => void
}) {
  const spreadOpts = getFieldOptions(field, 'spreadsheet')
  const subColumns: SpreadsheetCol[] = (spreadOpts?.sub_columns as SpreadsheetCol[]) || [
    { key: 'col1', label: 'A', type: 'text' },
    { key: 'col2', label: 'B', type: 'text' },
    { key: 'col3', label: 'C', type: 'text' },
  ]
  const initialRows = spreadOpts?.initial_rows || 5
  const rawRows = Array.isArray(value) ? (value as Row[]) : []

  const displayRows = useMemo(() => {
    if (rawRows.length >= initialRows) return rawRows
    return [
      ...rawRows,
      ...Array.from({ length: initialRows - rawRows.length }, () => {
        const empty: Row = {}
        for (const col of subColumns) empty[col.key] = col.type === 'number' ? null : ''
        return empty
      }),
    ]
  }, [rawRows, initialRows, subColumns])

  // ── State ──
  const [activeCell, setActiveCell] = useState<CellPos | null>(null)
  const [selAnchor, setSelAnchor] = useState<CellPos | null>(null)
  const [selCursor, setSelCursor] = useState<CellPos | null>(null)
  const [editingFormula, setEditingFormula] = useState<string | null>(null) // raw formula text while editing
  const [colWidths, setColWidths] = useState<Record<number, number>>({})
  const [rowHeights, setRowHeights] = useState<Record<number, number>>({})
  const [resizingCol, setResizingCol] = useState<{ idx: number; startX: number; startW: number } | null>(null)
  const [resizingRow, setResizingRow] = useState<{ idx: number; startY: number; startH: number } | null>(null)
  const [sortCol, setSortCol] = useState<{ idx: number; asc: boolean } | null>(null)
  const [filterCol, setFilterCol] = useState<number | null>(null)
  const [filterText, setFilterText] = useState('')
  const [condRules, setCondRules] = useState<ConditionalRule[]>(
    Array.isArray(spreadOpts?.conditional_rules)
      ? (spreadOpts.conditional_rules as unknown as ConditionalRule[])
      : [],
  )
  const [mergedCells, setMergedCells] = useState<MergedCell[]>(
    Array.isArray(spreadOpts?.merged_cells)
      ? (spreadOpts.merged_cells as unknown as MergedCell[])
      : [],
  )
  const [showCondEditor, setShowCondEditor] = useState(false)

  const tableRef = useRef<HTMLDivElement>(null)

  const selection = selAnchor && selCursor ? normalizeSelection(selAnchor, selCursor) : null

  // ── Data operations ──

  function emitChange(nextRows: Row[]) {
    let end = nextRows.length
    while (end > 0) {
      const row = nextRows[end - 1]
      const isEmpty = subColumns.every((c) => {
        const v = row[c.key]
        return v === null || v === undefined || v === ''
      })
      if (!isEmpty) break
      end--
    }
    onChange(nextRows.slice(0, Math.max(end, 0)))
  }

  const visibleRows = useMemo(() => {
    let indexed = displayRows.map((row, i) => ({ row, originalIdx: i }))

    // filter
    if (filterCol !== null && filterText) {
      const colKey = subColumns[filterCol]?.key
      if (colKey) {
        const lower = filterText.toLowerCase()
        indexed = indexed.filter(({ row }) => {
          const v = row[colKey]
          return v != null && String(v).toLowerCase().includes(lower)
        })
      }
    }

    // sort
    if (sortCol !== null) {
      const colKey = subColumns[sortCol.idx]?.key
      const colType = subColumns[sortCol.idx]?.type
      if (colKey) {
        indexed.sort((a, b) => {
          const av = a.row[colKey]
          const bv = b.row[colKey]
          let cmp: number
          if (colType === 'number') {
            cmp = (Number(av) || 0) - (Number(bv) || 0)
          } else {
            cmp = String(av ?? '').localeCompare(String(bv ?? ''))
          }
          return sortCol!.asc ? cmp : -cmp
        })
      }
    }

    return indexed
  }, [displayRows, filterCol, filterText, sortCol, subColumns])

  function updateCell(originalIdx: number, colKey: string, val: unknown) {
    const next = displayRows.map((r, i) => (i === originalIdx ? { ...r, [colKey]: val } : { ...r }))
    emitChange(next)
  }

  function addRow() {
    const empty: Row = {}
    for (const col of subColumns) empty[col.key] = col.type === 'number' ? null : ''
    emitChange([...displayRows, empty])
  }

  // ── Cell display value (formulas evaluated) ──

  function getCellDisplay(row: Row, colKey: string, rowIdx: number, colIdx: number): { display: string; isFormula: boolean } {
    const raw = row[colKey]
    if (typeof raw === 'string' && raw.startsWith('=')) {
      const result = evaluateFormula(raw.slice(1), displayRows, subColumns, rowIdx, colIdx)
      return { display: String(result), isFormula: true }
    }
    return { display: raw != null ? String(raw) : '', isFormula: false }
  }

  // ── Conditional formatting for a cell ──

  function getCellCondStyles(colKey: string, cellValue: unknown): string {
    const classes: string[] = []
    for (const rule of condRules) {
      if (rule.column === colKey && matchesRule(cellValue, rule)) {
        const cls = COND_STYLES[rule.style]
        if (cls) classes.push(cls)
      }
    }
    return classes.join(' ')
  }

  // ── Merged cell helpers ──

  function getMerge(row: number, col: number): MergedCell | undefined {
    return mergedCells.find((m) =>
      row >= m.row && row < m.row + m.rowSpan &&
      col >= m.col && col < m.col + m.colSpan,
    )
  }

  function isMergeOrigin(row: number, col: number): MergedCell | undefined {
    return mergedCells.find((m) => m.row === row && m.col === col)
  }

  function isMergeHidden(row: number, col: number): boolean {
    const merge = getMerge(row, col)
    return !!merge && (merge.row !== row || merge.col !== col)
  }

  function handleMerge() {
    if (!selection) return
    const { r0, r1, c0, c1 } = selection
    if (r0 === r1 && c0 === c1) return

    // Remove any existing merges in the selection
    const filtered = mergedCells.filter((m) => {
      const overlaps = m.row < r1 + 1 && m.row + m.rowSpan > r0 &&
        m.col < c1 + 1 && m.col + m.colSpan > c0
      return !overlaps
    })

    // Check if we're unmerging (exact existing merge)
    const existing = mergedCells.find((m) =>
      m.row === r0 && m.col === c0 && m.rowSpan === r1 - r0 + 1 && m.colSpan === c1 - c0 + 1,
    )

    if (existing) {
      setMergedCells(filtered)
    } else {
      setMergedCells([...filtered, { row: r0, col: c0, rowSpan: r1 - r0 + 1, colSpan: c1 - c0 + 1 }])
    }
  }

  // ── Selection ──

  function handleCellClick(row: number, col: number, e: React.MouseEvent) {
    if (e.shiftKey && activeCell) {
      setSelCursor({ row, col })
    } else {
      setActiveCell({ row, col })
      setSelAnchor({ row, col })
      setSelCursor({ row, col })
      setEditingFormula(null)
    }
  }

  // ── Keyboard navigation ──

  function focusCell(row: number, col: number) {
    setActiveCell({ row, col })
    setEditingFormula(null)
    requestAnimationFrame(() => {
      const input = tableRef.current?.querySelector(
        `[data-cell="${row}-${col}"]`,
      ) as HTMLInputElement | null
      input?.focus()
    })
  }

  function handleKeyDown(e: React.KeyboardEvent, rowIdx: number, colIdx: number) {
    const maxRow = visibleRows.length - 1
    const maxCol = subColumns.length - 1
    let nextRow = rowIdx
    let nextCol = colIdx

    // Shift+Arrow for selection expansion
    if (e.shiftKey && ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(e.key)) {
      e.preventDefault()
      const cursor = selCursor || { row: rowIdx, col: colIdx }
      let nr = cursor.row, nc = cursor.col
      if (e.key === 'ArrowUp') nr = Math.max(0, nr - 1)
      if (e.key === 'ArrowDown') nr = Math.min(maxRow, nr + 1)
      if (e.key === 'ArrowLeft') nc = Math.max(0, nc - 1)
      if (e.key === 'ArrowRight') nc = Math.min(maxCol, nc + 1)
      if (!selAnchor) setSelAnchor({ row: rowIdx, col: colIdx })
      setSelCursor({ row: nr, col: nc })
      return
    }

    switch (e.key) {
      case 'Tab':
        e.preventDefault()
        if (e.shiftKey) {
          if (colIdx > 0) nextCol = colIdx - 1
          else if (rowIdx > 0) { nextRow = rowIdx - 1; nextCol = maxCol }
        } else {
          if (colIdx < maxCol) nextCol = colIdx + 1
          else if (rowIdx < maxRow) { nextRow = rowIdx + 1; nextCol = 0 }
          else { addRow(); nextRow = rowIdx + 1; nextCol = 0 }
        }
        break
      case 'Enter':
        e.preventDefault()
        if (rowIdx < maxRow) nextRow = rowIdx + 1
        else { addRow(); nextRow = rowIdx + 1 }
        break
      case 'ArrowUp':
        if (rowIdx > 0) nextRow = rowIdx - 1
        break
      case 'ArrowDown':
        if (rowIdx < maxRow) nextRow = rowIdx + 1
        break
      case 'Escape':
        setEditingFormula(null)
        setSelAnchor(null)
        setSelCursor(null)
        return
      case 'Delete':
      case 'Backspace':
        if (selection && !(e.target instanceof HTMLInputElement)) {
          e.preventDefault()
          const { r0, r1, c0, c1 } = selection
          const next = displayRows.map((r, ri) => {
            if (ri < r0 || ri > r1) return { ...r }
            const updated = { ...r }
            for (let ci = c0; ci <= c1; ci++) {
              const colDef = subColumns[ci]
              if (colDef) updated[colDef.key] = colDef.type === 'number' ? null : ''
            }
            return updated
          })
          emitChange(next)
          return
        }
        return
      default:
        return
    }

    // clear selection on single-cell nav
    setSelAnchor({ row: nextRow, col: nextCol })
    setSelCursor({ row: nextRow, col: nextCol })
    focusCell(nextRow, nextCol)
  }

  // ── Copy / Paste ──

  useEffect(() => {
    const container = tableRef.current
    if (!container) return

    function handleCopy(e: ClipboardEvent) {
      if (!selection) return
      e.preventDefault()
      const { r0, r1, c0, c1 } = selection
      const lines: string[] = []
      for (let ri = r0; ri <= r1; ri++) {
        const cells: string[] = []
        for (let ci = c0; ci <= c1; ci++) {
          const colDef = subColumns[ci]
          const row = displayRows[ri]
          if (colDef && row) {
            const { display } = getCellDisplay(row, colDef.key, ri, ci)
            cells.push(display)
          } else {
            cells.push('')
          }
        }
        lines.push(cells.join('\t'))
      }
      e.clipboardData?.setData('text/plain', lines.join('\n'))
    }

    function handlePaste(e: ClipboardEvent) {
      const target = activeCell
      if (!target) return
      const text = e.clipboardData?.getData('text/plain')
      if (!text) return
      e.preventDefault()

      const pasteRows = text.split('\n').map((line) => line.split('\t'))
      const next = [...displayRows.map((r) => ({ ...r }))]

      // extend rows if needed
      const neededRows = target.row + pasteRows.length
      while (next.length < neededRows) {
        const empty: Row = {}
        for (const col of subColumns) empty[col.key] = col.type === 'number' ? null : ''
        next.push(empty)
      }

      for (let pr = 0; pr < pasteRows.length; pr++) {
        for (let pc = 0; pc < pasteRows[pr].length; pc++) {
          const ri = target.row + pr
          const ci = target.col + pc
          const colDef = subColumns[ci]
          if (colDef && next[ri]) {
            const val = pasteRows[pr][pc]
            next[ri][colDef.key] = colDef.type === 'number'
              ? (val === '' ? null : Number(val) || val)
              : val
          }
        }
      }

      emitChange(next)

      // select pasted range
      setSelAnchor(target)
      setSelCursor({
        row: Math.min(target.row + pasteRows.length - 1, next.length - 1),
        col: Math.min(target.col + (pasteRows[0]?.length || 1) - 1, subColumns.length - 1),
      })
    }

    container.addEventListener('copy', handleCopy)
    container.addEventListener('paste', handlePaste)
    return () => {
      container.removeEventListener('copy', handleCopy)
      container.removeEventListener('paste', handlePaste)
    }
  }, [selection, activeCell, displayRows, subColumns])

  // ── Column resize ──

  useEffect(() => {
    if (!resizingCol) return
    function onMove(e: MouseEvent) {
      if (!resizingCol) return
      const delta = e.clientX - resizingCol.startX
      setColWidths((prev) => ({
        ...prev,
        [resizingCol.idx]: Math.max(60, resizingCol.startW + delta),
      }))
    }
    function onUp() { setResizingCol(null) }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    return () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
  }, [resizingCol])

  // ── Row resize ──

  useEffect(() => {
    if (!resizingRow) return
    function onMove(e: MouseEvent) {
      if (!resizingRow) return
      const delta = e.clientY - resizingRow.startY
      setRowHeights((prev) => ({
        ...prev,
        [resizingRow.idx]: Math.max(24, resizingRow.startH + delta),
      }))
    }
    function onUp() { setResizingRow(null) }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    return () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
  }, [resizingRow])

  // ── Sort toggle ──

  function toggleSort(colIdx: number) {
    if (sortCol?.idx === colIdx) {
      if (sortCol.asc) setSortCol({ idx: colIdx, asc: false })
      else setSortCol(null) // third click removes sort
    } else {
      setSortCol({ idx: colIdx, asc: true })
    }
  }

  // ── Render ──

  const formulaBarValue = useMemo(() => {
    if (!activeCell) return ''
    const vr = visibleRows[activeCell.row]
    if (!vr) return ''
    const colDef = subColumns[activeCell.col]
    if (!colDef) return ''
    const raw = vr.row[colDef.key]
    return raw != null ? String(raw) : ''
  }, [activeCell, visibleRows, subColumns])

  return (
    <div className="space-y-1">
      {/* Formula bar */}
      <div className="flex items-center gap-1 rounded border bg-muted/30 px-2 py-0.5 text-xs">
        <span className="font-mono text-muted-foreground w-8 text-center">
          {activeCell ? `${colLetter(activeCell.col)}${activeCell.row + 1}` : ''}
        </span>
        <span className="text-muted-foreground">│</span>
        <span className="font-mono text-xs italic text-foreground/80 truncate flex-1">
          {editingFormula ?? formulaBarValue}
        </span>
      </div>

      {/* Toolbar */}
      <div className="flex items-center gap-1 flex-wrap">
        {selection && !(selection.r0 === selection.r1 && selection.c0 === selection.c1) && (
          <Button type="button" variant="outline" size="sm" className="h-6 text-[10px]" onClick={handleMerge}>
            {mergedCells.find((m) =>
              m.row === selection.r0 && m.col === selection.c0 &&
              m.rowSpan === selection.r1 - selection.r0 + 1 &&
              m.colSpan === selection.c1 - selection.c0 + 1,
            ) ? '병합 해제' : '셀 병합'}
          </Button>
        )}
        <Button
          type="button"
          variant={showCondEditor ? 'secondary' : 'outline'}
          size="sm"
          className="h-6 text-[10px]"
          onClick={() => setShowCondEditor(!showCondEditor)}
        >
          조건부 서식 {condRules.length > 0 && `(${condRules.length})`}
        </Button>
      </div>

      {/* Conditional formatting editor */}
      {showCondEditor && (
        <div className="rounded border bg-muted/20 p-2 space-y-2 text-xs">
          <div className="font-medium text-muted-foreground">조건부 서식 규칙</div>
          {condRules.map((rule, i) => (
            <div key={i} className="flex items-center gap-1 flex-wrap">
              <select
                className="h-6 rounded border bg-transparent px-1 text-xs"
                value={rule.column}
                onChange={(e) => {
                  const next = [...condRules]
                  next[i] = { ...rule, column: e.target.value }
                  setCondRules(next)
                }}
              >
                <option value="">열 선택</option>
                {subColumns.map((c) => (
                  <option key={c.key} value={c.key}>{c.label}</option>
                ))}
              </select>
              <select
                className="h-6 rounded border bg-transparent px-1 text-xs"
                value={rule.operator}
                onChange={(e) => {
                  const next = [...condRules]
                  next[i] = { ...rule, operator: e.target.value as ConditionalRule['operator'] }
                  setCondRules(next)
                }}
              >
                {COND_OPERATOR_LABELS.map((o) => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
              {!['empty', 'not_empty'].includes(rule.operator) && (
                <Input
                  className="h-6 w-16 text-xs"
                  value={rule.value}
                  onChange={(e) => {
                    const next = [...condRules]
                    next[i] = { ...rule, value: e.target.value }
                    setCondRules(next)
                  }}
                  placeholder="값"
                />
              )}
              <select
                className="h-6 rounded border bg-transparent px-1 text-xs"
                value={rule.style}
                onChange={(e) => {
                  const next = [...condRules]
                  next[i] = { ...rule, style: e.target.value as ConditionalRule['style'] }
                  setCondRules(next)
                }}
              >
                {COND_STYLE_LABELS.map((s) => (
                  <option key={s.value} value={s.value}>{s.label}</option>
                ))}
              </select>
              <button
                type="button"
                className="text-muted-foreground hover:text-destructive"
                onClick={() => setCondRules(condRules.filter((_, j) => j !== i))}
              >
                <X className="h-3 w-3" />
              </button>
            </div>
          ))}
          <button
            type="button"
            className="text-xs text-primary hover:underline"
            onClick={() => setCondRules([...condRules, { column: subColumns[0]?.key || '', operator: 'gt', value: '', style: 'red-bg' }])}
          >
            + 규칙 추가
          </button>
        </div>
      )}

      {/* Spreadsheet grid */}
      <div
        ref={tableRef}
        tabIndex={0}
        className="rounded-md border overflow-auto max-h-96 focus:outline-none select-none"
        onKeyDown={(e) => {
          if (activeCell && !(e.target instanceof HTMLInputElement) && !(e.target instanceof HTMLSelectElement)) {
            handleKeyDown(e, activeCell.row, activeCell.col)
          }
        }}
      >
        <table className="w-full text-sm border-collapse" style={{ tableLayout: 'fixed' }}>
          <colgroup>
            <col style={{ width: 40 }} />
            {subColumns.map((_, ci) => (
              <col key={ci} style={{ width: colWidths[ci] || 120 }} />
            ))}
          </colgroup>
          <thead className="sticky top-0 z-10">
            <tr className="bg-muted/70">
              <th className="px-1.5 py-1 text-center text-[10px] font-medium text-muted-foreground border-r border-b" />
              {subColumns.map((col, ci) => (
                <th
                  key={col.key}
                  className="relative px-1 py-1 text-center text-xs font-medium text-muted-foreground border-r border-b group"
                >
                  <div className="flex items-center justify-center gap-0.5">
                    <span className="text-[10px] text-muted-foreground/50">{colLetter(ci)}</span>
                    <span className="truncate">{col.label}</span>
                    {/* sort button */}
                    <button
                      type="button"
                      className="opacity-0 group-hover:opacity-100 transition-opacity"
                      onClick={() => toggleSort(ci)}
                      title="정렬"
                    >
                      {sortCol?.idx === ci
                        ? (sortCol.asc ? <ArrowDownAZ className="h-3 w-3" /> : <ArrowUpAZ className="h-3 w-3" />)
                        : <ArrowDownAZ className="h-3 w-3 text-muted-foreground/40" />
                      }
                    </button>
                    {/* filter button */}
                    <Popover>
                      <PopoverTrigger
                        className={`opacity-0 group-hover:opacity-100 transition-opacity ${filterCol === ci ? '!opacity-100 text-primary' : ''}`}
                        title="필터"
                      >
                        <Filter className="h-3 w-3" />
                      </PopoverTrigger>
                      <PopoverContent className="w-48 p-2" align="start">
                        <Input
                          className="h-7 text-xs"
                          placeholder="필터 값 입력..."
                          value={filterCol === ci ? filterText : ''}
                          onChange={(e) => {
                            setFilterCol(ci)
                            setFilterText(e.target.value)
                          }}
                          autoFocus
                        />
                        {filterCol === ci && filterText && (
                          <button
                            type="button"
                            className="mt-1 text-xs text-muted-foreground hover:text-foreground"
                            onClick={() => { setFilterCol(null); setFilterText('') }}
                          >
                            필터 해제
                          </button>
                        )}
                      </PopoverContent>
                    </Popover>
                  </div>
                  {/* Column resize handle */}
                  <div
                    className="absolute right-0 top-0 h-full w-1.5 cursor-col-resize hover:bg-primary/30"
                    onMouseDown={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      const th = (e.target as HTMLElement).parentElement!
                      setResizingCol({ idx: ci, startX: e.clientX, startW: th.offsetWidth })
                    }}
                  />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visibleRows.map(({ row, originalIdx }, ri) => (
              <tr key={originalIdx}>
                <td
                  className="relative px-1.5 py-0 text-center text-[10px] text-muted-foreground border-r border-b bg-muted/30 select-none"
                  style={{ height: rowHeights[ri] || 28 }}
                >
                  {ri + 1}
                  {/* Row resize handle */}
                  <div
                    className="absolute bottom-0 left-0 w-full h-1 cursor-row-resize hover:bg-primary/30"
                    onMouseDown={(e) => {
                      e.preventDefault()
                      const td = (e.target as HTMLElement).parentElement!
                      setResizingRow({ idx: ri, startY: e.clientY, startH: td.offsetHeight })
                    }}
                  />
                </td>
                {subColumns.map((col, ci) => {
                  if (isMergeHidden(ri, ci)) return null
                  const merge = isMergeOrigin(ri, ci)
                  const isActive = activeCell?.row === ri && activeCell?.col === ci
                  const isSelected = inSelection(ri, ci, selection)
                  const cellValue = row[col.key]
                  const { display, isFormula } = getCellDisplay(row, col.key, originalIdx, ci)
                  const condClass = getCellCondStyles(col.key, isFormula ? display : cellValue)

                  return (
                    <td
                      key={col.key}
                      className={[
                        'p-0 border-r border-b relative',
                        isActive ? 'ring-2 ring-primary ring-inset z-[5]' : '',
                        isSelected && !isActive ? 'bg-primary/10' : '',
                        condClass,
                      ].filter(Boolean).join(' ')}
                      style={{ height: rowHeights[ri] || 28 }}
                      rowSpan={merge?.rowSpan}
                      colSpan={merge?.colSpan}
                      onClick={(e) => handleCellClick(ri, ci, e)}
                    >
                      {col.type === 'select' ? (
                        <select
                          data-cell={`${ri}-${ci}`}
                          className="h-full w-full bg-transparent px-1.5 text-sm outline-none border-0"
                          value={(cellValue as string) || ''}
                          onChange={(e) => updateCell(originalIdx, col.key, e.target.value)}
                          onFocus={() => { setActiveCell({ row: ri, col: ci }); setEditingFormula(null) }}
                          onKeyDown={(e) => handleKeyDown(e, ri, ci)}
                        >
                          <option value="" />
                          {(col.choices || []).map((c) => (
                            <option key={c} value={c}>{c}</option>
                          ))}
                        </select>
                      ) : (
                        <input
                          data-cell={`${ri}-${ci}`}
                          type={isFormula || col.type === 'text' ? 'text' : 'number'}
                          className={[
                            'h-full w-full bg-transparent px-1.5 text-sm outline-none border-0',
                            isFormula && editingFormula === null ? 'text-right' : '',
                            condClass,
                          ].filter(Boolean).join(' ')}
                          value={
                            isActive && editingFormula !== null
                              ? editingFormula
                              : isFormula && !(isActive && document.activeElement?.getAttribute('data-cell') === `${ri}-${ci}`)
                                ? display
                                : (cellValue != null ? String(cellValue) : '')
                          }
                          onChange={(e) => {
                            const v = e.target.value
                            if (v.startsWith('=')) {
                              setEditingFormula(v)
                              updateCell(originalIdx, col.key, v)
                            } else {
                              setEditingFormula(null)
                              updateCell(
                                originalIdx,
                                col.key,
                                col.type === 'number'
                                  ? v === '' ? null : Number(v)
                                  : v,
                              )
                            }
                          }}
                          onFocus={() => {
                            setActiveCell({ row: ri, col: ci })
                            const raw = row[col.key]
                            if (typeof raw === 'string' && raw.startsWith('=')) {
                              setEditingFormula(raw)
                            } else {
                              setEditingFormula(null)
                            }
                          }}
                          onBlur={() => setEditingFormula(null)}
                          onKeyDown={(e) => handleKeyDown(e, ri, ci)}
                        />
                      )}
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Footer */}
      <div className="flex items-center gap-2 flex-wrap">
        <Button type="button" variant="outline" size="sm" onClick={addRow}>
          + 행 추가
        </Button>
        <span className="text-[10px] text-muted-foreground">
          {visibleRows.length}행 × {subColumns.length}열
          {filterCol !== null && filterText && ` (필터 적용)`}
        </span>
        {selection && (() => {
          const { r0, r1, c0, c1 } = selection
          if (r0 === r1 && c0 === c1) return null
          // quick stats for selected numeric cells
          const nums: number[] = []
          for (let r = r0; r <= r1; r++) {
            for (let c = c0; c <= c1; c++) {
              const colDef = subColumns[c]
              const vr = visibleRows[r]
              if (colDef && vr) {
                const v = Number(vr.row[colDef.key])
                if (!isNaN(v) && vr.row[colDef.key] !== '' && vr.row[colDef.key] != null) nums.push(v)
              }
            }
          }
          if (nums.length === 0) return null
          const sum = nums.reduce((a, b) => a + b, 0)
          return (
            <span className="text-[10px] text-muted-foreground ml-auto">
              합계: {sum.toLocaleString()} | 평균: {(sum / nums.length).toFixed(1)} | 개수: {nums.length}
            </span>
          )
        })()}
      </div>
    </div>
  )
}
