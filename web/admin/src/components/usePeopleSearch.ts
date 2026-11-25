/** The person search's asking: once typing pauses, the people the text finds, a reply to an older search dropped. */
import { useEffect, useState } from 'react';
import * as req from '../lib/admin-requests';
import type { Candidate } from '../lib/person-form';

/** usePeopleSearch asks searchBase (?q= added) for the text typed, 200 ms after the last key; none while paused or for under two characters. refresh changes when it should ask again. */
export default function usePeopleSearch(searchBase: string, typed: string, paused: boolean, refresh: unknown): Candidate[] {
  const [found, setFound] = useState<Candidate[]>([]);
  useEffect(() => {
    const q = typed.trim();
    if (paused || q.length < 2) {
      setFound([]);
      return;
    }
    let current = true;
    const timer = setTimeout(() => {
      void req.send<Candidate[]>(req.searchPeople(searchBase, q)).then((r) => {
        if (current) setFound(r.ok ? r.data : []);
      });
    }, 200);
    return () => {
      current = false;
      clearTimeout(timer);
    };
  }, [searchBase, typed, paused, refresh]);
  return found;
}
