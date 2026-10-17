import { describe, expect, it } from 'vitest';
import { count, percent } from './format';

describe('count and percent', () => {
  it('groups digits in the language style', () => {
    expect(count(1234567, 'en')).toBe('1,234,567');
    expect(count(1234567, 'es')).toBe('1.234.567');
  });

  it('formats a fraction as a percentage', () => {
    expect(percent(0.25, 'en')).toBe('25%');
  });
});
