/** Options & filters drawer for the Movies page: which creators' videos it shows, each with how many the search finds. */
import { count } from '@ttac/shared/numbers';
import ToggleGroup from '@ttac/shared/ToggleGroup';
import { everyVideo, type MovieCreator } from '../lib/movies';
import { OptionsPanel, OptionsSection } from './SideDrawer';

interface Props {
  creators: readonly MovieCreator[];
  chosen: ReadonlySet<string>;
  /** How many videos the choices show. */
  total: number;
  setChosen: (chosen: Set<string>) => void;
  onReset: () => void;
  onClose: () => void;
}

/** MoviesOptionsDrawer chooses the creators the Movies page shows. */
export default function MoviesOptionsDrawer({ creators, chosen, total, setChosen, onReset, onClose }: Props) {
  return (
    <OptionsPanel
      label="Movies options"
      onClose={onClose}
      reset={onReset}
      summary={
        <>
          Videos: <strong>{count(total)}</strong>
        </>
      }
    >
      <OptionsSection icon="🎬" title="Creator">
        <ToggleGroup
          label="Creator"
          all={`All creators (${count(everyVideo(creators))})`}
          noneMeansAll
          options={creators.map((c) => ({ value: c.channel_id, label: `${c.name} (${count(c.videos)})` }))}
          chosen={chosen}
          onChange={setChosen}
        />
      </OptionsSection>
    </OptionsPanel>
  );
}
