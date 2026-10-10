import { useEffect, useRef, useState } from 'react';
import type { Api } from './api';
import type { Chapter, LogLine, State } from './types';

export interface WindowData {
  state: State | null;
  chapters: Chapter[];
  /** Sorted indices of the selected chapters */
  selected: number[];
  log: LogLine[];
}

/**
 * Keeps the window's data current. It subscribes before asking for snapshots,
 * so nothing is missed, and a snapshot never overwrites a pushed value that
 * arrived first: whichever is newer wins.
 */
export function useWindow(api: Api): WindowData {
  const [data, setData] = useState<WindowData>({ state: null, chapters: [], selected: [], log: [] });
  // which fields a push has already filled, so a late snapshot doesn't clobber them
  const pushed = useRef({ state: false, chapters: false, selected: false, log: false });

  useEffect(() => {
    let alive = true;
    const seen = pushed.current;

    const offs = [
      api.onState((state) => {
        seen.state = true;
        setData((d) => ({ ...d, state }));
      }),
      api.onChapters((chapters) => {
        seen.chapters = true;
        setData((d) => ({ ...d, chapters }));
      }),
      api.onSelection((selected) => {
        seen.selected = true;
        setData((d) => ({ ...d, selected }));
      }),
      api.onLog((log) => {
        seen.log = true;
        setData((d) => ({ ...d, log }));
      }),
    ];

    Promise.all([api.state(), api.chapters(), api.selection(), api.log()]).then(([state, chapters, selected, log]) => {
      if (!alive) {
        return;
      }
      setData((d) => ({
        state: seen.state ? d.state : state,
        chapters: seen.chapters ? d.chapters : chapters,
        selected: seen.selected ? d.selected : selected,
        log: seen.log ? d.log : log,
      }));
    });

    return () => {
      alive = false;
      for (const off of offs) {
        off();
      }
    };
  }, [api]);

  return data;
}
