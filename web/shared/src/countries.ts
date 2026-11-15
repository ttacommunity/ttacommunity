/** Naming countries: each two-letter code in English, for pickers and for showing a country kept. */

/** Country is a country by its two-letter code, with its name in English. */
export type Country = { code: string; name: string };

/** englishRegions names regions in English, or is null where the browser cannot. */
export function englishRegions(): Intl.DisplayNames | null {
  try {
    return new Intl.DisplayNames(['en'], { type: 'region' });
  } catch {
    return null;
  }
}

/** countryName is code's name in English, or the code itself where there is none. */
export function countryName(code: string, names: Intl.DisplayNames | null = englishRegions()): string {
  try {
    return names?.of(code) ?? code;
  } catch {
    return code;
  }
}

/** countryNames names each code (the code itself where there is no name), sorted by name. */
export function countryNames(codes: readonly string[], names: Intl.DisplayNames | null = englishRegions()): Country[] {
  return codes.map((code) => ({ code, name: countryName(code, names) })).sort((a, b) => a.name.localeCompare(b.name));
}
