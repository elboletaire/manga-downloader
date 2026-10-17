import { useEffect, useRef, useState } from 'react';
import { errorMessage } from './format';
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
 * arrived first: whichever is newer wins. `initial` is a state the caller has
 * already read, so the window doesn't ask for it again.
 */
export function useWindow(api: Api, initial?: State): WindowData & { failure: string | null } {
  const [data, setData] = useState<WindowData>({ state: initial ?? null, chapters: [], selected: [], log: [] });
  // the error's text; the window words it in the language shown when it renders
  const [failure, setFailure] = useState<string | null>(null);
  // which fields a push has already filled, so a late snapshot doesn't clobber them
  const pushed = useRef({ state: false, chapters: false, selected: false, log: false });

  // a newer State from the caller: it saw pushes that came before this window subscribed
  useEffect(() => {
    if (initial !== undefined) {
      setData((d) => ({ ...d, state: initial }));
    }
  }, [initial]);

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

    Promise.all([initial ?? api.state(), api.chapters(), api.selection(), api.log()])
      .then(([state, chapters, selected, log]) => {
        if (!alive) {
          return;
        }
        setData((d) => ({
          state: seen.state ? d.state : state,
          chapters: seen.chapters ? d.chapters : chapters,
          selected: seen.selected ? d.selected : selected,
          log: seen.log ? d.log : log,
        }));
      })
      .catch((e: unknown) => {
        // without a snapshot the window has nothing to show: say so instead of "Loading…"
        if (alive) {
          setFailure(errorMessage(e));
        }
      });

    return () => {
      alive = false;
      for (const off of offs) {
        off();
      }
    };
  }, [api]);

  return { ...data, failure };
}
