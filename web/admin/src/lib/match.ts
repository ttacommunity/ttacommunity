/** How well a name matches the text typed into a field that offers choices as you type. */

/** norm is a name as typed matches it: lower case, an underscore as a space, no space around. */
export const norm = (name: string) => name.trim().toLowerCase().replaceAll('_', ' ');

/** wordsStart says whether each word typed starts a word of the name, in the name's order ("eu c" for Europe Central). */
function wordsStart(name: string, typed: string): boolean {
  const words = name.split(/[\s/&-]+/);
  let at = 0;
  for (const t of typed.split(/\s+/)) {
    while (at < words.length && !words[at].startsWith(t)) at++;
    if (at++ >= words.length) return false;
  }
  return true;
}

/** matchRank is how well a name matches the text typed, best first: 0 the name itself, 1 starting with it, 2 holding it, 3 each word typed starting one of the name's; null for no match. */
export function matchRank(name: string, typed: string): number | null {
  const n = norm(name);
  const q = norm(typed);
  if (n === q) return 0;
  if (n.startsWith(q)) return 1;
  if (n.includes(q)) return 2;
  return wordsStart(n, q) ? 3 : null;
}
