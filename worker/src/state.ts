import { DurableObject } from "cloudflare:workers";

export const RELEASE_URL = "https://api.github.com/repos/elboletaire/manga-downloader/releases/latest";
export const CACHE_TTL_MS = 5 * 60 * 1000;
const UPSTREAM_TIMEOUT_MS = 5000;

interface Salt {
  day: string;
  value: string;
}

interface CachedRelease {
  body: string;
  fetchedAt: number;
}

const hex = (bytes: Uint8Array) => Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");

// One instance for the whole app: it holds the day's salt and the cached release.
export class UpdateState extends DurableObject {
  // Keeps a single salt key, so the first request of a new UTC day replaces yesterday's salt.
  async identify(ip: string): Promise<{ day: string; hash: string }> {
    const day = new Date().toISOString().slice(0, 10);
    let salt = await this.ctx.storage.get<Salt>("salt");
    if (!salt || salt.day !== day) {
      salt = { day, value: hex(crypto.getRandomValues(new Uint8Array(32))) };
      await this.ctx.storage.put("salt", salt);
    }
    const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(salt.value + ip));
    return { day, hash: hex(new Uint8Array(digest)) };
  }

  // Serves the cached GitHub release, and the last good copy when GitHub fails; null if there is none.
  async latest(upstream: typeof fetch = fetch): Promise<string | null> {
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
    return cached?.body ?? null;
  }
}
