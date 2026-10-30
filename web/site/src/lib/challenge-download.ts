/** Every challenge entry as the API offers it to download: as CSV for spreadsheets, or as JSON. */

/** downloadUrls is where the API serves every entry as CSV and as JSON. */
export function downloadUrls(api: string): { csv: string; json: string } {
  const base = `${api.replace(/\/+$/, '')}/v1/challenges/entries`;
  return { csv: `${base}.csv`, json: base };
}
