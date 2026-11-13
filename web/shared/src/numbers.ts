// How the sites write numbers.

/** count writes a whole number with the locale's thousands separators; locale defaults to the browser's. */
export function count(n: number, locale?: string): string {
  return n.toLocaleString(locale);
}

/** score writes a game score: whole without decimals, else to one place. */
export function score(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

/** signed writes value to decimals places with a sign, and zero (unsigned by default) when it rounds to zero: "+2.1", "-3.0", "0.0". */
export function signed(value: number, decimals: number, zero?: string): string {
  const size = Math.abs(value).toFixed(decimals);
  if (Number(size) === 0) return zero ?? size;
  return value > 0 ? `+${size}` : `-${size}`;
}

/** plural returns one for exactly 1, else many (one with an s by default). */
export function plural(n: number, one: string, many = `${one}s`): string {
  return n === 1 ? one : many;
}

/** counted writes n, as count does, then its word: "1 entry", "3 entries". */
export function counted(n: number, one: string, many = `${one}s`, locale?: string): string {
  return `${count(n, locale)} ${plural(n, one, many)}`;
}

/** ordinal writes a place in English: "1st", "2nd", "3rd", "4th", "11th", "21st". */
export function ordinal(n: number): string {
  const teen = n % 100 >= 11 && n % 100 <= 13;
  const suffix = teen ? 'th' : (['th', 'st', 'nd', 'rd'][n % 10] ?? 'th');
  return `${n}${suffix}`;
}
