// How the sites write numbers.

/** signed writes value to decimals places with a sign, and none when it rounds to zero: "+2.1", "-3.0", "0.0". */
export function signed(value: number, decimals: number): string {
  const size = Math.abs(value).toFixed(decimals);
  if (Number(size) === 0) return size;
  return value > 0 ? `+${size}` : `-${size}`;
}
