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
type Given = { question: number; answer: string[]; token: boolean };

/** Draft is a player's answers to a round as they fill them in, by question. */
export type Draft = Record<number, { answer: string[]; token: boolean }>;

/** draftFrom starts a round's form from the player's saved answers, empty where they gave none. */
export function draftFrom(questions: readonly Question[], saved: readonly Given[]): Draft {
  const draft: Draft = {};
  for (const q of questions) {
    const given = saved.find((g) => g.question === q.id);
    draft[q.id] = { answer: given ? [...given.answer] : [], token: given?.token ?? false };
  }
  return draft;
}

/** choose answers q with value: a pick takes it instead, a set adds it (or drops it when chosen again) up to its picks, a number takes what is typed. */
export function choose(draft: Draft, q: Question, value: string): Draft {
  const now = draft[q.id] ?? { answer: [], token: false };
  let answer: string[];
  if (q.kind === 'set') {
    if (now.answer.includes(value)) answer = now.answer.filter((v) => v !== value);
    else answer = now.answer.length < q.picks ? [...now.answer, value] : now.answer;
  } else {
    answer = value.trim() === '' ? [] : [value.trim()];
  }
  return { ...draft, [q.id]: { ...now, answer } };
}

/** tokensLeft is how many of the round's tokens the form has not used. */
export function tokensLeft(draft: Draft, tokens: number): number {
  return tokens - Object.values(draft).filter((a) => a.token).length;
}

/** toggleToken puts a token on question's answer, or takes it off; none goes on once the round's tokens are used. */
export function toggleToken(draft: Draft, question: number, tokens: number): Draft {
  const now = draft[question] ?? { answer: [], token: false };
  if (!now.token && tokensLeft(draft, tokens) <= 0) return draft;
  return { ...draft, [question]: { ...now, token: !now.token } };
}

/** givenFrom is what the form sends: each answer complete (a set with all its picks), in question order; one half done or empty is left out. */
export function givenFrom(questions: readonly Question[], draft: Draft): Given[] {
  return questions.flatMap((q) => {
    const a = draft[q.id];
    if (!a || a.answer.length === 0 || (q.kind === 'set' && a.answer.length !== q.picks)) return [];
    return [{ question: q.id, answer: [...a.answer], token: a.token }];
  });
}

/** pollShare is how many answers in a closed round named label, ignoring case, and their share of all answers to the question, in whole percent. */
export function pollShare(q: { counts: readonly { value: string; answers: number }[] }, label: string): { answers: number; share: number } {
  const total = q.counts.reduce((n, c) => n + c.answers, 0);
  const answers = q.counts.find((c) => c.value.toLowerCase() === label.toLowerCase())?.answers ?? 0;
  return { answers, share: total === 0 ? 0 : Math.round((answers * 100) / total) };
}
