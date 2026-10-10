// The chapter list: what's in the series and what's picked. It's windowed
// (virtual.ts) so thousands of chapters stay fast, and it's one focus stop:
// arrows move, Space toggles, Shift extends, as in a native list box.

import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react';
import { errorMessage, plural } from './format';
import { AlertIcon, LinkIcon, RefreshIcon } from './icons';
import { Mascot } from './Mascot';
import { extendsSpan, moveActive, spanIndices } from './listKeys';
import { formatNumber, hostOf } from './run';
import type { Chapter, State } from './types';
import { useVirtualList } from './virtual';

const ROW_HEIGHT = 34;

interface Props {
  state: State;
  chapters: Chapter[];
  selected: number[];
  /** Locked while a series loads or a download runs */
  disabled: boolean;
  onToggle: (index: number, on: boolean) => Promise<void>;
  /** Shift-span: one call for the whole span, so it's one change for the window */
  onToggleMany: (indices: number[], on: boolean) => Promise<void>;
  onSetAll: (on: boolean) => Promise<void>;
  onRange: (text: string) => Promise<number>;
  onReload: () => void;
  onError: (message: string) => void;
}

export function ChapterPanel(props: Props) {
  const { state } = props;

  if (state.loading && !state.loaded) {
    return <LoadingState url={state.url} />;
  }
  if (!state.loaded && state.loadError !== '') {
    return <FailedState state={state} onRetry={props.onReload} />;
  }
  if (!state.loaded) {
    return <EmptyState />;
  }

  return <ChapterList {...props} />;
}

function EmptyState() {
  return (
    <section className="panel chapters empty-state" aria-label="Chapters">
      <Mascot expression="happy" size={112} />
      <h2>Start with a series</h2>
      <p>
        Paste the link to a series in the bar above and press <kbd>Enter</kbd>. A link to any of its chapters works too.
      </p>
      <p className="muted">Its chapters will be listed here, ready to pick and save as CBZ files for your reader.</p>
    </section>
  );
}

function LoadingState({ url }: { url: string }) {
  const host = hostOf(url);
  return (
    <section className="panel chapters" aria-label="Chapters" aria-busy="true">
      <div className="series-head">
        <div>
          <div className="skeleton skeleton-title" />
          <p className="series-meta">
            <LinkIcon size={13} /> {host !== '' ? `Asking ${host} for the chapter list…` : 'Fetching the chapter list…'}
          </p>
        </div>
      </div>
      <div className="list skeleton-list" aria-hidden="true">
        {Array.from({ length: 14 }, (_, i) => (
          <div className="skeleton-row" key={i} style={{ animationDelay: `${i * 60}ms` }}>
            <span className="skeleton skeleton-box" />
            <span className="skeleton skeleton-num" />
            <span className="skeleton skeleton-text" style={{ width: `${30 + ((i * 37) % 40)}%` }} />
          </div>
        ))}
      </div>
    </section>
  );
}

function FailedState({ state, onRetry }: { state: State; onRetry: () => void }) {
  const host = hostOf(state.url);
  return (
    <section className="panel chapters empty-state failed" aria-label="Chapters">
      <div className="empty-icon danger">
        <AlertIcon size={26} />
      </div>
      <h2>Couldn't load this series</h2>
      <p className="error-text" role="alert">
        {state.loadError}
      </p>
      {host !== '' && <p className="muted mono small">{state.url}</p>}
      <p className="muted">
        Check that the link opens the series in a browser. Sites behind a Cloudflare check may need{' '}
        <em>Show the browser window</em>, under Options › Advanced.
      </p>
      <button type="button" className="btn" disabled={state.loading || state.running} onClick={onRetry}>
        <RefreshIcon /> Try again
      </button>
    </section>
  );
}

function ChapterList({
  state,
  chapters,
  selected,
  disabled,
  onToggle,
  onToggleMany,
  onSetAll,
  onRange,
  onError,
}: Props) {
  const [rangeText, setRangeText] = useState('');
  const [rangeNote, setRangeNote] = useState<{ text: string; bad: boolean } | null>(null);
  const [active, setActive] = useState(-1);
  // where Shift+click and Shift+Space extend from
  const anchor = useRef(-1);
  const list = useVirtualList(chapters.length, ROW_HEIGHT);
  const chosen = useMemo(() => new Set(selected), [selected]);
  const host = hostOf(state.url);

  // a new list starts at the top, with nothing active
  useEffect(() => {
    list.reset();
    setActive(-1);
    anchor.current = -1;
  }, [chapters, list.reset]);

  const fail = (e: unknown) => onError(errorMessage(e));

  const toggleRow = (pos: number, extend: boolean) => {
    const ch = chapters[pos];
    if (!ch || disabled) {
      return;
    }
    const on = !chosen.has(ch.index);
    if (extend && anchor.current >= 0) {
      // Shift: the whole span takes the state the clicked row is switching to
      const span = spanIndices(anchor.current, pos).map((p) => chapters[p].index);
      const changing = span.filter((i) => chosen.has(i) !== on);
      if (changing.length > 0) {
        onToggleMany(changing, on).catch(fail);
      }
    } else {
      onToggle(ch.index, on).catch(fail);
    }
    anchor.current = pos;
    setActive(pos);
  };

  // Shift+arrow: selects the rows from the anchor to the new active row, in
  // one bulk change. Select-only, by choice: Shift+click and Shift+Space set
  // the span to the clicked row's state, which here would deselect the anchor
  // when walking back over rows already selected.
  const selectSpan = (pos: number) => {
    if (anchor.current < 0) {
      anchor.current = pos;
    }
    const changing = spanIndices(anchor.current, pos)
      .map((p) => chapters[p].index)
      .filter((i) => !chosen.has(i));
    if (changing.length > 0) {
      onToggleMany(changing, true).catch(fail);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const page = Math.max(1, Math.floor(list.viewHeight / ROW_HEIGHT));
    const next = moveActive(e.key, active, chapters.length, page);
    if (next !== null) {
      e.preventDefault();
      if (extendsSpan(e.key, e.shiftKey) && !disabled) {
        if (anchor.current < 0 && active >= 0) {
          anchor.current = active;
        }
        selectSpan(next);
      }
      setActive(next);
      list.reveal(next);
      return;
    }
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      toggleRow(active < 0 ? 0 : active, e.shiftKey);
      return;
    }
    if (e.key.toLowerCase() === 'a' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      if (!disabled) {
        onSetAll(selected.length !== chapters.length).catch(fail);
      }
    }
  };

  const applyRange = async () => {
    try {
      const n = await onRange(rangeText);
      setRangeNote(
        n === 0
          ? { text: 'No chapter matches that range.', bad: true }
          : { text: `Added ${plural(n, 'chapter', 'chapters')}.`, bad: false },
      );
    } catch (e) {
      setRangeNote({ text: errorMessage(e), bad: true });
    }
  };

  const all = state.chapterCount > 0 && state.selectedCount === state.chapterCount;
  const some = state.selectedCount > 0 && !all;
  const { first, last } = list.span;

  return (
    <section className="panel chapters" aria-label="Chapters">
      <div className="series-head">
        <div className="series-id">
          <h2 className="series-title" title={state.title}>
            {state.title}
          </h2>
          <p className="series-meta">
            {host !== '' && (
              <>
                <span className="host">{host}</span>
                <span className="dot" aria-hidden="true">
                  ·
                </span>
              </>
            )}
            <span>{plural(state.chapterCount, 'chapter', 'chapters')}</span>
            {state.loading && <span className="reloading">Reloading…</span>}
          </p>
        </div>
      </div>

      <div className="list-toolbar">
        <label className="master" title={all ? 'Select none' : 'Select all'}>
          <input
            type="checkbox"
            checked={all}
            ref={(el) => {
              if (el) {
                el.indeterminate = some;
              }
            }}
            disabled={disabled}
            aria-label={all ? 'Select none' : 'Select all'}
            onChange={() => onSetAll(!all).catch(fail)}
          />
        </label>
        <p className="count" aria-live="polite">
          <strong>{state.selectedCount}</strong> of {state.chapterCount} selected
        </p>
        <div className="toolbar-buttons">
          <button
            type="button"
            className="btn ghost small"
            disabled={disabled || all}
            onClick={() => onSetAll(true).catch(fail)}
          >
            All
          </button>
          <button
            type="button"
            className="btn ghost small"
            disabled={disabled || state.selectedCount === 0}
            onClick={() => onSetAll(false).catch(fail)}
          >
            None
          </button>
        </div>
        <form
          className="range-form"
          onSubmit={(e) => {
            e.preventDefault();
            void applyRange();
          }}
        >
          <input
            type="text"
            className={`input small mono${rangeNote?.bad ? ' invalid' : ''}`}
            placeholder="1-10,12"
            aria-label="Chapter range to add to the selection"
            value={rangeText}
            disabled={disabled}
            spellCheck={false}
            onChange={(e) => {
              setRangeText(e.target.value);
              setRangeNote(null);
            }}
          />
          <button type="submit" className="btn small" disabled={disabled || rangeText.trim() === ''}>
            Add range
          </button>
        </form>
      </div>
      {rangeNote && (
        <p className={`range-note ${rangeNote.bad ? 'bad' : 'good'}`} role="status">
          {rangeNote.text}
        </p>
      )}

      <div
        className={`list${disabled ? ' locked' : ''}`}
        ref={list.ref}
        onScroll={list.onScroll}
        role="listbox"
        aria-label={`Chapters of ${state.title}`}
        aria-multiselectable="true"
        aria-disabled={disabled || undefined}
        aria-activedescendant={
          // only a row that's rendered can be the descendant; scrolling may have moved the active one out
          active >= first && active < last && chapters[active] ? `chapter-${chapters[active].index}` : undefined
        }
        tabIndex={0}
        onKeyDown={onKeyDown}
        onFocus={() => {
          if (active < 0 && chapters.length > 0) {
            setActive(Math.min(chapters.length - 1, Math.floor((list.ref.current?.scrollTop ?? 0) / ROW_HEIGHT)));
          }
        }}
      >
        <div className="list-inner" style={{ height: chapters.length * ROW_HEIGHT }}>
          {chapters.slice(first, last).map((ch, i) => {
            const pos = first + i;
            const on = chosen.has(ch.index);
            return (
              <div
                key={ch.index}
                id={`chapter-${ch.index}`}
                role="option"
                aria-setsize={chapters.length}
                aria-posinset={pos + 1}
                aria-selected={on}
                className={`row${on ? ' on' : ''}${pos === active ? ' active' : ''}`}
                style={{ transform: `translateY(${pos * ROW_HEIGHT}px)`, height: ROW_HEIGHT }}
                onMouseDown={(e: MouseEvent) => {
                  // keep the focus on the list and stop Shift from selecting text
                  e.preventDefault();
                  list.ref.current?.focus({ preventScroll: true });
                }}
                onClick={(e) => toggleRow(pos, e.shiftKey)}
              >
                <span className={`box${on ? ' on' : ''}`} aria-hidden="true" />
                <span className="num">{formatNumber(ch.number)}</span>
                <span className="title">{ch.title}</span>
                {ch.language !== '' && <span className="lang">{ch.language}</span>}
              </div>
            );
          })}
        </div>
      </div>
      <p className="list-foot">
        {state.hasDuplicates ? (
          <>Some numbers are listed once per language; the language tag tells them apart.</>
        ) : (
          <>
            <kbd>Shift</kbd>+click selects a span · <kbd>↑</kbd>
            <kbd>↓</kbd> move · <kbd>Space</kbd> toggles
          </>
        )}
      </p>
    </section>
  );
}
