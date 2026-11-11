/** Nostradamus, for admins and organisers: an edition's questions, each outcome set as it becomes known, and its participants linked to members. */
import { useEffect, useState } from 'react';
import type { components } from '@ttac/shared/api';
import type { Load } from '@ttac/shared/load';
import { adminRequest } from '../lib/admin-api';
import { memberPath, outcomePath, outcomeText, outcomeValues } from '../lib/nostradamus-admin';
import { outcomeOf, shownAt, type Outcome } from '../lib/outcome';
import Notice, { LoadNotice } from './Notice';

type Editions = components['schemas']['NostradamusEditions'];
type Edition = components['schemas']['AdminNostradamus'];
type Question = components['schemas']['AdminNostradamusQuestion'];
type Participant = components['schemas']['AdminNostradamusParticipant'];

/** NostradamusPanel loads the editions, then the one chosen (the newest first). */
export default function NostradamusPanel() {
  const [editions, setEditions] = useState<Load<Editions>>({ kind: 'loading' });
  const [chosen, setChosen] = useState('');
  const [edition, setEdition] = useState<Load<Edition>>({ kind: 'loading' });
  const [outcome, setOutcome] = useState<Outcome | null>(null);

  useEffect(() => {
    void adminRequest<Editions>('/api/nostradamus', {}).then((r) => {
      setEditions(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
      if (r.ok) setChosen(r.data.editions[0]?.slug ?? '');
    });
  }, []);
  const reload = async () => {
    if (chosen === '') return;
    const r = await adminRequest<Edition>(`/api/nostradamus/${encodeURIComponent(chosen)}`, {});
    setEdition(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    setEdition({ kind: 'loading' });
    void reload();
  }, [chosen]);

  if (editions.kind === 'error') return <LoadNotice message={editions.message} />;
  if (editions.kind === 'loading') return <p className="muted">Loading the editions…</p>;
  if (editions.data.editions.length === 0) return <p className="muted">No Nostradamus edition yet.</p>;
  return (
    <section className="rating-card admin-card" aria-labelledby="nostradamus-heading">
      <h2 id="nostradamus-heading" className="rating-card-heading">
        Nostradamus
      </h2>
      <label>
        Edition{' '}
        <select className="input-antique" value={chosen} onChange={(e) => setChosen(e.target.value)}>
          {editions.data.editions.map((e) => (
            <option key={e.slug} value={e.slug}>
              {e.name}
            </option>
          ))}
        </select>
      </label>
      <Notice outcome={shownAt(outcome, 'nostradamus')} onClose={() => setOutcome(null)} />
      {edition.kind === 'error' && <LoadNotice message={edition.message} />}
      {edition.kind === 'ready' && (
        <>
          <p className="muted">
            An outcome scores every answer to its question. Names are separated by commas: a pick lists everyone who tied, a set every name. Leave it empty
            and save to clear it.
          </p>
          {edition.data.rounds.map((r) => (
            <div key={r.round} className="admin-subsection">
              <h3>{r.name}</h3>
              <div className="table-responsive">
                <table className="leaderboard-table admin-table">
                  <caption className="visually-hidden">{r.name}: each question and its outcome</caption>
                  <thead>
                    <tr>
                      <th scope="col">Question</th>
                      <th scope="col">Kind</th>
                      <th scope="col" className="num">
                        Points
                      </th>
                      <th scope="col">Outcome</th>
                    </tr>
                  </thead>
                  <tbody>
                    {r.questions.map((q) => (
                      <QuestionRow key={q.id} edition={chosen} round={r.round} question={q} say={setOutcome} onSaved={reload} />
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
          <div className="admin-subsection">
            <h3>Participants</h3>
            <p className="muted">Each is linked to the member with the TTA username it answered under, where there is one. Link the others by hand.</p>
            <div className="table-responsive">
              <table className="leaderboard-table admin-table">
                <caption className="visually-hidden">Each participant and the member it is linked to</caption>
                <thead>
                  <tr>
                    <th scope="col">Answered as</th>
                    <th scope="col">Member</th>
                  </tr>
                </thead>
                <tbody>
                  {edition.data.participants.map((p) => (
                    <ParticipantRow key={p.id} edition={chosen} participant={p} say={setOutcome} onSaved={reload} />
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}
    </section>
  );
}

/** QuestionRow is one question with its outcome to set. */
function QuestionRow({
  edition,
  round,
  question,
  say,
  onSaved,
}: {
  edition: string;
  round: string;
  question: Question;
  say: (o: Outcome) => void;
  onSaved: () => Promise<void>;
}) {
  const q = question.question;
  const [text, setText] = useState(outcomeText(q.outcome));
  const save = async () => {
    const r = await adminRequest(outcomePath(edition, question.id), { method: 'PUT', json: { outcome: outcomeValues(q.kind, text) } });
    say(outcomeOf(r, `Saved the outcome of ${round} ${q.label}.`, 'nostradamus'));
    if (r.ok) await onSaved();
  };
  return (
    <tr>
      <td>{q.prompt || q.label}</td>
      <td className="muted">{q.kind}</td>
      <td className="num">{q.near_points > 0 ? `${q.points} (${q.near_points} one off)` : q.points}</td>
      <td>
        <input className="input-antique" aria-label={`Outcome: ${round} ${q.label}`} value={text} maxLength={500} onChange={(e) => setText(e.target.value)} />{' '}
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={text === outcomeText(q.outcome)}>
          Save
        </button>
      </td>
    </tr>
  );
}

/** ParticipantRow is one participant with the member to link it to. */
function ParticipantRow({ edition, participant, say, onSaved }: { edition: string; participant: Participant; say: (o: Outcome) => void; onSaved: () => Promise<void> }) {
  const [member, setMember] = useState(participant.member ?? '');
  const save = async () => {
    const r = await adminRequest(memberPath(edition, participant.id), { method: 'PUT', json: { member } });
    say(outcomeOf(r, member.trim() === '' ? `Unlinked ${participant.name}.` : `Linked ${participant.name} to ${member.trim()}.`, 'nostradamus'));
    if (r.ok) await onSaved();
  };
  return (
    <tr>
      <td>
        {participant.name}
        {participant.bot && <span className="muted"> (bot)</span>}
      </td>
      <td>
        <input className="input-antique" aria-label={`Member: ${participant.name}`} value={member} maxLength={100} onChange={(e) => setMember(e.target.value)} />{' '}
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={member === (participant.member ?? '')}>
          Save
        </button>
      </td>
    </tr>
  );
}
