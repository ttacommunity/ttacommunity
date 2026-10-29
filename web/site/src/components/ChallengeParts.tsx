/** The pieces every challenge page shares: its remembered switches, links to players and proofs, difficulty pips and the compact set grid. */
import { useState, type ReactNode } from 'react';
import Segmented from '@ttac/shared/Segmented';
import { recallChoice, rememberChoice, type Choice, type ChoiceValue } from '../lib/challenge-view';
import { challengeHref, challengePicture, challengePlayerHref, groupLabel, type ChallengeGroup, type Measure } from '../lib/challenges';

function storage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

/** useChoice holds a remembered switch: its value, and a function that sets and remembers it. */
export function useChoice<C extends Choice>(choice: C): [ChoiceValue<C>, (value: ChoiceValue<C>) => void] {
  const [value, setValue] = useState(() => recallChoice(storage(), choice));
  return [
    value,
    (v) => {
      setValue(v);
      rememberChoice(storage(), choice, v);
    },
  ];
}

/** VerifiedSwitch chooses between the official records (entries with a Discord proof) and every entry. */
export function VerifiedSwitch({ value, onChange }: { value: ChoiceValue<'verified'>; onChange: (v: ChoiceValue<'verified'>) => void }) {
  return (
    <Segmented
      label="Which entries count"
      look="raised"
      value={value}
      onChange={onChange}
      options={[
        { value: 'verified', label: 'Verified only', title: 'Only entries with a linked Discord proof' },
        { value: 'all', label: 'All entries', title: 'Every entry, with or without proof' },
      ]}
    />
  );
}

/** MeasureSwitch chooses how players compare: rating or total domination. */
export function MeasureSwitch({ value, onChange }: { value: Measure; onChange: (m: Measure) => void }) {
  return (
    <Segmented
      label="Compare players by"
      look="raised"
      value={value}
      onChange={onChange}
      options={[
        { value: 'rating', label: 'Rating', title: 'Each best as a % of the record, averaged over all challenges' },
        { value: 'total', label: 'Total', title: "Sum of each player's best dominations" },
      ]}
    />
  );
}

/** ChallengePlayerLink links to a player's challenges page. */
export function ChallengePlayerLink({ name }: { name: string }) {
  return (
    <a className="player-link" href={challengePlayerHref(name)}>
      {name}
    </a>
  );
}

/** ProofLink opens an entry's Discord proof, or says it has none. */
export function ProofLink({ url }: { url: string | null }) {
  if (url === null) {
    return (
      <span className="challenge-unverified" title="No proof linked">
        unverified
      </span>
    );
  }
  return (
    <a className="challenge-proof" href={url} target="_blank" rel="noopener noreferrer" title="Open the proof in Discord">
      ✓ proof
    </a>
  );
}

/** ProofMark is a small tick opening a proof, or nothing without one. */
export function ProofMark({ url }: { url: string | null }) {
  if (url === null) return null;
  return (
    <a className="challenge-proof-mark" href={url} target="_blank" rel="noopener noreferrer" title="Open the proof in Discord" aria-label="Proof">
      ✓
    </a>
  );
}

/** BugTag marks an entry made through a since-fixed game bug. */
export function BugTag() {
  return (
    <span className="challenge-bug" title="Made through a game bug that has since been fixed. Listed, but never counted.">
      bug
    </span>
  );
}

/** Difficulty is the game's difficulty as filled pips out of six. */
export function Difficulty({ value }: { value: number }) {
  return (
    <span className="challenge-difficulty" role="img" aria-label={`Difficulty ${value} of 6`} title={`Difficulty ${value} of 6`}>
      {Array.from({ length: 6 }, (_, i) => (
        <span key={i} className={i < value ? 'pip on' : 'pip'} />
      ))}
    </span>
  );
}

interface Gridded {
  id: string;
  name: string;
}

/** SetGrid lays out a set's challenges as the game does, a row per group; cell fills each challenge, label adds under a group's name. */
export function SetGrid<C extends Gridded>({
  groups,
  cell,
  label,
}: {
  groups: ChallengeGroup<C>[];
  cell: (c: C) => ReactNode;
  label?: (challenges: C[], group: string) => ReactNode;
}) {
  return (
    <div className="challenge-grid">
      {groups.map((g) => (
        <div key={g.id} className="challenge-grid-row">
          <div className="challenge-grid-label">
            {groupLabel(g.id)}
            {label && <div className="challenge-grid-extra">{label(g.challenges, g.id)}</div>}
          </div>
          {g.challenges.map((c) => (
            <div key={c.id} className="challenge-grid-cell">
              <a className="challenge-grid-name" href={challengeHref(c.id)} title={c.name}>
                <img src={challengePicture(c.id)} alt="" loading="lazy" width={34} height={28} />
                <span>{c.name}</span>
              </a>
              <div className="challenge-grid-value">{cell(c)}</div>
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}
