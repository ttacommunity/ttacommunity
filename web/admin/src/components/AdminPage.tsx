/** AdminPage is the frame of every admin page: the header with the tabs the signed-in admin may see and who is signed in, Loading… or what failed to load, then the page. */
import type { ReactNode } from 'react';
import type { Load } from '@ttac/shared/load';
import type { AdminMe } from '../lib/admin-api';
import { tabsFor, type TabKey } from '../lib/admin-tabs';
import AdminHeader from './AdminHeader';
import PageError from './PageError';

interface Props {
  me: Load<AdminMe>;
  current: TabKey | null;
  counts?: Partial<Record<TabKey, number>>;
  loading: boolean;
  error: string | null;
  children: ReactNode;
}

export default function AdminPage({ me, current, counts, loading, error, children }: Props) {
  return (
    <>
      <AdminHeader tabs={me.kind === 'ready' ? tabsFor(me.data.role) : []} current={current} counts={counts} who={me.kind === 'ready' ? { name: me.data.name, role: me.data.role } : undefined} />
      <main className="admin-main">
        {loading && <p className="muted">Loading…</p>}
        {error !== null && <PageError message={error} />}
        {children}
      </main>
    </>
  );
}
