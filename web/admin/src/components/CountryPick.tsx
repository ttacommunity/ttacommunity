/** A member's country picked as you type its name or code. */
import { useState } from 'react';
import { countryChoices, countryFromText, type Country } from '../lib/countries';
import PickBox from './PickBox';

/** COUNTRY_CHOICES is how many matches the picker shows. */
const COUNTRY_CHOICES = 8;

type Props = { label: string; countries: readonly Country[]; value: string; onChange: (code: string) => void; disabled?: boolean; hideLabel?: boolean };

/** CountryPick shows the country's name, or nothing for none. Text naming a country in full, by name or code, is taken when focus leaves, and no text means none; anything else goes back to the country. */
export default function CountryPick({ label, countries, value, onChange, disabled, hideLabel }: Props) {
  // typed is the text while it is being changed, null while the field shows the country.
  const [typed, setTyped] = useState<string | null>(null);
  const shown = countries.find((c) => c.code === value)?.name ?? value;
  const options = countryChoices(countries, typed ?? '', COUNTRY_CHOICES).map((c) => ({ key: c.code, label: c.name, detail: c.code }));
  return (
    <PickBox
      label={label}
      text={typed ?? shown}
      options={options}
      placeholder="none"
      disabled={disabled}
      hideLabel={hideLabel}
      onText={setTyped}
      onPick={(o) => {
        setTyped(null);
        onChange(o.key);
      }}
      onLeave={(text) => {
        const code = countryFromText(countries, text);
        if (code !== null && code !== value) onChange(code);
        setTyped(null);
      }}
    />
  );
}
