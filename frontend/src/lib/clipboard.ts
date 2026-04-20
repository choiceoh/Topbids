/**
 * Clipboard utilities for the spreadsheet-style data grid.
 *
 * Supports copy/paste between the grid and external spreadsheet applications
 * (Excel, Google Sheets) via TSV (tab-separated values) on the system clipboard.
 */

import type { SelectionRange } from '@/hooks/useGridNavigation'

/**
 * Extract a rectangular selection from grid data as a 2D array of raw values.
 *
 * Normalizes the selection range (handles inverted start/end) and maps
 * each cell to its raw value from the data array using column IDs.
 * Missing rows or columns yield empty strings.
 */
export function extractSelectionData(
  data: Record<string, unknown>[],
  columnIds: string[],
  range: SelectionRange,
): unknown[][] {
  const r1 = Math.min(range.startRow, range.endRow)
  const r2 = Math.max(range.startRow, range.endRow)
  const c1 = Math.min(range.startCol, range.endCol)
  const c2 = Math.max(range.startCol, range.endCol)

  const result: unknown[][] = []
  for (let r = r1; r <= r2; r++) {
    const row = data[r]
    if (!row) continue
    const cells: unknown[] = []
    for (let c = c1; c <= c2; c++) {
      const colId = columnIds[c]
      if (!colId) continue
      cells.push(row[colId] ?? '')
    }
    result.push(cells)
  }
  return result
}

/**
 * Serialize a 2D array to a TSV (tab-separated values) string.
 *
 * Cells containing tabs, newlines, or double-quotes are wrapped in
 * double-quotes with internal quotes escaped as `""` (RFC 4180 style).
 * Rows are joined with `\n`.
 */
export function toTSV(matrix: unknown[][]): string {
  return matrix
    .map((row) =>
      row
        .map((cell) => {
          const s = cell == null ? '' : String(cell)
          // Escape tabs and newlines.
          if (s.includes('\t') || s.includes('\n') || s.includes('"')) {
            return `"${s.replace(/"/g, '""')}"`
          }
          return s
        })
        .join('\t'),
    )
    .join('\n')
}

/**
 * Copy a grid selection to the system clipboard as TSV text.
 *
 * Uses `navigator.clipboard.writeText` (requires secure context).
 * The TSV format is compatible with Excel and Google Sheets paste.
 */
export async function copyToClipboard(
  data: Record<string, unknown>[],
  columnIds: string[],
  range: SelectionRange,
): Promise<void> {
  const matrix = extractSelectionData(data, columnIds, range)
  const tsv = toTSV(matrix)
  await navigator.clipboard.writeText(tsv)
}

