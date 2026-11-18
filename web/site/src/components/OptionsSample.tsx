/** The design page's options drawer: the button that opens it, and a drawer with a choice, a field, and Reset All. */
import { useState } from 'react';
import Segmented from '@ttac/shared/Segmented';
import { OptionsDivider, OptionsPanel, OptionsSection, OptionsToggle } from './SideDrawer';

/** OptionsSample opens a sample drawer whose choices change only the sample. */
export default function OptionsSample() {
  const [open, setOpen] = useState(false);
  const [format, setFormat] = useState(0);
  return (
    <div>
      <OptionsToggle title="Open the sample options" active={format !== 0} open={open} onClick={() => setOpen(!open)} />
      {open && (
        <OptionsPanel label="Sample options" onClose={() => setOpen(false)} reset={() => setFormat(0)} summary={<>Active: <strong>Sample</strong></>}>
          <OptionsSection icon="🎮" title="Format" tip="A sample choice">
            <Segmented label="Format" options={[0, 2, 3, 4].map((f) => ({ value: f, label: f === 0 ? 'All Formats' : `${f}-Player` }))} value={format} onChange={setFormat} />
          </OptionsSection>
          <OptionsDivider>A part of the drawer</OptionsDivider>
          <OptionsSection icon="⚔️" title="Sample field" htmlFor="sample-field">
            <input id="sample-field" className="input-antique" defaultValue="30" />
          </OptionsSection>
        </OptionsPanel>
      )}
    </div>
  );
}
