/** The sign-up box any tournament page places: how many have signed up and when sign-ups close, then signing in, signing up (with a TTA username when the account is not linked) or withdrawing. It shows itself only while sign-ups take entries, or to a player whose sign-up waits for a link. */
import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { viewerZone } from '@ttac/shared/time';
import { browserStorage } from '@ttac/shared/storage';
import Note from '@ttac/shared/Note';
import OutcomeNotice from '@ttac/shared/OutcomeNotice';
import { seemsSignedIn, type Done } from '../lib/account';
import { closesLine, countLine, fetchMySignUp, signUp, signUpBox, withdraw, type MySignUp, type SignUpBox as Box, type SignUps } from '../lib/sign-up';
import Panel from './Panel';
import PlayerSearch, { type SearchPlayer } from './PlayerSearch';
import SignInNote from './SignInNote';
import usePlayerSearch from './usePlayerSearch';
import './SignUpBox.css';

type Props = {
  apiUrl: string;
  /** The tournament it signs up for: its slug and edition. */
  tournament: string;
  season: string;
  signUps: SignUps;
  /** How many have entered so far. */
  entered: number;
  /** Called after the player signs up or withdraws, so the page reads the tournament again. */
  onChanged: () => void;
};

/** SignUpBox reads the viewer's sign-up for the tournament and shows the box for it. */
export default function SignUpBox({ apiUrl, tournament, season, signUps, entered, onChanged }: Props) {
  const [mine, setMine] = useState<MySignUp | null | undefined>(undefined);
  const [searching, setSearching] = useState(false);
  const players = usePlayerSearch(apiUrl, searching);
  const load = useCallback(async () => {
    setMine(seemsSignedIn(browserStorage()) ? await fetchMySignUp(apiUrl, tournament, season) : null);
  }, [apiUrl, tournament, season]);
  useEffect(() => {
    load().catch(() => setMine(null));
  }, [load]);
  const after = async (done: Done) => {
    if (!done.ok) return done.message;
    await load();
    onChanged();
    return null;
  };
  return (
    <SignUpPanel
      box={signUpBox(signUps, mine)}
      count={countLine(entered, signUps.waiting)}
      closes={signUps.taking ? closesLine(signUps, viewerZone()) : null}
      onEnter={async () => after(await signUp(apiUrl, tournament, season, null))}
      onGiveName={async (name) => after(await signUp(apiUrl, tournament, season, name))}
      onWithdraw={async () => after(await withdraw(apiUrl, tournament, season))}
      players={players}
      onSearch={() => setSearching(true)}
    />
  );
}

type PanelProps = {
  box: Box;
  count: string;
  closes: string | null;
  /** Each answers null when done, or why it was refused. */
  onEnter: () => Promise<string | null>;
  onGiveName: (name: string) => Promise<string | null>;
  onWithdraw: () => Promise<string | null>;
  /** The players the username field suggests, and the call asking for them once someone uses it. */
  players: readonly SearchPlayer[];
  onSearch: () => void;
};

/** SignUpPanel is the box as box says, with what to do in it; the page's SignUpBox gives it the viewer's sign-up. */
export function SignUpPanel({ box, count, closes, onEnter, onGiveName, onWithdraw, players, onSearch }: PanelProps) {
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<string | null>(null);
  if (box.kind === 'hidden') return null;
  const act = async (go: () => Promise<string | null>) => {
    setBusy(true);
    setRefused(await go());
    setBusy(false);
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    void act(() => onGiveName(name));
  };
  return (
    <Panel icon="✍️" title="Sign up" subtitle={count}>
      {closes !== null && <p className="sign-up-closes">{closes}</p>}
      {box.kind === 'sign-in' && <SignInNote lead="Want to play?" to="sign up." />}
      {box.kind === 'enter' && (
        <button type="button" className="btn-antique" disabled={busy} onClick={() => void act(onEnter)}>
          Sign up
        </button>
      )}
      {box.kind === 'give-name' && (
        <form className="sign-up-form" onSubmit={submit}>
          <label htmlFor="sign-up-name">Your TTA username, as in the app</label>
          <div className="sign-up-row">
            <PlayerSearch id="sign-up-name" players={players} value={name} onChange={setName} onFocus={onSearch} />
            <button type="submit" className="btn-antique" disabled={busy}>
              Sign up
            </button>
          </div>
          <Note>Your account is not linked to a TTA username yet. Signing up asks the organisers to link it: you are entered once they do.</Note>
        </form>
      )}
      {box.kind === 'entered' && (
        <p className="sign-up-row">
          <span>You are signed up.</span>
          <button type="button" className="btn-antique" disabled={busy} onClick={() => void act(onWithdraw)}>
            Withdraw
          </button>
        </p>
      )}
      {box.kind === 'pending' && (
        <p className="sign-up-row">
          <span>
            {box.canWithdraw
              ? `You signed up as ${box.name}. You are entered once the organisers link your account to that username.`
              : `Sign-ups have closed. You signed up as ${box.name}, and the organisers decide whether you are entered once they link your account.`}
          </span>
          {box.canWithdraw && (
            <button type="button" className="btn-antique" disabled={busy} onClick={() => void act(onWithdraw)}>
              Withdraw
            </button>
          )}
        </p>
      )}
      {refused !== null && <OutcomeNotice kind="refused" text={refused} onClose={() => setRefused(null)} />}
    </Panel>
  );
}
