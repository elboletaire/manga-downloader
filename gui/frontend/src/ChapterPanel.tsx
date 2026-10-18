// The chapter list: what's in the series and what's picked. It's windowed
// (virtual.ts) so thousands of chapters stay fast, and it's one focus stop:
// arrows move, Space toggles, Shift extends, as in a native list box.

import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react';
import { errorMessage, formatNumber } from './format';
import { say } from './i18n';
import type { T as Translator } from './i18n';
import { T, useI18n } from './i18n/react';
import { AlertIcon, ChevronIcon, LinkIcon, LockIcon, RefreshIcon } from './icons';
import { Mascot } from './Mascot';
import { extendsSpan, moveActive, pickable, spanIndices } from './listKeys';
import { hostOf } from './run';
import type { Chapter, State } from './types';
import { useVirtualList } from './virtual';
import {
  chapterSpan,
  groupChapters,
  groupedRows,
  groupParts,
  isSiteVolumes,
  parseVolumes,
  partRows,
  plainRows,
  selectedIn,
  type GroupedRow,
  volumesSummary,
} from './volumes';

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
  /** Sets the Volumes box; rejects a value that doesn't parse */
  onVolumes: (text: string) => Promise<void>;
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
  const { t } = useI18n();
  return (
    <section className="panel chapters empty-state" aria-label={t('chapters.section')}>
      <Mascot expression="happy" size={112} />
      <h2>{t('chapters.empty.title')}</h2>
      <p>{t('chapters.empty.paste')}</p>
      <p className="muted">{t('chapters.empty.listed')}</p>
    </section>
  );
}

function LoadingState({ url }: { url: string }) {
  const { t } = useI18n();
  const host = hostOf(url);
  return (
    <section className="panel chapters" aria-label={t('chapters.section')} aria-busy="true">
      <div className="series-head">
        <div>
          <div className="skeleton skeleton-title" />
          <p className="series-meta">
            <LinkIcon size={13} />{' '}
            {host !== '' ? t('chapters.loading.asking', { host }) : t('chapters.loading.fetching')}
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
  const { t } = useI18n();
  const host = hostOf(state.url);
  return (
    <section className="panel chapters empty-state failed" aria-label={t('chapters.section')}>
      <div className="empty-icon danger">
        <AlertIcon size={26} />
      </div>
      <h2>{t('chapters.failed.title')}</h2>
      <p className="error-text" role="alert">
        {say(t, state.loadErrorMsg, state.loadError)}
      </p>
      {host !== '' && <p className="muted mono small">{state.url}</p>}
      <p className="muted">
        <T k="chapters.failed.hint" />
      </p>
      <button type="button" className="btn" disabled={state.loading || state.running} onClick={onRetry}>
        <RefreshIcon /> {t('chapters.failed.retry')}
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
  onVolumes,
  onError,
}: Props) {
  const { t } = useI18n();
  const [rangeText, setRangeText] = useState('');
  const [rangeNote, setRangeNote] = useState<{ text: string; bad: boolean } | null>(null);
  const [active, setActive] = useState(-1);
  // the groups folded away, by key (LOOSE for "Not in a volume")
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  // where Shift+click and Shift+Space extend from
  const anchor = useRef(-1);
  const chosen = useMemo(() => new Set(selected), [selected]);
  const host = hostOf(state.url);
  const byVolume = state.options.packAs === 'volumes';
  const byParts = state.options.packAs === 'parts';

  // the backend only takes a Volumes box that parses, so this is the grouping
  // a run would pack; the site's volumes stand in should it not
  const grouping = useMemo(() => {
    if (!byVolume) {
      return null;
    }
    let spec;
    try {
      spec = parseVolumes(state.volumes.text);
    } catch {
      spec = parseVolumes('');
    }
    return groupChapters(chapters, spec);
  }, [byVolume, chapters, state.volumes.text]);
  // a parts run merges the chapters sharing a number, group and language
  const parts = useMemo(() => (byParts ? groupParts(chapters) : null), [byParts, chapters]);
  const split = useMemo(() => parts?.filter((g) => g.chapters.length > 1).length ?? 0, [parts]);
  const rows: GroupedRow[] = useMemo(() => {
    if (grouping !== null) {
      return groupedRows(grouping, collapsed, t);
    }
    return parts !== null ? partRows(parts, collapsed, t) : plainRows(chapters);
  }, [grouping, parts, chapters, collapsed, t]);
  const list = useVirtualList(rows.length, ROW_HEIGHT);

  // a new list starts at the top, with nothing active and nothing folded
  useEffect(() => {
    list.reset();
    setActive(-1);
    setCollapsed(new Set());
    anchor.current = -1;
  }, [chapters, list.reset]);

  // rows come and go as groups fold: keep the active row in the list
  useEffect(() => {
    if (active >= rows.length) {
      setActive(rows.length - 1);
    }
  }, [rows.length, active]);

  const fail = (e: unknown) => onError(errorMessage(e, t));

  // the chapters a span of rows covers: a header stands for its chapters
  // only while folded, as unfolded they're rows of the span already.
  // Locked chapters can't be picked, so no span covers them.
  const chaptersOf = (positions: number[]) => {
    const out: number[] = [];
    for (const p of positions) {
      const row = rows[p];
      if (row.kind === 'chapter') {
        if (!row.chapter.locked) {
          out.push(row.chapter.index);
        }
      } else if (row.collapsed) {
        out.push(...pickable(row.chapters).map((c) => c.index));
      }
    }
    return out;
  };

  const toggleGroup = (row: Extract<GroupedRow, { kind: 'group' }>) => {
    const open = pickable(row.chapters);
    const on = selectedIn(open, chosen) < open.length;
    const changing = open.map((c) => c.index).filter((i) => chosen.has(i) !== on);
    if (changing.length > 0) {
      onToggleMany(changing, on).catch(fail);
    }
  };

  const toggleFold = (key: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const toggleRow = (pos: number, extend: boolean) => {
    const row = rows[pos];
    if (!row || disabled) {
      return;
    }
    if (row.kind === 'group' && !extend) {
      toggleGroup(row);
      anchor.current = pos;
      setActive(pos);
      return;
    }
    if (row.kind === 'chapter' && row.chapter.locked) {
      // a locked chapter takes the focus but is never picked, nor does it anchor a span
      setActive(pos);
      return;
    }
    const on =
      row.kind === 'chapter'
        ? !chosen.has(row.chapter.index)
        : selectedIn(pickable(row.chapters), chosen) < pickable(row.chapters).length;
    if (extend && anchor.current >= 0) {
      // Shift: the whole span takes the state the clicked row is switching to
      const changing = chaptersOf(spanIndices(anchor.current, pos)).filter((i) => chosen.has(i) !== on);
      if (changing.length > 0) {
        onToggleMany(changing, on).catch(fail);
      }
    } else if (row.kind === 'chapter') {
      onToggle(row.chapter.index, on).catch(fail);
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
    const changing = chaptersOf(spanIndices(anchor.current, pos)).filter((i) => !chosen.has(i));
    if (changing.length > 0) {
      onToggleMany(changing, true).catch(fail);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const page = Math.max(1, Math.floor(list.viewHeight / ROW_HEIGHT));
    const next = moveActive(e.key, active, rows.length, page);
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
    const row = rows[active];
    // left and right fold and unfold a group, as in a tree
    if (row?.kind === 'group' && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
      e.preventDefault();
      if (row.collapsed === (e.key === 'ArrowRight')) {
        toggleFold(row.key);
      }
      return;
    }
    // and left on a group's chapter goes up to its header
    if (row?.kind === 'chapter' && row.inGroup && e.key === 'ArrowLeft') {
      e.preventDefault();
      for (let p = active - 1; p >= 0; p--) {
        if (rows[p].kind === 'group') {
          setActive(p);
          list.reveal(p);
          break;
        }
      }
      return;
    }
    if (row?.kind === 'group' && e.key === 'Enter') {
      e.preventDefault();
      toggleFold(row.key);
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
        onSetAll(selected.length !== selectable).catch(fail);
      }
    }
  };

  const applyRange = async () => {
    try {
      const n = await onRange(rangeText);
      setRangeNote(
        n === 0
          ? { text: t('chapters.rangeNone'), bad: true }
          : { text: t('chapters.rangeAdded', { count: n }), bad: false },
      );
    } catch (e) {
      setRangeNote({ text: errorMessage(e, t), bad: true });
    }
  };

  // locked chapters can't be picked, so "all" and the count are of the others
  const selectable = useMemo(() => pickable(chapters).length, [chapters]);
  const all = selectable > 0 && state.selectedCount === selectable;
  const some = state.selectedCount > 0 && !all;
  const { first, last } = list.span;
  const activeRow = active >= first && active < last ? rows[active] : undefined;

  return (
    <section className="panel chapters" aria-label={t('chapters.section')}>
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
            <span>{t('count.chapter', { count: state.chapterCount })}</span>
            {byParts && split > 0 && (
              <>
                <span className="dot" aria-hidden="true">
                  ·
                </span>
                <span>{t('chapters.splitParts', { count: split })}</span>
              </>
            )}
            {state.loading && <span className="reloading">{t('chapters.reloading')}</span>}
          </p>
        </div>
      </div>

      <div className="list-toolbar">
        <label className="master" title={all ? t('chapters.selectNone') : t('chapters.selectAll')}>
          <input
            type="checkbox"
            checked={all}
            ref={(el) => {
              if (el) {
                el.indeterminate = some;
              }
            }}
            disabled={disabled || selectable === 0}
            aria-label={all ? t('chapters.selectNone') : t('chapters.selectAll')}
            onChange={() => onSetAll(!all).catch(fail)}
          />
        </label>
        <p className="count" aria-live="polite">
          <T k="chapters.selected" values={{ selected: state.selectedCount, total: selectable }} />
        </p>
        <div className="toolbar-buttons">
          <button
            type="button"
            className="btn ghost small"
            disabled={disabled || all || selectable === 0}
            onClick={() => onSetAll(true).catch(fail)}
          >
            {t('common.all')}
          </button>
          <button
            type="button"
            className="btn ghost small"
            disabled={disabled || state.selectedCount === 0}
            onClick={() => onSetAll(false).catch(fail)}
          >
            {t('common.none')}
          </button>
        </div>
        <RangeForm
          text={rangeText}
          bad={rangeNote?.bad ?? false}
          disabled={disabled}
          onText={(text) => {
            setRangeText(text);
            setRangeNote(null);
          }}
          onApply={() => void applyRange()}
        />
      </div>
      {rangeNote && (
        <p className={`range-note ${rangeNote.bad ? 'bad' : 'good'}`} role="status">
          {rangeNote.text}
        </p>
      )}
      {grouping !== null && (
        <VolumesBar
          text={state.volumes.text}
          site={state.volumes.site}
          listed={state.volumes.listed}
          volumes={grouping.volumes.length}
          loose={grouping.loose.length}
          disabled={disabled}
          onVolumes={onVolumes}
        />
      )}

      <div
        className={`list${disabled ? ' locked' : ''}`}
        ref={list.ref}
        onScroll={list.onScroll}
        role="listbox"
        aria-label={t('chapters.listLabel', { title: state.title })}
        aria-multiselectable="true"
        aria-disabled={disabled || undefined}
        aria-activedescendant={
          // only a row that's rendered can be the descendant; scrolling may have moved the active one out
          activeRow ? rowId(activeRow) : undefined
        }
        tabIndex={0}
        onKeyDown={onKeyDown}
        onFocus={() => {
          if (active < 0 && rows.length > 0) {
            setActive(Math.min(rows.length - 1, Math.floor((list.ref.current?.scrollTop ?? 0) / ROW_HEIGHT)));
          }
        }}
      >
        <div className="list-inner" style={{ height: rows.length * ROW_HEIGHT }}>
          {rows.slice(first, last).map((row, i) => {
            const pos = first + i;
            const style = { transform: `translateY(${pos * ROW_HEIGHT}px)`, height: ROW_HEIGHT };
            const keepFocus = (e: MouseEvent) => {
              // keep the focus on the list and stop Shift from selecting text
              e.preventDefault();
              list.ref.current?.focus({ preventScroll: true });
            };
            if (row.kind === 'group') {
              const open = pickable(row.chapters);
              const n = selectedIn(open, chosen);
              const on = open.length > 0 && n === open.length;
              const label = `${row.label} · ${row.detail}`;
              const count = byParts
                ? partsCount(n, row.chapters.length, t)
                : n > 0
                  ? t('chapters.group.selected', { count: n })
                  : t('chapters.noneSelected');
              return (
                <div
                  key={rowId(row)}
                  id={rowId(row)}
                  role="option"
                  aria-setsize={rows.length}
                  aria-posinset={pos + 1}
                  aria-selected={on}
                  aria-disabled={open.length === 0 || undefined}
                  aria-label={
                    row.collapsed
                      ? t('chapters.group.ariaFolded', { label: row.label, detail: row.detail, selected: n, total: open.length })
                      : t('chapters.group.aria', { label: row.label, detail: row.detail, selected: n, total: open.length })
                  }
                  className={`row group-row${on ? ' on' : ''}${pos === active ? ' active' : ''}`}
                  style={style}
                  onMouseDown={keepFocus}
                  onClick={(e) => toggleRow(pos, e.shiftKey)}
                >
                  <span className={`box${on ? ' on' : n > 0 ? ' some' : ''}`} aria-hidden="true" />
                  <button
                    type="button"
                    className={`fold${row.collapsed ? '' : ' open'}`}
                    tabIndex={-1}
                    aria-label={row.collapsed ? t('chapters.group.unfold', { label: row.label }) : t('chapters.group.fold', { label: row.label })}
                    onMouseDown={keepFocus}
                    onClick={(e) => {
                      e.stopPropagation();
                      toggleFold(row.key);
                      setActive(pos);
                    }}
                  >
                    <ChevronIcon size={14} />
                  </button>
                  <span className="title group-title">{label}</span>
                  <span className="group-end">
                    {row.tag !== '' && (
                      <span className="tags">
                        <span className="group">{row.tag}</span>
                      </span>
                    )}
                    <span
                      className={`group-count${byParts && n > 0 && n < row.chapters.length ? ' partial' : ''}`}
                      title={byParts && n > 0 && n < row.chapters.length ? t('chapters.parts.partialHint') : undefined}
                    >
                      {count}
                    </span>
                  </span>
                </div>
              );
            }
            const ch = row.chapter;
            const on = chosen.has(ch.index);
            return (
              <div
                key={rowId(row)}
                id={rowId(row)}
                role="option"
                aria-setsize={rows.length}
                aria-posinset={pos + 1}
                aria-selected={on}
                aria-disabled={ch.locked || undefined}
                className={`row${row.inGroup ? ' in-group' : ''}${row.part ? ' part-row' : ''}${on ? ' on' : ''}${ch.locked ? ' locked' : ''}${pos === active ? ' active' : ''}`}
                style={style}
                onMouseDown={keepFocus}
                onClick={(e) => toggleRow(pos, e.shiftKey)}
              >
                <ChapterBox on={on} locked={ch.locked} />
                {!row.part && <span className="num">{formatNumber(ch.number)}</span>}
                <ChapterTitle title={row.part ? row.part.label : ch.title} locked={ch.locked} />
                {(ch.language !== '' || ch.group !== '') && (
                  <span className="tags">
                    {ch.language !== '' && <span className="lang">{ch.language}</span>}
                    {ch.group !== '' && <span className="group">{ch.group}</span>}
                  </span>
                )}
              </div>
            );
          })}
        </div>
      </div>
      <p className="list-foot">
        {state.hasDuplicates || state.hasGroupDuplicates ? (
          <>
            {state.hasDuplicates && <>{t('chapters.duplicates.language')} </>}
            {state.hasGroupDuplicates && <>{t('chapters.duplicates.group')}</>}
          </>
        ) : grouping !== null ? (
          <T k="chapters.hint.volume" />
        ) : byParts && split > 0 ? (
          <T k="chapters.hint.part" />
        ) : byParts ? (
          t('chapters.parts.none')
        ) : (
          <T k="chapters.hint.plain" />
        )}
      </p>
    </section>
  );
}

/** A chapter's checkbox: greyed out for a locked chapter, which can't be picked */
export function ChapterBox({ on, locked }: { on: boolean; locked: boolean }) {
  return <span className={`box${on ? ' on' : ''}${locked ? ' disabled' : ''}`} aria-hidden="true" />;
}

/** A range box with only blanks in it has nothing to apply, so Enter does nothing there */
export function canApplyRange(text: string): boolean {
  return text.trim() !== '';
}

/** The range box: Enter or the + inside it applies the range, and the + shows only once there's text */
export function RangeForm({
  text,
  bad,
  disabled,
  onText,
  onApply,
}: {
  text: string;
  bad: boolean;
  disabled?: boolean;
  onText: (text: string) => void;
  onApply: () => void;
}) {
  const { t } = useI18n();
  return (
    <form
      className="range-form"
      onSubmit={(e) => {
        e.preventDefault();
        if (canApplyRange(text)) {
          onApply();
        }
      }}
    >
      <div className="input-wrap">
        <input
          type="text"
          className={`input small mono${bad ? ' invalid' : ''}`}
          placeholder="1-10,12"
          aria-label={t('chapters.rangeLabel')}
          value={text}
          disabled={disabled}
          spellCheck={false}
          onChange={(e) => onText(e.target.value)}
        />
        {text.trim() !== '' && (
          <div className="input-action">
            <button
              type="submit"
              className="btn ghost small icon-only"
              aria-label={t('chapters.addRange')}
              title={t('chapters.addRange')}
              disabled={disabled}
            >
              +
            </button>
          </div>
        )}
      </div>
    </form>
  );
}

/** A chapter's title, marked with a lock when the listing says it's locked */
export function ChapterTitle({ title, locked }: { title: string; locked: boolean }) {
  const { t } = useI18n();
  return (
    <span className="title">
      {locked && (
        <span className="lock" role="img" title={t('chapters.locked')} aria-label={t('chapters.locked')}>
          <LockIcon size={12} />
        </span>
      )}
      {title}
    </span>
  );
}

function rowId(row: GroupedRow): string {
  return row.kind === 'group' ? `group-${row.key}` : `chapter-${row.chapter.index}`;
}

/** What a chapter's header says of its picked parts: none, all of them, or how many */
export function partsCount(selected: number, total: number, t: Translator): string {
  if (selected === 0) {
    return t('chapters.noneSelected');
  }
  return selected === total ? t('chapters.parts.whole') : t('chapters.parts.partial', { selected, total });
}

interface VolumesBarProps {
  text: string;
  site: string;
  listed: number;
  volumes: number;
  loose: number;
  disabled: boolean;
  onVolumes: (text: string) => Promise<void>;
}

/**
 * The Volumes box: chapter ranges, one group per volume, as --volumes takes
 * them. It starts empty (the site's volumes); the line under it offers them
 * as ranges when they can be written so. Commits on Enter or on blur.
 */
function VolumesBar({ text, site, listed, volumes, loose, disabled, onVolumes }: VolumesBarProps) {
  const { t } = useI18n();
  const [draft, setDraft] = useState(text);
  const [error, setError] = useState('');
  // a pushed value replaces the draft, as in the option fields
  useEffect(() => {
    setDraft(text);
    setError('');
  }, [text]);

  const commit = (value: string) => {
    if (value.trim() === text.trim()) {
      setError('');
      return;
    }
    onVolumes(value).then(
      () => setError(''),
      (e: unknown) => setError(errorMessage(e, t)),
    );
  };

  const summary = volumesSummary(text, site, listed, volumes, loose, t);

  return (
    <div className="volumes-bar">
      <form
        className="volumes-form"
        onSubmit={(e) => {
          e.preventDefault();
          commit(draft);
        }}
      >
        <label htmlFor="volumes-input">{t('volumes.label')}</label>
        <input
          id="volumes-input"
          type="text"
          className={`input small mono${error !== '' ? ' invalid' : ''}`}
          placeholder={listed > 0 ? t('volumes.placeholder', { example: '1-7/8-16' }) : '1-7/8-16'}
          title={t('volumes.title', { separator: '/', prefix: 'N:', example: '16:112-114' })}
          value={draft}
          disabled={disabled}
          spellCheck={false}
          autoComplete="off"
          aria-describedby="volumes-note"
          aria-invalid={error !== '' || undefined}
          onChange={(e) => {
            setDraft(e.target.value);
            setError('');
          }}
          onBlur={() => commit(draft)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              setDraft(text);
              setError('');
            }
          }}
        />
        {site !== '' && (
          <button
            type="button"
            className="btn ghost small"
            disabled={disabled || (text === site && draft === site)}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => {
              setDraft(site);
              commit(site);
            }}
          >
            {t('volumes.useSite')}
          </button>
        )}
      </form>
      <p
        id="volumes-note"
        className={`volumes-note${error !== '' ? ' bad' : ''}`}
        role={error !== '' ? 'alert' : undefined}
      >
        {error !== '' ? (
          error
        ) : (
          <>
            <span className="volumes-lead">{summary.text}</span>
            {summary.ranges !== undefined && (
              <>
                <span className="volumes-sep" aria-hidden="true">
                  ·
                </span>
                <span className="volumes-ranges mono" title={summary.ranges}>
                  {summary.ranges}
                </span>
              </>
            )}
          </>
        )}
      </p>
    </div>
  );
}
