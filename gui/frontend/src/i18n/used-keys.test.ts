import { describe, expect, it } from 'vitest';
import { CATALOG } from './index';

// The app's sources, as text: there's no @types/node to read the disk with.
// gui/*.go is not read here; msg_test.go already checks the Go side's keys.
const sources = import.meta.glob('../**/*.{ts,tsx}', { query: '?raw', import: 'default', eager: true }) as Record<
  string,
  string
>;

// The glob's paths are relative to this file; resolved, they name the source as /i18n/index.ts.
const resolve = (path: string) => new URL(path, 'http://src/i18n/').pathname;

// Tests and the mock's own messages are not the app's UI.
const app = Object.entries(sources)
  .map(([path, text]) => [resolve(path), text] as const)
  .filter(([path]) => !/\.test\.tsx?$/.test(path) && !path.endsWith('/mock.ts'));

const en = new Set(Object.keys(CATALOG.en ?? {}));
const known = (key: string) => en.has(key) || (en.has(`${key}_one`) && en.has(`${key}_other`));

// Helpers that pass on a key they were given, not one they wrote; t() gets
// their first argument as is.
const DYNAMIC_KEYS = [{ path: '/i18n/index.ts', arg: 'msg.key' }];

// Comment lines are not code; the examples in them use keys that don't exist.
const code = (text: string) =>
  text
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\/\*|\*)/.test(line))
    .join('\n');

const CALL = /(?<![\w$.])t\(/g;
const LITERAL = /^\s*(['"`])([^'"`$\\]*)\1/;
const T_KEY = /<T\s+k="([^"]*)"/g;
const MESSAGE_KEY = /\b(?:msgOf|reject)\(\s*'([^']*)'/g;

interface Use {
  path: string;
  key: string;
}

const literalUses: Use[] = [];
const dynamic: string[] = [];

for (const [path, raw] of app) {
  const text = code(raw);
  for (const call of text.matchAll(CALL)) {
    const rest = text.slice(call.index + call[0].length);
    const literal = LITERAL.exec(rest);
    if (literal) {
      literalUses.push({ path, key: literal[2] });
    } else if (!DYNAMIC_KEYS.some((d) => path === d.path && rest.startsWith(d.arg))) {
      dynamic.push(`${path}: t(${rest.slice(0, 40).split('\n')[0]}`);
    }
  }
  for (const m of text.matchAll(T_KEY)) literalUses.push({ path, key: m[1] });
  for (const m of text.matchAll(MESSAGE_KEY)) literalUses.push({ path, key: m[1] });
}

describe('used keys', () => {
  it('finds the keys the app uses', () => {
    expect(literalUses.length).toBeGreaterThan(200);
  });

  it('has every key the app uses in the en catalog', () => {
    const missing = literalUses.filter((u) => !known(u.key)).map((u) => `${u.path}: ${u.key}`);
    expect(missing).toEqual([]);
  });

  it('takes a string literal as the key of every t() call', () => {
    expect(dynamic).toEqual([]);
  });
});
