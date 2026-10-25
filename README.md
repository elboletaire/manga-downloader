# Stats data

Written daily by the `Stats` workflow ([github-stats-action](https://github.com/elboletaire/github-stats-action)).

- [`dashboard/`](dashboard/index.html): the dashboard, a standalone HTML page.
- `elboletaire/manga-downloader/`: the data of this repo.
  - `snapshots/YYYY-MM-DD.json`: raw numbers of that day (release assets with their download counts, Docker Hub pulls, stars and forks).
  - `daily.csv`: one row per snapshot, with the totals and the change since the previous one.
  - `dataset.json`: everything the dashboard, charts and badges are built from.
  - `stargazers.json`: when each star was given, for the star history.
  - `charts/`: the SVG charts for the README, in light and dark.
  - `badges/`: the badges, as SVG and as shields.io endpoint JSON.
