// Helpers for the lint tests that read the sources as text (see used-keys.test.ts and hardcoded.test.ts).

/** A source file's code without its comment lines: the examples in doc comments are not UI. */
export function code(text: string): string {
  return text
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\/\*|\*)/.test(line))
    .join('\n');
}
