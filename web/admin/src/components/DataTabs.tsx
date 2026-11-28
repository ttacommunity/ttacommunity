/** The Data tab: one card of sub tabs (summary, runs, games, standings check), the one shown named by the address. */
import type { ReactNode } from 'react';
import SubTabs, { useHash } from '@ttac/shared/SubTabs';
import { dataTabFrom, dataTabs, type Panel, type Role } from '../lib/admin-tabs';
import AdminCard, { InTabs } from './AdminCard';

interface Props {
  role: Role;
  /** render draws one of the panels. */
  render: (p: Panel) => ReactNode;
}

export default function DataTabs({ role, render }: Props) {
  const [hash, setHash] = useHash();
  const tabs = dataTabs(role);
  return (
    <AdminCard label="Data">
      <InTabs>
        <SubTabs<Panel>
          label="Data"
          tabs={tabs.map((t) => ({ value: t.panel, label: t.label, panel: render(t.panel) }))}
          chosen={dataTabFrom(hash, role)}
          onChoose={(p) => setHash(tabs.find((t) => t.panel === p)?.hash ?? 'data')}
        />
      </InTabs>
    </AdminCard>
  );
}
