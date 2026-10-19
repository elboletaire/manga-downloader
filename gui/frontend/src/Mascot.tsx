// The Neko Book mascot, drawn from the brand's SVGs (assets/brand/mark). The
// header shows Soft, and Happy after a run that saved everything; the empty
// state shows Happy. The decisions live in mascot.ts.

import { useEffect, useRef, useState } from 'react';
import happyMark from '../../../assets/brand/mark/happy/mark.svg';
import happyMarkDark from '../../../assets/brand/mark/happy/mark-dark.svg';
import softMark from '../../../assets/brand/mark/soft/mark.svg';
import softMarkDark from '../../../assets/brand/mark/soft/mark-dark.svg';
import smallMark from '../../../assets/brand/mark/soft/mark-small.svg';
import smallMarkDark from '../../../assets/brand/mark/soft/mark-small-dark.svg';
import wordmark from '../../../assets/brand/wordmark/wordmark-line.svg';
import wordmarkDark from '../../../assets/brand/wordmark/wordmark-line-dark.svg';
import { celebrating, celebratesOnOpen, type Expression } from './expression';
import type { State } from './types';

/** Below this the detailed artwork turns to mush; the brand's small one is used */
const SMALL_BELOW = 48;

/**
 * The mascot as a <picture>, so the dark artwork follows the system theme the
 * way the brand's README snippet does. It's decorative: the text beside it
 * names the app.
 */
export function Mascot({ expression, size, flip }: { expression: Expression; size: number; flip?: string }) {
  const happy = expression === 'happy';
  const small = size < SMALL_BELOW;
  // below 48px Happy is Soft's face (mark-small is the same for both)
  const light = small ? smallMark : happy ? happyMark : softMark;
  const dark = small ? smallMarkDark : happy ? happyMarkDark : softMarkDark;

  return (
    <picture className="mascot" data-flip={flip}>
      <source media="(prefers-color-scheme: dark)" srcSet={dark} />
      {/* keyed by expression, so a change plays the entrance again */}
      <img key={expression} className="mascot-face" src={light} width={size} height={size} alt="" aria-hidden="true" />
    </picture>
  );
}

/** The brand's "manga downloader" lettering, themed like the mascot; hidden on narrow windows */
/** flip marks it for the simple view's glide (flip.ts) */
export function Wordmark({ flip }: { flip?: string }) {
  return (
    <picture className="brand-name" data-flip={flip}>
      <source media="(prefers-color-scheme: dark)" srcSet={wordmarkDark} />
      <img src={wordmark} height={20} alt="Manga Downloader" />
    </picture>
  );
}

/**
 * The header mascot's expression for the window's state. It's Happy from the
 * end of a run that saved everything, until the next load or download starts.
 */
export function useExpression(state: Pick<State, 'loading' | 'running' | 'summary'>): Expression {
  const [on, setOn] = useState(() => celebratesOnOpen(state));
  const wasRunning = useRef(state.running);
  useEffect(() => {
    const was = wasRunning.current;
    wasRunning.current = state.running;
    setOn((current) => celebrating(current, was, state));
  }, [state]);

  return on ? 'happy' : 'soft';
}
