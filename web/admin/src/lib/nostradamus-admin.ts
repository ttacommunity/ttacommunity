/** Admin's Nostradamus decisions: outcomes and choices as typed, the question form to and from the API's body, a round's stage, and what keeps it from closing. */
import type { components } from '@ttac/shared/api';
import { personChoices, type Candidate, type PersonChoice } from './person-form';

type Kind = 'pick' | 'set' | 'number';
type Choice = components['schemas']['NostradamusChoice'];
type QuestionBody = components['schemas']['AdminNostradamusQuestionDraft'];

/** outcomeValues reads a typed outcome: a number whole, any other kind's names split on commas; none clears it. */
export function outcomeValues(kind: Kind, text: string): string[] {
  if (kind === 'number') return text.trim() === '' ? [] : [text.trim()];
  return text
    .split(',')
    .map((v) => v.trim())
    .filter((v) => v !== '');
}

/** outcomeText writes an outcome for its field, empty while it is not known. */
export function outcomeText(outcome: readonly string[] | null | undefined): string {
  return outcome == null ? '' : outcome.join(', ');
}

/** wholeOrNone reads a positive whole number, or 0 for anything else. */
function wholeOrNone(text: string | undefined): number {
  const n = Number((text ?? '').trim());
  return Number.isInteger(n) && n > 0 ? n : 0;
}

/** choicesFromText reads a question's choices, one a line: a name, then its seed and leaderboard rank when given, separated by "|". */
export function choicesFromText(text: string): Choice[] {
  return text
    .split('\n')
    .map((line) => line.split('|'))
    .filter(([label]) => (label ?? '').trim() !== '')
    .map(([label, seed, rank]) => ({ label: (label ?? '').trim(), seed: wholeOrNone(seed), leaderboard_rank: wholeOrNone(rank) }));
}

/** choicesText writes choices as choicesFromText reads them, leaving out a seed or rank not given. */
export function choicesText(choices: readonly Choice[]): string {
  return choices
    .map((c) => {
      if (c.leaderboard_rank > 0) return `${c.label} | ${c.seed || ''} | ${c.leaderboard_rank}`;
      return c.seed > 0 ? `${c.label} | ${c.seed}` : c.label;
    })
    .join('\n');
}

/** QuestionForm is the question form's fields, its choices as typed. */
export type QuestionForm = Omit<QuestionBody, 'choices'> & { choices: string };

export const emptyQuestionForm: QuestionForm = { label: '', prompt: '', kind: 'pick', points: 4, near_points: 0, picks: 1, asks_last: false, choices: '' };

/** questionFormFrom fills the form from a question. */
export function questionFormFrom(q: QuestionBody): QuestionForm {
  return { label: q.label, prompt: q.prompt, kind: q.kind, points: q.points, near_points: q.near_points, picks: q.picks, asks_last: q.asks_last, choices: choicesText(q.choices) };
}

/** questionBody is what the form sends: a pick takes one name, only a number scores one off, and a number has no choices or last place. */
export function questionBody(f: QuestionForm): QuestionBody {
  const number = f.kind === 'number';
  return {
    label: f.label.trim(),
    prompt: f.prompt.trim(),
    kind: f.kind,
    points: f.points,
    near_points: number ? f.near_points : 0,
    picks: f.kind === 'set' ? f.picks : 1,
    asks_last: number ? false : f.asks_last,
    choices: number ? [] : choicesFromText(f.choices),
  };
}

export type RoundStatus = 'draft' | 'open' | 'awaiting' | 'closed';

/** roundStatus says where a round is at now: a draft, open for answers, awaiting its outcomes after the deadline, or closed with its results out. */
export function roundStatus(round: { published: boolean; closed: boolean; closes_at?: string | null }, now: Date): RoundStatus {
  if (!round.published) return 'draft';
  if (round.closed) return 'closed';
  return round.closes_at != null && new Date(round.closes_at) > now ? 'open' : 'awaiting';
}

/** closeBlocker says why a published round cannot close yet (its deadline to come, outcomes missing), or '' when it can. */
export function closeBlocker(round: { published: boolean; closed: boolean; closes_at?: string | null; questions: readonly { outcome?: readonly string[] | null }[] }, now: Date): string {
  if (roundStatus(round, now) === 'open') return 'Answers are open until the deadline.';
  const missing = round.questions.filter((q) => q.outcome == null || q.outcome.length === 0).length;
  return missing > 0 ? `Set every outcome first: ${missing} missing.` : '';
}

/** gameHref is a game's page in admin. */
export function gameHref(slug: string): string {
  return `/nostradamus/?${new URLSearchParams({ edition: slug })}`;
}

/** roundLabel is a new round's label: the one typed, else the first of R1, R2, … the game has not taken. */
export function roundLabel(typed: string, taken: readonly string[]): string {
  if (typed.trim() !== '') return typed.trim();
  let n = 1;
  while (taken.includes(`R${n}`)) n++;
  return `R${n}`;
}

/** linkChoices are whom a participant may be linked to: the members and rated players the search found, as personChoices offers them, without a new name, which a link cannot make. */
export function linkChoices(found: readonly Candidate[], typed: string): PersonChoice[] {
  return personChoices(found, typed, { entered: [], membersTaken: false }).filter((c) => c.key !== 'new' && c.key !== 'entered');
}

/** seriesChoices offers the series (not groups) whose name or slug holds the text typed, in any case, as PickBox options naming their slug. */
export function seriesChoices(list: readonly { slug: string; name: string; is_group: boolean }[], typed: string): { key: string; label: string; detail: string }[] {
  const t = typed.trim().toLowerCase();
  return list
    .filter((s) => !s.is_group && (s.name.toLowerCase().includes(t) || s.slug.includes(t)))
    .map((s) => ({ key: s.slug, label: s.name, detail: s.slug }));
}

/** gameSlug makes a game's address name from its name: lower case, words joined by dashes, at most 40 characters. */
export function gameSlug(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40)
    .replace(/-+$/, '');
}

/** withChoice adds name to typed choices on a line of its own, unless a line already names it. */
export function withChoice(text: string, name: string): string {
  if (choicesFromText(text).some((c) => c.label.toLowerCase() === name.toLowerCase())) return text;
  return text.trim() === '' ? name : `${text.replace(/\n+$/, '')}\n${name}`;
}

/** toggled ticks value among values, or unticks it; at most limit stay ticked (0 for no limit), a limit of 1 swapping the one ticked. */
export function toggled(values: readonly string[], value: string, limit: number): string[] {
  if (values.includes(value)) return values.filter((v) => v !== value);
  if (limit === 1) return [value];
  return limit > 0 && values.length >= limit ? [...values] : [...values, value];
}

/** countOrNone reads an optional count field: a whole number, or null for one left empty or not whole. */
export function countOrNone(text: string): number | null {
  const n = Number(text.trim());
  return text.trim() === '' || !Number.isInteger(n) || n < 0 ? null : n;
}

/** tokensText says what a round lets a player stake: its least and most (null for no bound), and each token's worth. */
export function tokensText(min: number | null, max: number | null, points: number): string {
  if (max === 0) return 'no tokens';
  const each = `, ±${points} each`;
  if (min !== null && min > 0 && max !== null) return `${min} to ${max} tokens${each}`;
  if (max !== null) return `up to ${max} tokens${each}`;
  if (min !== null && min > 0) return `at least ${min} tokens${each}`;
  return `any number of tokens${each}`;
}
