import type { UpdateState } from "./state";

export { UpdateState } from "./state";

declare global {
  namespace Cloudflare {
    interface Env {
      UPDATE_CHECKS: AnalyticsEngineDataset;
      UPDATE_STATE: DurableObjectNamespace<UpdateState>;
    }
  }
}

const MAX_PARAM_LENGTH = 32;
const ALLOW = "GET, HEAD";

const text = (body: string, status: number, headers: HeadersInit = {}) =>
  new Response(body, { status, headers: { "Content-Type": "text/plain; charset=utf-8", ...headers } });

// Drops anything outside the charset, so a crafted value can't pollute the dataset.
// `missing` is what an empty value records: "" means "not sent" for the language fields.
const clean = (value: string | null, missing = "unknown") =>
  (value ?? "").replace(/[^A-Za-z0-9._-]/g, "").slice(0, MAX_PARAM_LENGTH) || missing;

// The X-MD-* header wins; the query param of the same name serves clients older than the headers.
const field = (request: Request, url: URL, name: string, missing = "unknown") =>
  clean(request.headers.get(`X-MD-${name}`) || url.searchParams.get(name.toLowerCase()), missing);

export default {
  async fetch(request: Request, env: Cloudflare.Env): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname !== "/v1/latest") return text("Not found", 404);
    if (request.method === "OPTIONS") return new Response(null, { status: 204, headers: { Allow: ALLOW } });
    if (request.method !== "GET" && request.method !== "HEAD") {
      return text("Method not allowed", 405, { Allow: ALLOW });
    }

    const state = env.UPDATE_STATE.getByName("updates");
    if (request.method === "GET") {
      // Recording must never fail the update check, so errors are dropped on purpose.
      try {
        const { day, hash } = await state.identify(request.headers.get("CF-Connecting-IP") ?? "");
        env.UPDATE_CHECKS.writeDataPoint({
          indexes: [hash],
          blobs: [
            field(request, url, "Client"),
            field(request, url, "Version"),
            field(request, url, "OS"),
            field(request, url, "Arch"),
            day,
            field(request, url, "Lang", ""),
            field(request, url, "Syslang", ""),
          ],
          doubles: [1],
        });
      } catch {}
    }

    const release = await state.latest();
    if (release === null) return text("Update check unavailable", 502);
    const headers = { "Content-Type": "application/json", "Cache-Control": "no-store" };
    return new Response(request.method === "HEAD" ? null : release, { headers });
  },
} satisfies ExportedHandler<Cloudflare.Env>;
