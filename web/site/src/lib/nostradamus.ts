/** Nostradamus pages' decisions: which game to show, each round's stage, what the join box offers, a player's answers as they fill them in, and how names, points and polls read. */

/** editionParam is the game the address names (?edition=), or null for none. */
export function editionParam(search: string): string | null {
  const edition = (new URLSearchParams(search).get('edition') ?? '').trim();
  return edition === '' ? null : edition;
}

/** chosenEdition is the game asked for when it is one, else the newest (games come newest first), or null. */
export function chosenEdition(editions: readonly { slug: string }[], wanted: string | null): string | null {
  if (wanted !== null && editions.some((e) => e.slug === wanted)) return wanted;
  return editions[0]?.slug ?? null;
}

/** nostradamusHref is the address of a game's page. */
export function nostradamusHref(edition: string): string {
  return `/nostradamus/?${new URLSearchParams({ edition })}`;
}

/** shownName is the member a participant is linked to, else the name it plays under. */
export function shownName(p: { name: string; member?: string | null }): string {
  return p.member ?? p.name;
}

/** valuesText writes an answer or outcome's names, or says it is not known yet. */
export function valuesText(values: readonly string[] | null | undefined): string {
  return values == null || values.length === 0 ? 'Not known yet' : values.join(', ');
}

/** pointsText writes an answer's points signed, or a dash until they are known. */
export function pointsText(points: number | null | undefined): string {
  if (points == null) return '–';
  if (points > 0) return `+${points}`;
  return points < 0 ? `−${-points}` : '0';
}

/** places gives each total, highest first, its place: a tie shares one, and the next comes after them all. */
export function places(totals: readonly number[]): number[] {
  return totals.map((t, i) => (i > 0 && totals[i - 1] === t ? totals.indexOf(t) + 1 : i + 1));
}

export type RoundStage = 'open' | 'awaiting' | 'closed';

/** roundStage says where a published round is at now: open for answers, awaiting its results after the deadline, or closed with them out. */
export function roundStage(round: { closed: boolean; closes_at?: string | null }, now: Date): RoundStage {
  if (round.closed) return 'closed';
  return round.closes_at != null && new Date(round.closes_at) > now ? 'open' : 'awaiting';
}

export type JoinState = 'sign-in' | 'link' | 'ask' | 'requested' | 'refused' | 'play';

/** joinState says what the join box offers: to sign in, to link the account to a TTA username, to ask to play, that the request waits or was refused, or the questions. */
export function joinState(signedIn: boolean, linked: boolean, status: 'none' | 'requested' | 'accepted' | 'refused'): JoinState {
  if (!signedIn) return 'sign-in';
  if (!linked) return 'link';
  switch (status) {
    case 'requested':
      return 'requested';
    case 'refused':
      return 'refused';
    case 'accepted':
      return 'play';
  }
  return 'ask';
}

type Question = { id: number; kind: 'pick' | 'set' | 'number'; picks: number };
type Given = { question: number; answer: string[]; tokens: number };

/** Draft is a player's answers to a round as they fill them in, by question, with the tokens staked on each. */
export type Draft = Record<number, { answer: string[]; tokens: number }>;

/** draftFrom starts a round's form from the player's saved answers, empty where they gave none. */
export function draftFrom(questions: readonly Question[], saved: readonly Given[]): Draft {
  const draft: Draft = {};
  for (const q of questions) {
    const given = saved.find((g) => g.question === q.id);
    draft[q.id] = { answer: given ? [...given.answer] : [], tokens: given?.tokens ?? 0 };
  }
  return draft;
}

/** choose answers q with value: a pick takes it instead, a set adds it (or drops it when chosen again) up to its picks, a number takes what is typed. */
export function choose(draft: Draft, q: Question, value: string): Draft {
  const now = draft[q.id] ?? { answer: [], tokens: 0 };
  let answer: string[];
  if (q.kind === 'set') {
    if (now.answer.includes(value)) answer = now.answer.filter((v) => v !== value);
    else answer = now.answer.length < q.picks ? [...now.answer, value] : now.answer;
  } else {
    answer = value.trim() === '' ? [] : [value.trim()];
  }
  return { ...draft, [q.id]: { ...now, answer } };
}

/** staked is how many tokens the form stakes in all. */
export function staked(draft: Draft): number {
  return Object.values(draft).reduce((n, a) => n + a.tokens, 0);
}

/** MOST_ON_ONE is the most tokens one answer takes, as the database allows, when the round sets no most. */
const MOST_ON_ONE = 99;

/** stakeRange is how many tokens question's answer may take: from none to what is on it plus what is left of the round's most (null for none set). */
export function stakeRange(draft: Draft, question: number, most: number | null): { min: number; max: number } {
  const on = draft[question]?.tokens ?? 0;
  return { min: 0, max: most === null ? MOST_ON_ONE : Math.min(MOST_ON_ONE, on + most - staked(draft)) };
}

/** withStake stakes tokens on question's answer, kept within stakeRange. */
export function withStake(draft: Draft, question: number, tokens: number, most: number | null): Draft {
  const { min, max } = stakeRange(draft, question, most);
  const now = draft[question] ?? { answer: [], tokens: 0 };
  return { ...draft, [question]: { ...now, tokens: Math.max(min, Math.min(max, Math.trunc(tokens))) } };
}

/** tokensShort is how many more tokens the round's least asks for (null for none set), 0 once met. */
export function tokensShort(draft: Draft, least: number | null): number {
  return least === null ? 0 : Math.max(0, least - staked(draft));
}

/** givenFrom is what the form sends: each answer complete (a set with all its picks), in question order; one half done or empty is left out. */
export function givenFrom(questions: readonly Question[], draft: Draft): Given[] {
  return questions.flatMap((q) => {
    const a = draft[q.id];
    if (!a || a.answer.length === 0 || (q.kind === 'set' && a.answer.length !== q.picks)) return [];
    return [{ question: q.id, answer: [...a.answer], tokens: a.tokens }];
  });
}

/** pollShare is how many answers in a closed round named label, ignoring case, and their share of all answers to the question, in whole percent. */
export function pollShare(q: { counts: readonly { value: string; answers: number }[] }, label: string): { answers: number; share: number } {
  const total = q.counts.reduce((n, c) => n + c.answers, 0);
  const answers = q.counts.find((c) => c.value.toLowerCase() === label.toLowerCase())?.answers ?? 0;
  return { answers, share: total === 0 ? 0 : Math.round((answers * 100) / total) };
}

/** timeLeft says how long until a deadline: days and hours, hours and minutes, or minutes; "closed" once passed. */
export function timeLeft(closes: string, now: Date): string {
  const minutes = Math.floor((new Date(closes).getTime() - now.getTime()) / 60000);
  if (minutes <= 0) return 'closed';
  const count = (n: number, word: string) => (n === 0 ? '' : `${n} ${word}${n === 1 ? '' : 's'}`);
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  const parts = days > 0 ? [count(days, 'day'), count(hours, 'hour')] : [count(hours, 'hour'), count(minutes % 60, 'minute')];
  return `${parts.filter((p) => p !== '').join(' ')} left`;
}

/** answeredOf counts how many of a round's questions the player has answered. */
export function answeredOf(questions: readonly { id: number }[], mine: readonly { question: number }[]): { answered: number; of: number } {
  return { answered: questions.filter((q) => mine.some((a) => a.question === q.id)).length, of: questions.length };
}

/** editionLabel names a game in the game picker, saying when it has a round open and how long is left. */
export function editionLabel(e: { name: string; open_until?: string | null }, now: Date): string {
  return e.open_until == null ? e.name : `${e.name} · round open, ${timeLeft(e.open_until, now)}`;
}
