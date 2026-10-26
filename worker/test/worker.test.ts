import { env, runDurableObjectAlarm, runInDurableObject } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import worker from "../src/index";
import { CACHE_TTL_MS } from "../src/state";

// Whitespace and non-ASCII on purpose: the Worker must not re-serialise the body.
const RELEASE = '{ "tag_name":"v23.0.0",  "name":"Ünïcode ☃", "assets":[] }\n';
const NEWER = '{"tag_name":"v23.1.0"}';
const IP = "203.0.113.77";
const BASE = "https://updates.test";

const stub = () => env.UPDATE_STATE.getByName("updates");
const points: AnalyticsEngineDataPoint[] = [];
const testEnv = {
  UPDATE_STATE: env.UPDATE_STATE,
  UPDATE_CHECKS: { writeDataPoint: (p: AnalyticsEngineDataPoint) => points.push(p) },
} as Cloudflare.Env;

const latest = (query = "client=gui&version=23.0.0&os=linux&arch=amd64", ip = IP) =>
  worker.fetch(new Request(`${BASE}/v1/latest?${query}`, { headers: { "CF-Connecting-IP": ip } }), testEnv);
const request = (path: string, init: RequestInit = {}) => worker.fetch(new Request(`${BASE}${path}`, init), testEnv);

// The release lookup runs inside the Durable Object, so tests drive it there with a stand-in for GitHub.
const ok = (body: string) => async () => new Response(body, { headers: { "Content-Type": "application/json; charset=utf-8" } });
const fail = async () => new Response("rate limited", { status: 403 });
const upstream = (fetchFn: typeof fetch) => runInDurableObject(stub(), (instance) => instance.latest(fetchFn));

// Expires the cached release by moving its fetch time back past the TTL.
const expireCache = () =>
  runInDurableObject(stub(), async (_, state) => {
    const cached = await state.storage.get<{ body: string; fetchedAt: number }>("release");
    await state.storage.put("release", { ...cached, fetchedAt: Date.now() - CACHE_TTL_MS - 1 });
  });
const primeCache = (body: string) =>
  runInDurableObject(stub(), (_, state) => state.storage.put("release", { body, fetchedAt: Date.now() }));

const today = () => new Date().toISOString().slice(0, 10);

beforeEach(async () => {
  points.length = 0;
  // Storage isn't always reset between tests, so start each one empty.
  await runInDurableObject(stub(), (_, state) => state.storage.deleteAll());
});

describe("GitHub release lookup", () => {
  it("returns GitHub's JSON byte-for-byte, with the headers GitHub expects", async () => {
    const body = await upstream(async (url, init) => {
      const headers = new Headers(init?.headers);
      const expected = headers.get("User-Agent") && headers.get("Accept") === "application/vnd.github+json";
      return expected && url === "https://api.github.com/repos/elboletaire/manga-downloader/releases/latest"
        ? new Response(RELEASE)
        : new Response("bad request", { status: 400 });
    });
    expect(body).toBe(RELEASE);
  });

  it("caches the release for five minutes", async () => {
    expect(await upstream(ok(RELEASE))).toBe(RELEASE);
    expect(await upstream(ok(NEWER))).toBe(RELEASE);

    await expireCache();
    expect(await upstream(ok(NEWER))).toBe(NEWER);
  });

  it("serves the stale copy when GitHub fails", async () => {
    await upstream(ok(RELEASE));
    await expireCache();
    expect(await upstream(fail)).toBe(RELEASE);
  });

  it("serves the stale copy when GitHub is unreachable", async () => {
    await upstream(ok(RELEASE));
    await expireCache();
    expect(await upstream(() => Promise.reject(new TypeError("network down")))).toBe(RELEASE);
  });

  it("waits a minute before asking GitHub again after a failed refresh", async () => {
    await upstream(ok(RELEASE));
    await expireCache();
    expect(await upstream(fail)).toBe(RELEASE);
    expect(await upstream(ok(NEWER))).toBe(RELEASE);

    await expireCache();
    expect(await upstream(ok(NEWER))).toBe(NEWER);
  });

  it("shares one GitHub request between concurrent callers", async () => {
    const counts = await runInDurableObject(stub(), async (instance) => {
      let calls = 0;
      const slow = async () => {
        calls++;
        await new Promise((resolve) => setTimeout(resolve, 10));
        return new Response('{"tag_name":"v23.0.0"}');
      };
      const bodies = await Promise.all([instance.latest(slow), instance.latest(slow), instance.latest(slow)]);
      return { calls, bodies };
    });
    expect(counts.calls).toBe(1);
    expect(counts.bodies).toEqual(['{"tag_name":"v23.0.0"}', '{"tag_name":"v23.0.0"}', '{"tag_name":"v23.0.0"}']);
  });

  it("returns null when GitHub fails and nothing is cached", async () => {
    expect(await upstream(fail)).toBeNull();
  });
});

describe("GET /v1/latest", () => {
  it("answers with the cached release as JSON", async () => {
    await primeCache(RELEASE);
    const res = await latest();
    expect(res.status).toBe(200);
    expect(res.headers.get("Content-Type")).toBe("application/json");
    expect(await res.text()).toBe(RELEASE);
  });

  it("answers 502 when the release can't be had", async () => {
    const down = {
      UPDATE_CHECKS: testEnv.UPDATE_CHECKS,
      UPDATE_STATE: { getByName: () => ({ identify: async () => ({ day: today(), hash: "x" }), latest: async () => null }) },
    } as unknown as Cloudflare.Env;
    const res = await worker.fetch(new Request(`${BASE}/v1/latest`), down);
    expect(res.status).toBe(502);
  });

  it("clamps and sanitises the query params to 32 characters of [A-Za-z0-9._-]", async () => {
    await primeCache(RELEASE);
    await latest(`client=${"a".repeat(40)}&version=${encodeURIComponent("1.2 3/x")}&os=%3Cscript%3E&arch=`);
    expect(points[0].blobs?.slice(0, 4)).toEqual(["a".repeat(32), "1.23x", "script", "unknown"]);
  });

  it("records 'unknown' for missing params", async () => {
    await primeCache(RELEASE);
    await request("/v1/latest");
    expect(points[0].blobs?.slice(0, 4)).toEqual(["unknown", "unknown", "unknown", "unknown"]);
  });

  it("records the UTC day as the fifth blob and one as the double", async () => {
    await primeCache(RELEASE);
    await latest();
    expect(points[0].blobs?.[4]).toBe(today());
    expect(points[0].doubles).toEqual([1]);
  });

  it("records the app's language and the system locale as blobs 6 and 7", async () => {
    await primeCache(RELEASE);
    await latest("client=gui&version=23.0.0&os=linux&arch=amd64&lang=system&syslang=zh-Hant-TW");
    expect(points[0].blobs?.slice(5)).toEqual(["system", "zh-Hant-TW"]);
  });

  it("records '' for a missing language, so it isn't mistaken for 'unknown'", async () => {
    await primeCache(RELEASE);
    await latest("client=cli&version=23.0.0&os=linux&arch=amd64");
    expect(points[0].blobs?.slice(5)).toEqual(["", ""]);
  });

  it("clamps and sanitises the language params like the others", async () => {
    await primeCache(RELEASE);
    // "e<n>" is 4 characters and keeps "en" once the brackets are dropped, so 20 of them clamp to 16 pairs.
    await latest(`client=gui&lang=${encodeURIComponent("e<n>".repeat(20))}&syslang=${encodeURIComponent("pt BR/x")}`);
    expect(points[0].blobs?.slice(5)).toEqual(["en".repeat(16), "ptBRx"]);
  });
});

describe("the check's fields as headers", () => {
  beforeEach(() => primeCache(RELEASE));

  const withHeaders = (headers: Record<string, string>, query = "") =>
    worker.fetch(new Request(`${BASE}/v1/latest${query}`, { headers: { "CF-Connecting-IP": IP, ...headers } }), testEnv);

  it("records the X-MD-* headers", async () => {
    await withHeaders({
      "X-MD-Client": "gui",
      "X-MD-Version": "23.0.1",
      "X-MD-OS": "linux",
      "X-MD-Arch": "amd64",
      "X-MD-Lang": "system",
      "X-MD-Syslang": "fr-FR",
    });
    expect(points[0].blobs?.slice(0, 4)).toEqual(["gui", "23.0.1", "linux", "amd64"]);
    expect(points[0].blobs?.slice(5)).toEqual(["system", "fr-FR"]);
  });

  it("still records the query params of older clients", async () => {
    await latest("client=cli&version=23.0.0&os=linux&arch=amd64&syslang=de-DE");
    expect(points[0].blobs?.slice(0, 4)).toEqual(["cli", "23.0.0", "linux", "amd64"]);
    expect(points[0].blobs?.slice(5)).toEqual(["", "de-DE"]);
  });

  it("prefers a header over the query param of the same name", async () => {
    await withHeaders({ "X-MD-Client": "cli", "X-MD-Version": "23.0.1", "X-MD-Lang": "ja" }, "?client=gui&version=23.0.0&lang=en&os=linux&arch=amd64");
    expect(points[0].blobs?.slice(0, 2)).toEqual(["cli", "23.0.1"]);
    expect(points[0].blobs?.[5]).toBe("ja");
    expect(points[0].blobs?.slice(2, 4)).toEqual(["linux", "amd64"]);
  });

  it("sanitises and clamps the headers like the params", async () => {
    await withHeaders({ "X-MD-Client": "a".repeat(40), "X-MD-Syslang": "pt BR/x" });
    expect(points[0].blobs?.[0]).toBe("a".repeat(32));
    expect(points[0].blobs?.[6]).toBe("ptBRx");
  });

  it("records 'unknown' or '' for absent headers and params", async () => {
    await withHeaders({});
    expect(points[0].blobs?.slice(0, 4)).toEqual(["unknown", "unknown", "unknown", "unknown"]);
    expect(points[0].blobs?.slice(5)).toEqual(["", ""]);
  });
});

describe("recording failures", () => {
  beforeEach(() => primeCache(RELEASE));

  it("still answers when the data point can't be written", async () => {
    const broken = { ...testEnv, UPDATE_CHECKS: { writeDataPoint: () => { throw new Error("binding down"); } } } as Cloudflare.Env;
    const res = await worker.fetch(new Request(`${BASE}/v1/latest`), broken);
    expect(res.status).toBe(200);
    expect(await res.text()).toBe(RELEASE);
  });

  it("still answers when the salt can't be read", async () => {
    const broken = {
      ...testEnv,
      UPDATE_STATE: { getByName: () => ({ identify: async () => { throw new Error("do down"); }, latest: async () => RELEASE }) },
    } as unknown as Cloudflare.Env;
    const res = await worker.fetch(new Request(`${BASE}/v1/latest`), broken);
    expect(res.status).toBe(200);
    expect(await res.text()).toBe(RELEASE);
  });
});

describe("daily salt", () => {
  beforeEach(() => primeCache(RELEASE));

  it("gives the same IP the same hash on the same day, and different IPs different hashes", async () => {
    await latest();
    await latest();
    await latest("client=cli", "198.51.100.1");
    expect(points[0].indexes).toEqual(points[1].indexes);
    expect(points[0].indexes?.[0]).not.toEqual(points[2].indexes?.[0]);
    expect(points[0].indexes?.[0]).toMatch(/^[0-9a-f]{64}$/);
  });

  it("rotates the salt on a new day and deletes the old one", async () => {
    await latest();
    const oldHash = points[0].indexes?.[0];
    const oldSalt = await runInDurableObject(stub(), async (_, state) => {
      const salt = await state.storage.get<{ day: string; value: string }>("salt");
      // Pretend the stored salt belongs to yesterday, so the next request starts a new day.
      await state.storage.put("salt", { ...salt, day: "1999-01-01" });
      return salt?.value;
    });

    await latest();
    expect(points[1].blobs?.[4]).toBe(today());
    expect(points[1].indexes?.[0]).not.toEqual(oldHash);

    const stored = await runInDurableObject(stub(), (_, state) => state.storage.list());
    expect(JSON.stringify([...stored.entries()])).not.toContain(oldSalt);
  });

  it("drops the salt at the next UTC midnight, even without traffic", async () => {
    await latest();
    const now = new Date();
    const nextMidnight = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() + 1);
    expect(await runInDurableObject(stub(), (_, state) => state.storage.getAlarm())).toBe(nextMidnight);

    // The alarm runs after midnight, so the stored salt is yesterday's by then.
    await runInDurableObject(stub(), (_, state) => state.storage.put("salt", { day: "1999-01-01", value: "old" }));
    expect(await runDurableObjectAlarm(stub())).toBe(true);
    expect(await runInDurableObject(stub(), (_, state) => state.storage.get("salt"))).toBeUndefined();
  });

  it("keeps today's salt when the midnight alarm lands after a request already started the day", async () => {
    await latest();
    const salt = await runInDurableObject(stub(), (_, state) => state.storage.get("salt"));

    expect(await runDurableObjectAlarm(stub())).toBe(true);
    expect(await runInDurableObject(stub(), (_, state) => state.storage.get("salt"))).toEqual(salt);
  });

  it("never stores or records the raw IP", async () => {
    await latest();
    const stored = await runInDurableObject(stub(), (_, state) => state.storage.list());
    expect(JSON.stringify(points)).not.toContain(IP);
    expect(JSON.stringify([...stored.entries()])).not.toContain(IP);
  });
});

describe("other requests", () => {
  beforeEach(() => primeCache(RELEASE));

  it("answers HEAD without a body and without recording a check", async () => {
    const res = await request("/v1/latest", { method: "HEAD" });
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("");
    expect(points).toHaveLength(0);
  });

  it("answers OPTIONS with the allowed methods", async () => {
    const res = await request("/v1/latest", { method: "OPTIONS" });
    expect(res.status).toBe(204);
    expect(res.headers.get("Allow")).toBe("GET, HEAD");
  });

  it("rejects other methods on the endpoint", async () => {
    expect((await request("/v1/latest", { method: "POST" })).status).toBe(405);
  });

  it("404s everything else", async () => {
    expect((await request("/")).status).toBe(404);
    expect((await request("/v2/latest")).status).toBe(404);
    expect(points).toHaveLength(0);
  });
});
