/** What Enter does in a field offering choices as you type (PickBox). */

/** EnterAction is Enter's effect: pick an option, hold the form back, or let the form send. */
export type EnterAction = { kind: 'pick'; index: number } | { kind: 'hold' } | { kind: 'send' };

/** enterAction is the option chosen with the arrow keys; once something is typed, the first that may be picked, or the form held while none may be; else the form sent. */
export function enterAction(options: readonly { disabled?: boolean }[], chosen: number, typed: boolean): EnterAction {
  if (chosen >= 0 && chosen < options.length) return options[chosen].disabled === true ? { kind: 'hold' } : { kind: 'pick', index: chosen };
  if (!typed) return { kind: 'send' };
  const first = options.findIndex((o) => o.disabled !== true);
  return first >= 0 ? { kind: 'pick', index: first } : { kind: 'hold' };
}
