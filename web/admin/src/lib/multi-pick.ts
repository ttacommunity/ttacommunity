/** What a multi-select filter shows while closed. */

/** pickSummary is what a multi-select shows closed: all when nothing is ticked, the one ticked, or how many. */
export function pickSummary(options: readonly { value: string; label: string }[], ticked: readonly string[], all: string, noun: string): string {
  if (ticked.length === 0) return all;
  if (ticked.length === 1) return options.find((o) => o.value === ticked[0])?.label ?? all;
  return `${ticked.length} ${noun}`;
}
