import { describe, expect, it } from 'vitest';
import { createOptionsWriter } from './optionsWriter';
import type { Options } from './types';

const base: Options = {
  outputDir: '/out',
  format: 'cbz',
  bundle: false,
  language: '',
  scanlator: '',
  convertImages: 'avif',
  skipPages: '',
  filenameTemplate: '{{.Series}}',
  browserVisible: false,
};

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

/**
 * A fake SetOptions that holds each call until the test answers it. The writer
 * calls it from a queued promise, so answer() waits for the call to arrive.
 */
function delayedSend() {
  const sent: Options[] = [];
  const pending: Array<{ resolve: () => void; reject: (e: unknown) => void }> = [];
  const send = (options: Options) => {
    sent.push(options);
    return new Promise<void>((resolve, reject) => pending.push({ resolve, reject }));
  };
  const answer = async (i: number, refusal?: unknown) => {
    while (pending.length <= i) {
      await tick();
    }
    if (refusal === undefined) {
      pending[i].resolve();
    } else {
      pending[i].reject(refusal);
    }
  };
  return { sent, send, answer };
}

describe('createOptionsWriter', () => {
  it('keeps a text edit when a checkbox is clicked before the text write returns', async () => {
    const { sent, send, answer } = delayedSend();
    const writer = createOptionsWriter(send);
    writer.pushed(base);

    // the blur commits the text, then the click lands before the backend answers
    const text = writer.patch({ skipPages: '2' });
    const click = writer.patch({ bundle: true });
    await answer(0);
    await answer(1);
    await Promise.all([text, click]);

    expect(sent).toHaveLength(2);
    expect(sent[1]).toEqual({ ...base, skipPages: '2', bundle: true });
  });

  it('builds each write on the one before it, however many are queued', async () => {
    const { sent, send, answer } = delayedSend();
    const writer = createOptionsWriter(send);
    writer.pushed(base);

    const writes = [
      writer.patch({ format: 'raw' }),
      writer.patch({ filenameTemplate: 'x' }),
      writer.patch({ browserVisible: true }),
    ];
    for (let i = 0; i < writes.length; i++) {
      await answer(i);
    }
    await Promise.all(writes);

    expect(sent[2]).toEqual({ ...base, format: 'raw', filenameTemplate: 'x', browserVisible: true });
  });

  it('adopts a change the backend pushed, such as a folder picked from the dialog', async () => {
    const { sent, send, answer } = delayedSend();
    const writer = createOptionsWriter(send);
    writer.pushed(base);
    writer.pushed({ ...base, outputDir: '/picked' });

    const write = writer.patch({ bundle: true });
    await answer(0);
    await write;

    expect(sent[0]).toEqual({ ...base, outputDir: '/picked', bundle: true });
  });

  it('resets to the last pushed options when a write is refused', async () => {
    const { sent, send, answer } = delayedSend();
    const writer = createOptionsWriter(send);
    writer.pushed(base);

    const refused = expect(writer.patch({ skipPages: 'nope' })).rejects.toBe('bad pages');
    await answer(0, 'bad pages');
    await refused;

    const next = writer.patch({ bundle: true });
    await answer(1);
    await next;

    expect(sent[1]).toEqual({ ...base, bundle: true });
  });

  it('keeps writing after a refusal, and the next write carries none of the refused patch', async () => {
    const { sent, send, answer } = delayedSend();
    const writer = createOptionsWriter(send);
    writer.pushed(base);

    const refused = expect(writer.patch({ skipPages: 'nope' })).rejects.toBe('bad pages');
    const after = writer.patch({ bundle: true });
    await answer(0, 'bad pages');
    await answer(1);

    await refused;
    await expect(after).resolves.toBeUndefined();
    expect(sent[1]).toEqual({ ...base, bundle: true });
    expect(sent[1].skipPages).toBe(base.skipPages);
  });
});
