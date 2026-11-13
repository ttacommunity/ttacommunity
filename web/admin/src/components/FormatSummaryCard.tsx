/** A tournament's format in brief, which its group's Hall of Fame page lists as its format and regulations. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournament } from '../lib/admin-api';
import SaveBar from './SaveBar';
import { useSave } from './useSave';

/** FormatSummaryCard edits the line the group's page shows after the tournament's name. */
export default function FormatSummaryCard({ t, group, onReload }: { t: AdminTournament; group: string; onReload: () => Promise<void> }) {
  const [text, setText] = useState(t.format_summary);
  const saving = useSave();
  const save = async () => {
    const r = await saving.run(() => req.send(req.setFormatSummary(t.slug, text)), { done: 'Saved. The site shows it at once.' });
    if (r?.ok) await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="format-summary-heading">
      <h2 id="format-summary-heading" className="rating-card-heading">
        Format in Brief
      </h2>
      <p className="muted" id="format-summary-hint">
        One line on its format. The {group} page lists it in its Format &amp; Regulations box, after this tournament&apos;s name.
        Leave it empty to leave this tournament out of the box.
      </p>
      <input
        className="input-antique format-summary"
        aria-labelledby="format-summary-heading"
        aria-describedby="format-summary-hint"
        value={text}
        maxLength={300}
        onChange={(e) => setText(e.target.value)}
      />
      <SaveBar save={saving}>
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={text.trim() === t.format_summary}>
          Save
        </button>
      </SaveBar>
    </section>
  );
}
