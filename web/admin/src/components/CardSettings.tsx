/** A tournament's card and page header. */
import { cardChanged } from '../lib/admin-edits';
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournament } from '../lib/admin-api';
import { refusalOf, type Outcome } from '../lib/outcome';
import Notice from './Notice';

/** CardSettings edits how the tournament presents itself on its card and its page's header. */
export default function CardSettings({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [card, setCard] = useState(t.card);
  const [status, setStatus] = useState('');
  const [refusal, setRefusal] = useState<Outcome | null>(null);
  const changed = cardChanged(t.card, card);
  const save = async () => {
    setStatus('Saving…');
    setRefusal(null);
    const r = await req.send(req.setCard(t.slug, card));
    setStatus(r.ok ? 'Saved. The cards and the tournament page show it at once.' : '');
    setRefusal(refusalOf(r));
    if (r.ok) await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="card-heading">
      <h2 id="card-heading" className="rating-card-heading">
        Card
      </h2>
      <p className="muted">
        How the tournament shows on the Hall of Fame and Every Tournament, and at the top of its own page. Leave a box empty
        to leave it out; with no icon it shows 📜.
      </p>
      <div className="card-fields">
        <label>
          Icon
          <input className="input-antique" id="card-icon" value={card.icon} maxLength={8} placeholder="📜" onChange={(e) => setCard({ ...card, icon: e.target.value })} />
        </label>
        <label className="card-tagline">
          Short description
          <input className="input-antique"
            id="card-tagline"
            value={card.tagline}
            maxLength={60}
            placeholder="e.g. Annual World Championship"
            onChange={(e) => setCard({ ...card, tagline: e.target.value })}
          />
        </label>
        <label className="card-tagline">
          Page subtitle
          <input className="input-antique"
            id="card-subtitle"
            value={card.subtitle}
            maxLength={60}
            placeholder="e.g. 4-Player Competitive Format"
            onChange={(e) => setCard({ ...card, subtitle: e.target.value })}
          />
        </label>
        <label className="card-tagline">
          Cadence
          <input className="input-antique"
            id="card-cadence"
            value={card.cadence}
            maxLength={60}
            placeholder="e.g. Annual (Every Year)"
            onChange={(e) => setCard({ ...card, cadence: e.target.value })}
          />
        </label>
        <label className="card-summary">
          Information box
          <textarea className="input-antique"
            id="card-summary"
            value={card.summary}
            maxLength={1000}
            rows={4}
            onChange={(e) => setCard({ ...card, summary: e.target.value })}
          />
        </label>
      </div>
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={!changed}>
          Save card
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
      <Notice outcome={refusal} onClose={() => setRefusal(null)} />
    </section>
  );
}
