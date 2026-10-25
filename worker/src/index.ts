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
const param = (value: string | null) =>
  (value ?? "").replace(/[^A-Za-z0-9._-]/g, "").slice(0, MAX_PARAM_LENGTH) || "unknown";

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
            param(url.searchParams.get("client")),
            param(url.searchParams.get("version")),
            param(url.searchParams.get("os")),
            param(url.searchParams.get("arch")),
            day,
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
