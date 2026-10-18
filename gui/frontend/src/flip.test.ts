import { describe, expect, it } from 'vitest';
import { invert } from './flip';

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
