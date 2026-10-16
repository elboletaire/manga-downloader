# Stats data

Written daily by the `Stats` workflow (`tools/stats-snapshot`).

- `snapshots/YYYY-MM-DD.json`: raw numbers of that day (release assets with their download counts, Docker Hub pulls, repo stars, forks, watchers and open issues).
- `daily.csv`: one row per snapshot, with the downloads split into CLI and app, and the change since the previous row.
