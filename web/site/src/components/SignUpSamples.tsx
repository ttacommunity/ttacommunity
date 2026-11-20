/** The sign-up box on /design, in each state a player sees it in, with buttons that do nothing. */
import { SignUpPanel } from './SignUpBox';
import type { SignUpBox } from '../lib/sign-up';

const nothing = async () => null;

const SAMPLES: { box: SignUpBox; count: string; closes: string | null }[] = [
  { box: { kind: 'sign-in' }, count: 'No one has signed up yet.', closes: 'Sign-ups close when round 1 is paired.' },
  { box: { kind: 'enter' }, count: '12 players entered.', closes: 'Sign-ups close on 1 Dec 2026, 18:00 (Australia/Sydney), or when round 1 is paired.' },
  { box: { kind: 'give-name' }, count: '12 players entered, and 1 waiting for their account to be linked.', closes: 'Sign-ups close when round 1 is paired.' },
  { box: { kind: 'entered' }, count: '13 players entered.', closes: 'Sign-ups close when round 1 is paired.' },
  { box: { kind: 'pending', name: 'Newcomer', canWithdraw: true }, count: '12 players entered, and 1 waiting for their account to be linked.', closes: 'Sign-ups close when round 1 is paired.' },
];

export default function SignUpSamples() {
  return (
    <>
      {SAMPLES.map((s) => (
        <SignUpPanel key={s.box.kind} box={s.box} count={s.count} closes={s.closes} onEnter={nothing} onGiveName={nothing} onWithdraw={nothing} players={[]} onSearch={() => undefined} />
      ))}
    </>
  );
}
