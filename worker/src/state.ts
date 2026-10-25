import { DurableObject } from "cloudflare:workers";

export const RELEASE_URL = "https://api.github.com/repos/elboletaire/manga-downloader/releases/latest";
export const CACHE_TTL_MS = 5 * 60 * 1000;
// Shorter than the client's own 5 s timeout, so a slow GitHub never outlasts the client.
const UPSTREAM_TIMEOUT_MS = 2500;
// After a failed refresh, the stale copy is served this long before GitHub is asked again.
const RETRY_AFTER_FAILURE_MS = 60 * 1000;

interface Salt {
  day: string;
  value: string;
}

interface CachedRelease {
  body: string;
  fetchedAt: number;
}

const hex = (bytes: Uint8Array) => Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");

const nextUtcMidnight = (now: number) => {
  const date = new Date(now);
  return Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate() + 1);
};

// One instance for the whole app: it holds the day's salt and the cached release.
export class UpdateState extends DurableObject {
  private inFlight: Promise<string | null> | null = null;

  // Keeps a single salt key, so a new UTC day replaces yesterday's salt; an alarm drops it at midnight even without traffic.
  async identify(ip: string): Promise<{ day: string; hash: string }> {
    const now = Date.now();
    const day = new Date(now).toISOString().slice(0, 10);
    let salt = await this.ctx.storage.get<Salt>("salt");
    if (!salt || salt.day !== day) {
      salt = { day, value: hex(crypto.getRandomValues(new Uint8Array(32))) };
      await this.ctx.storage.put("salt", salt);
      await this.ctx.storage.setAlarm(nextUtcMidnight(now));
    }
    const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(salt.value + ip));
    return { day, hash: hex(new Uint8Array(digest)) };
  }

  // Runs at midnight UTC. A request that already started today's salt keeps it.
  async alarm(): Promise<void> {
    const salt = await this.ctx.storage.get<Salt>("salt");
    if (salt && salt.day !== new Date().toISOString().slice(0, 10)) await this.ctx.storage.delete("salt");
  }

  // Serves the cached GitHub release, and the last good copy when GitHub fails; null if there is none.
  // Concurrent callers share one upstream request.
  latest(upstream: typeof fetch = fetch): Promise<string | null> {
    this.inFlight ??= this.load(upstream).finally(() => {
      this.inFlight = null;
    });
    return this.inFlight;
  }

  private async load(upstream: typeof fetch): Promise<string | null> {
    const cached = await this.ctx.storage.get<CachedRelease>("release");
    if (cached && Date.now() - cached.fetchedAt < CACHE_TTL_MS) return cached.body;
    try {
      const res = await upstream(RELEASE_URL, {
        headers: {
          "User-Agent": "manga-downloader-updates",
          Accept: "application/vnd.github+json",
        },
        signal: AbortSignal.timeout(UPSTREAM_TIMEOUT_MS),
      });
      if (res.ok) {
        const body = await res.text();
        await this.ctx.storage.put("release", { body, fetchedAt: Date.now() });
        return body;
      }
    } catch {
      // Network error or timeout: fall through to the stale copy.
    }
    if (!cached) return null;
    // Backdates the copy so it reads as stale only after the retry delay.
    await this.ctx.storage.put("release", { body: cached.body, fetchedAt: Date.now() - CACHE_TTL_MS + RETRY_AFTER_FAILURE_MS });
    return cached.body;
  }
}
