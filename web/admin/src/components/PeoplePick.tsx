/** A member or rated player picked from admin's people search as you type their name. */
import { useState } from 'react';
import type { Candidate } from '../lib/person-form';
import { linkChoices } from '../lib/person-form';
import PickBox, { type PickOption } from './PickBox';
import usePeopleSearch from './usePeopleSearch';

type Props = {
  label: string;
  /** value is the text in the field. */
  value: string;
  onChange: (text: string) => void;
  /** onPick acts on a pick instead of putting its name in the field, for a field that adds someone and clears. */
  onPick?: (option: PickOption) => void;
  /** choices turns what the search found into options; linkChoices (members and rated players) when left out. */
  choices?: (found: readonly Candidate[], typed: string) => readonly PickOption[];
  /** refresh changes when the search should ask again, such as after a save. */
  refresh?: unknown;
  placeholder?: string;
  disabled?: boolean;
  hideLabel?: boolean;
};

/** PeoplePick asks the people search once typing pauses, and not for the name already picked or first shown. */
export default function PeoplePick({ label, value, onChange, onPick, choices = linkChoices, refresh, placeholder = 'Search a TTA username', disabled, hideLabel }: Props) {
  const [picked, setPicked] = useState(value);
  const found = usePeopleSearch('/api/people', value, value === picked, refresh);
  return (
    <PickBox
      label={label}
      text={value}
      options={choices(found, value)}
      placeholder={placeholder}
      disabled={disabled}
      hideLabel={hideLabel}
      onText={onChange}
      onPick={(o) => {
        if (onPick !== undefined) {
          onPick(o);
          return;
        }
        setPicked(o.label);
        onChange(o.label);
      }}
    />
  );
}
