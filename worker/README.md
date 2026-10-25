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

One Analytics Engine data point per `GET` (dataset `manga_downloader_checks`):

| Field | Value |
| --- | --- |
| `index1` | hex SHA-256 of (the day's salt + the client IP) |
| `blob1`–`blob4` | `client`, `version`, `os`, `arch` (clamped as above) |
| `blob5` | UTC date, `YYYY-MM-DD` |
| `double1` | `1` |

The salt is 32 random bytes, created on the first request of each UTC day and held in the Durable Object.
The first request of a new day overwrites yesterday's salt, and an alarm set for the next UTC midnight deletes it even without traffic.
So the code never keeps a salt past its day, and hashes from different days can't be linked.

Honest limit: SQLite-backed Durable Objects keep about 30 days of point-in-time recovery, so the account owner could restore a deleted salt from there.
The deletion is what the code does, not an unrecoverable erase.

**Not recorded:** the IP itself, any other header, the User-Agent, the path or query beyond the four fields above, request logs.
Observability is disabled in `wrangler.jsonc`, and the code never logs requests, headers, IPs or the salt.

## Layout

- `src/index.ts`: routing, parameter clamping, the data point.
- `src/state.ts`: the `UpdateState` Durable Object (SQLite): the daily salt and the cached release.
- `wrangler.jsonc`: bindings (`UPDATE_STATE`, `UPDATE_CHECKS`), the SQLite migration, observability off.
- `test/worker.test.ts`: the tests, run inside workerd with `@cloudflare/vitest-plugin`.

## Tests and local run

```sh
make test/worker   # from the repo root: install, pnpm test, pnpm typecheck (what CI runs)
pnpm test          # in worker/: vitest in workerd, no network, GitHub is stubbed
pnpm dev           # wrangler dev on localhost, local bindings only
```

## Deploy

The first deploy is by hand, from `worker/`, so the Durable Object namespace (from the migration) exists before CI touches it:

```sh
pnpm exec wrangler login        # or export CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID
pnpm exec wrangler deploy
```

After that, `worker-deploy.yaml` deploys on every push to master that touches `worker/` (or that workflow), and it can be run by hand from master. It runs `make test/worker` first, then `make deploy/worker`.
It needs two repository secrets:

- `CLOUDFLARE_API_TOKEN`: a token from the "Edit Cloudflare Workers" template, scoped to the one account.
- `CLOUDFLARE_ACCOUNT_ID`: the account's ID.

The Analytics Engine dataset is created on the first write, so it needs no setup.

## Custom domain (`releases.mangadownloader.cat`)

The apps ask `https://releases.mangadownloader.cat`, which isn't routed yet because the zone isn't active. `workers_dev` stays on, so `manga-downloader-updates.elboletaire.workers.dev` keeps working.
Once the zone is active, add this to `wrangler.jsonc` (next to `workers_dev`) and deploy:

```jsonc
"routes": [{ "pattern": "releases.mangadownloader.cat", "custom_domain": true }]
```

Until then the apps fall back to GitHub, so the missing route costs nothing but the count.

## Querying the counts

Analytics Engine SQL runs through the API, with a token that can read Analytics:

```sh
curl -s "https://api.cloudflare.com/client/v4/accounts/$CLOUDFLARE_ACCOUNT_ID/analytics_engine/sql" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  --data "SELECT blob5 AS day, count(DISTINCT index1) AS uniques, sum(_sample_interval) AS checks FROM manga_downloader_checks GROUP BY day ORDER BY day DESC LIMIT 30"
```

`uniques` is distinct salted hashes per day. A hash changes at midnight UTC, so a user counts once per day, not once overall.
