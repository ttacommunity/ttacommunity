// How the sites write numbers.

/** count writes a whole number with the locale's thousands separators; locale defaults to the browser's. */
export function count(n: number, locale?: string): string {
  return n.toLocaleString(locale);
}

/** score writes a game score: whole without decimals, else to one place. */
export function score(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

/** signed writes value to decimals places with a sign, and none when it rounds to zero: "+2.1", "-3.0", "0.0". */
export function signed(value: number, decimals: number): string {
  const size = Math.abs(value).toFixed(decimals);
  if (Number(size) === 0) return size;
  return value > 0 ? `+${size}` : `-${size}`;
}
