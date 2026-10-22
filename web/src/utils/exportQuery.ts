/** Export the complete filtered result. Page controls never limit an export. */
export function exportQuery(query: object): Record<string, unknown> {
  return Object.fromEntries(Object.entries(query).filter(([key,value]) =>
    key !== 'page' && key !== 'page_size' && value !== '' && value !== null && value !== undefined,
  ))
}
