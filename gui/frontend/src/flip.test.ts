import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { invert, play, stop, type Box } from './flip';

const box = (left: number, top: number, width: number, height: number) => ({ left, top, width, height });

describe('invert', () => {
  it('scale mode puts the centre back and scales by the height', () => {
    // a 200x96 logo centred at (500, 300) now 100x48 centred at (100, 30)
    expect(invert(box(400, 252, 200, 96), box(50, 6, 100, 48), 'scale')).toEqual({
      transform: 'translate(400px, 270px) scale(2)',
    });
  });

  it('width mode keeps the left edge and the old width, so nothing is stretched', () => {
    expect(invert(box(200, 300, 640, 44), box(180, 12, 900, 38), 'width')).toEqual({
      transform: 'translate(20px, 288px)',
      width: '640px',
    });
  });

  it('move mode only moves the centre', () => {
    expect(invert(box(0, 0, 100, 30), box(500, 10, 28, 28), 'move')).toEqual({
      transform: 'translate(-464px, -9px)',
    });
  });
});

describe('play and stop', () => {
  // a fake element with the inline styles play() writes, laid out at (300, 40)
  const element = () => ({
    dataset: { flip: 'mascot' },
    style: { transition: '', transform: '', flex: '', width: '' },
    offsetWidth: 0,
    getBoundingClientRect: () => box(300, 40, 100, 40),
  });
  const root = (el: ReturnType<typeof element>) => ({ querySelectorAll: () => [el] }) as unknown as ParentNode;
  const first = new Map([['mascot', box(0, 0, 200, 80)]]);
  const modes = { mascot: 'scale' as const };

  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('document', { documentElement: {} });
    vi.stubGlobal('getComputedStyle', () => ({ getPropertyValue: () => '480' }));
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('a new glide is not cut short by the reset of the one before it', () => {
    const el = element();
    const r = root(el);
    play(r, first, modes);
    vi.advanceTimersByTime(400);
    play(r, first, modes);
    // the first glide's reset was due at 530ms; the second one's at 930ms
    vi.advanceTimersByTime(200);
    expect(el.style.transition).not.toBe('');
    vi.advanceTimersByTime(400);
    expect(el.style.transition).toBe('');
  });

  it('stop removes the inline styles of a glide still running', () => {
    const el = element();
    play(root(el), first, modes);
    expect(el.style.transition).not.toBe('');
    stop(root(el));
    expect(el.style.transition).toBe('');
    expect(el.style.transform).toBe('');
  });
});
