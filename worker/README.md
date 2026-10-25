# Update-check Worker

`manga-downloader-updates` answers the desktop app's and the CLI's update check. It serves GitHub's
`releases/latest` JSON unchanged, caches it for five minutes, and counts checks without keeping who made them.

## Endpoint

`GET /v1/latest?client=<gui|cli>&version=<version>&os=<GOOS>&arch=<GOARCH>`

- `200` with GitHub's release JSON, byte-for-byte, as `application/json`.
- `502` when GitHub fails and nothing is cached. Clients fall back to GitHub directly, so this is never fatal for them.
- Missing or odd params are fine: each one keeps `[A-Za-z0-9._-]` characters, is cut to 32, and becomes `unknown` when empty.
- `HEAD` answers like `GET` without a body and without counting. `OPTIONS` gets `204`. Other methods get `405`, other paths `404`.

## What is recorded

One Analytics Engine data point per `GET` (dataset `update_checks`):

| Field | Value |
| --- | --- |
| `index1` | hex SHA-256 of (the day's salt + the client IP) |
| `blob1`–`blob4` | `client`, `version`, `os`, `arch` (clamped as above) |
| `blob5` | UTC date, `YYYY-MM-DD` |
| `double1` | `1` |

The salt is 32 random bytes, created on the first request of each UTC day and held in the Durable Object.
The first request of a new day overwrites yesterday's salt, so the old one is gone and its hashes can't be matched to new ones.

**Not recorded:** the IP itself, any other header, the User-Agent, the path or query beyond the four fields above, request logs.
Observability is disabled in `wrangler.jsonc`, and the code never logs requests, headers, IPs or the salt.

## Layout

- `src/index.ts`: routing, parameter clamping, the data point.
- `src/state.ts`: the `UpdateState` Durable Object (SQLite): the daily salt and the cached release.
- `wrangler.jsonc`: bindings (`UPDATE_STATE`, `UPDATE_CHECKS`), the SQLite migration, observability off.
- `test/worker.test.ts`: the tests, run inside workerd with `@cloudflare/vitest-plugin`.

## Tests and local run

```sh
pnpm install
pnpm test        # vitest in workerd, no network: GitHub is stubbed
pnpm typecheck
pnpm dev         # wrangler dev on localhost, local bindings only
```

## Deploy

Deploys are manual. CI only runs the tests.

```sh
pnpm exec wrangler login        # or CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID
pnpm exec wrangler deploy
```

The first deploy creates the Durable Object namespace (from the migration) and the Analytics Engine dataset (created on its first write).

## Querying the counts

Analytics Engine SQL runs through the API, with a token that can read Analytics:

```sh
curl -s "https://api.cloudflare.com/client/v4/accounts/$CLOUDFLARE_ACCOUNT_ID/analytics_engine/sql" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  --data "SELECT blob5 AS day, count(DISTINCT index1) AS uniques, sum(_sample_interval) AS checks FROM update_checks GROUP BY day ORDER BY day DESC LIMIT 30"
```

`uniques` is distinct salted hashes per day. A hash changes at midnight UTC, so a user counts once per day, not once overall.
