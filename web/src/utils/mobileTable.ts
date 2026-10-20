/** Reuse the existing cell contents and actions in the mobile record layout. */
export function mobileTableCellStyle({ column }: { column: { label?: string } }) {
  // CSS string escaping keeps labels as text, including quotes and line breaks.
  const label = (column.label || '').replace(/["\\\n\r\f]/g, char => `\\${char.codePointAt(0)!.toString(16)} `)
  return { '--mobile-label': `"${label}"` }
}
