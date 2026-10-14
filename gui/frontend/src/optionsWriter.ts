// Writes the options to the backend. SetOptions takes the whole set, so each
// write is built from the last set the backend took, not from the last set
// pushed: two edits within one round trip would otherwise lose the first.
// Writes run one at a time, in call order. Only the patch is queued, and the
// full set is built when its turn comes, so a refused write never reaches the
// writes queued behind it.

import type { Options } from './types';

export interface OptionsWriter {
  /**
   * Queues a patch over the last options the backend took. With load, the write
   * also reloads that URL in the same step (a pick from a selector).
   */
  patch(patch: Partial<Options>, load?: string): Promise<void>;
  /** The backend's latest pushed options. They become the base once no write is in flight */
  pushed(options: Options): void;
}

export function createOptionsWriter(send: (options: Options, load?: string) => Promise<void>): OptionsWriter {
  // the last options the backend took, or pushed while nothing was in flight
  let committed: Options | null = null;
  let inFlight = 0;
  let queue: Promise<void> = Promise.resolve();

  return {
    patch(patch, load) {
      if (committed === null) {
        return Promise.reject(new Error('options patched before the window had them'));
      }
      inFlight += 1;
      const write = queue.then(() => {
        if (committed === null) {
          throw new Error('options patched before the window had them');
        }
        const next = { ...committed, ...patch };

        return send(next, load).then(() => {
          committed = next;
        });
      });
      // a refused write must not stop the ones queued behind it
      queue = write.catch(() => undefined);

      return write.then(
        () => {
          inFlight -= 1;
        },
        (e: unknown) => {
          inFlight -= 1;
          throw e;
        },
      );
    },
    pushed(options) {
      if (inFlight === 0) {
        committed = options;
      }
    },
  };
}
