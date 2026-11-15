/** AdminPage is the frame of every admin page: the header with the tabs the signed-in admin may see, what failed to load, who is signed in, then the page. */
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
  error: string | null;
  children: ReactNode;
}

export default function AdminPage({ me, current, counts, error, children }: Props) {
  return (
    <>
      <AdminHeader tabs={me.kind === 'ready' ? tabsFor(me.data.role) : []} current={current} counts={counts} />
      <main className="admin-main">
        {error !== null && <PageError message={error} />}
        {me.kind === 'ready' && (
          <p className="signed-in">
            Signed in as <strong>{me.data.name}</strong> ({me.data.role})
          </p>
        )}
        {children}
      </main>
    </>
  );
}
