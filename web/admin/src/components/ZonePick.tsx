/** A member's time zone picked as you type: Unknown, a region, or any IANA zone. */
import { useState } from 'react';
import { knownZones, zoneChoices, zoneFromText, type Region } from '../lib/zones';
import PickBox from './PickBox';

/** ZONE_CHOICES is how many matches the picker shows once something is typed. */
const ZONE_CHOICES = 8;

type Props = { label: string; regions: readonly Region[]; value: string; onChange: (zone: string) => void; disabled?: boolean; hideLabel?: boolean };

/** ZonePick shows the zone; focused, it offers Unknown and the regions, then what the text typed matches. Text naming a zone in full is taken when focus leaves; anything else goes back to the zone. */
export default function ZonePick({ label, regions, value, onChange, disabled, hideLabel }: Props) {
  const [known] = useState(knownZones);
  // typed is the text while it is being changed, null while the field shows the zone.
  const [typed, setTyped] = useState<string | null>(null);
  const options = zoneChoices(regions, known, typed ?? '', ZONE_CHOICES).map((c) => ({ key: c.value, label: c.value, detail: c.detail }));
  return (
    <PickBox
      label={label}
      text={typed ?? value}
      options={options}
      disabled={disabled}
      hideLabel={hideLabel}
      onText={setTyped}
      onPick={(o) => {
        setTyped(null);
        onChange(o.key);
      }}
      onLeave={(text) => {
        const zone = zoneFromText(regions, known, text);
        if (zone !== null && zone !== value) onChange(zone);
        setTyped(null);
      }}
    />
  );
}
