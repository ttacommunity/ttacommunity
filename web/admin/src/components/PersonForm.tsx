/** The person form, for adding a member or entering someone in an edition: a search of the members and the players the ratings know, picking one filling in their details, or someone new added by the name typed. */
import { useState, type FormEvent, type ReactNode } from 'react';
import type { Country } from '../lib/countries';
import { emptyPerson, personChoices, type PersonChoice, type PersonDetails, type Taken } from '../lib/person-form';
import type { Region } from '../lib/zones';
import CountryPick from './CountryPick';
import PickBox from './PickBox';
import usePeopleSearch from './usePeopleSearch';
import ZonePick from './ZonePick';

type Props = {
  /** searchBase is where the search asks, ?q= added: an edition's candidates, or everyone. */
  searchBase: string;
  taken: Taken;
  regions: readonly Region[];
  countries: readonly Country[];
  submitLabel: string;
  /** onSubmit sends the details and says whether they were taken, so the form empties. */
  onSubmit: (details: PersonDetails) => Promise<boolean>;
  /** refresh changes when the search should ask again, as when someone was just added. */
  refresh: unknown;
  /** children are more fields, shown before the button. */
  children?: ReactNode;
};

/** PersonForm searches as usePeopleSearch does; the details stay off until someone is picked. */
export default function PersonForm({ searchBase, taken, regions, countries, submitLabel, onSubmit, refresh, children }: Props) {
  const [typed, setTyped] = useState('');
  const [picked, setPicked] = useState<PersonChoice | null>(null);
  const [details, setDetails] = useState<PersonDetails>(emptyPerson);
  const found = usePeopleSearch(searchBase, typed, picked !== null, refresh);
  const choices = picked === null ? personChoices(found, typed, taken) : [];
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (picked === null || !(await onSubmit(details))) return;
    setPicked(null);
    setTyped('');
    setDetails(emptyPerson);
  };
  return (
    <form className="admin-add" onSubmit={(e) => void submit(e)}>
      <PickBox
        label="Player"
        text={typed}
        placeholder="Type a name"
        options={choices.map((c) => ({ key: c.key, label: c.label, detail: c.detail, disabled: c.disabled }))}
        onText={(text) => {
          setTyped(text);
          if (picked !== null) {
            setPicked(null);
            setDetails(emptyPerson);
          }
        }}
        onPick={(o) => {
          const c = choices.find((x) => x.key === o.key);
          if (c === undefined) return;
          setPicked(c);
          setTyped(c.fill.name);
          setDetails(c.fill);
        }}
      />
      {picked !== null && <span className="muted pick-note">{picked.detail}</span>}
      <label>
        Discord name
        <input className="input-antique" value={details.discord_name} maxLength={100} disabled={picked === null} onChange={(e) => setDetails({ ...details, discord_name: e.target.value })} />
      </label>
      <label>
        Email
        <input className="input-antique" type="email" value={details.email} maxLength={254} disabled={picked === null} onChange={(e) => setDetails({ ...details, email: e.target.value })} />
      </label>
      <CountryPick label="Country" countries={countries} value={details.country} disabled={picked === null} onChange={(country) => setDetails({ ...details, country })} />
      <ZonePick label="Time zone" regions={regions} value={details.zone} disabled={picked === null} onChange={(zone) => setDetails({ ...details, zone })} />
      {children}
      <button type="submit" className="btn-antique" disabled={picked === null}>
        {submitLabel}
      </button>
    </form>
  );
}
