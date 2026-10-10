import { useEffect, useMemo, useRef, useState } from 'react';
import type { Chapter, State } from './types';
import { errorMessage, plural } from './format';

const ROW_HEIGHT = 30;
const VIEW_HEIGHT = 360;
const OVERSCAN = 8;

interface Props {
  state: State;
  chapters: Chapter[];
  selected: number[];
  /** Locked while a series loads or a download runs */
  disabled: boolean;
  onToggle: (index: number, on: boolean) => Promise<void>;
  onSetAll: (on: boolean) => Promise<void>;
  onRange: (text: string) => Promise<number>;
  onError: (message: string) => void;
}

export function ChapterPanel({ state, chapters, selected, disabled, onToggle, onSetAll, onRange, onError }: Props) {
  const [rangeText, setRangeText] = useState('');
  const [rangeNote, setRangeNote] = useState('');
  const [scrollTop, setScrollTop] = useState(0);
  const scroller = useRef<HTMLDivElement>(null);

  // a new list starts at the top, whatever the old list was scrolled to
  useEffect(() => {
    setScrollTop(0);
    if (scroller.current) {
      scroller.current.scrollTop = 0;
    }
  }, [chapters]);

  const chosen = useMemo(() => new Set(selected), [selected]);
  const first = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const last = Math.min(chapters.length, Math.ceil((scrollTop + VIEW_HEIGHT) / ROW_HEIGHT) + OVERSCAN);
  const visible = chapters.slice(first, last);

  const applyRange = async () => {
    try {
      const n = await onRange(rangeText);
      setRangeNote(
        n === 0 ? 'No chapter matches that range.' : `Added ${plural(n, 'chapter', 'chapters')} to the selection.`,
      );
    } catch (e) {
      setRangeNote(`Invalid range: ${errorMessage(e)}`);
    }
  };

  const toggle = (index: number, on: boolean) => {
    onToggle(index, on).catch((e) => onError(errorMessage(e)));
  };

  if (!state.loaded) {
    return (
      <section className="panel chapters">
        <h2>Chapters</h2>
        <p className="hint">Paste a series URL above and press Load to list its chapters.</p>
      </section>
    );
  }

  return (
    <section className="panel chapters">
      <h2>{state.title}</h2>
      <p className="count">
        {state.selectedCount} of {state.chapterCount} selected
      </p>
      <div className="buttons">
        <button type="button" disabled={disabled} onClick={() => onSetAll(true).catch((e) => onError(errorMessage(e)))}>
          Select all
        </button>
        <button type="button" disabled={disabled} onClick={() => onSetAll(false).catch((e) => onError(errorMessage(e)))}>
          Select none
        </button>
      </div>
      <form
        className="range"
        onSubmit={(e) => {
          e.preventDefault();
          void applyRange();
        }}
      >
        <input
          type="text"
          placeholder="e.g. 1-10,12"
          aria-label="Chapter range"
          value={rangeText}
          disabled={disabled}
          onChange={(e) => setRangeText(e.target.value)}
        />
        <button type="submit" disabled={disabled || rangeText.trim() === ''}>
          Select range
        </button>
      </form>
      {rangeNote !== '' && <p className="note">{rangeNote}</p>}

      <div
        className="list"
        ref={scroller}
        style={{ height: VIEW_HEIGHT }}
        onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
        role="list"
      >
        <div style={{ height: chapters.length * ROW_HEIGHT, position: 'relative' }}>
          {visible.map((ch, i) => {
            const index = first + i;
            return (
              <label
                key={ch.index}
                role="listitem"
                className="row"
                style={{ top: index * ROW_HEIGHT, height: ROW_HEIGHT }}
              >
                <input
                  type="checkbox"
                  checked={chosen.has(ch.index)}
                  disabled={disabled}
                  onChange={(e) => toggle(ch.index, e.target.checked)}
                />
                <span className="number">{ch.number}</span>
                <span className="title">{ch.title}</span>
                {ch.language !== '' && <span className="lang">{ch.language}</span>}
              </label>
            );
          })}
        </div>
      </div>
      {state.hasDuplicates && (
        <p className="hint">Some numbers appear more than once, once per language. The language is shown next to each.</p>
      )}
    </section>
  );
}
