# Running audiobookr on a QNAP NAS (Container Station)

This gets audiobookr running next to your other *arr apps in Container
Station, pulling a published image the same way they do.

## 1. Publish the image (once)

The repository builds a multi-arch image (amd64 + arm64) in GitHub Actions and
pushes it to GitHub's container registry.

1. Push to `main` — the **Build and publish image** workflow runs automatically.
   Watch it under the repo's **Actions** tab; it takes a few minutes (the arm64
   layer builds under emulation).
2. When it finishes, the image exists at
   `ghcr.io/lbinderup/audiobookr:latest`.
3. **Make the package public** so Container Station can pull it without
   credentials: GitHub → your profile → **Packages** → `audiobookr` →
   *Package settings* → **Change visibility** → Public.

   Prefer to keep it private? Then in Container Station go to
   **Preferences → Registry → Add**, URL `https://ghcr.io`, username = your
   GitHub username, password = a Personal Access Token with `read:packages`.

> Building on the NAS instead: SSH in, `git clone` the repo, and run
> `docker build -t audiobookr:latest .`. It works, but it's slow on NAS
> hardware and needs the CLI — the registry route is easier to keep updated.

## 2. Find your PUID / PGID

audiobookr runs as an unprivileged user inside the container. That user must
own (or be able to read/write) your media folders, or conversions fail with
permission errors.

SSH into the NAS and check who owns the media:

```bash
ls -ln /share/Multimedia/Audiobooks     # the numbers in cols 3 and 4 are uid and gid
id your-username                        # uid/gid for a specific account
```

Use those numbers as `PUID`/`PGID`. On QTS the `everyone` group is commonly
`100`; regular users start at `1000`. Don't guess — read them off the share.

## 3. Create the application

Container Station → **Applications** → **Create** → give it the name
`audiobookr`, then paste the YAML from
[`docker-compose.qnap.yml`](../docker-compose.qnap.yml), editing:

- the four host paths (`/share/...`) to match your actual shares,
- `PUID` / `PGID` from step 2,
- `TZ` if you're not in Copenhagen.

Notes on the paths:

| Container path | What it is | Requirements |
|---|---|---|
| `/config` | SQLite database + job logs | **Local disk only.** SQLite's WAL journal breaks on NFS/SMB mounts. `/share/Container/...` is fine. |
| `/input` | Where raw audiobooks arrive | Read access; write access too if you use the move/delete cleanup modes. |
| `/output` | Your audiobook library | Write access. Point it at the folder Plex/Prologue already scans. |
| `/completed` | Optional | Only used by the "move" cleanup mode. Must be **outside** `/input` — the app enforces this. |

Click **Create**. Container Station pulls the image and starts it.

## 4. First run

Open `http://<nas>:8684` and go to **Settings** first. The defaults already
match the volume layout above, but confirm:

- **Output path template** — default
  `{author}/{series_name}/{title}/{title} [{asin}]`; the preview shows both a
  series book and a standalone as you type.
- **Region** — `us` unless your ASINs come from another Audible storefront.
- **Cleanup mode** — starts at "leave sources in place"; switch to move/delete
  once you trust it.

Then **Import → Match → Queue**. Watch the first conversion's log on the job
detail page; a working run ends with a `verified: …, N chapters` line.

## 5. Updating

Container Station → Applications → `audiobookr` → **Recreate** (or *Pull* then
restart) fetches the newest `:latest`. Your database and settings live in
`/config`, so they survive updates.

To pin a specific version instead of tracking `latest`, tag a release
(`git tag v1.0.0 && git push --tags`) and use
`ghcr.io/lbinderup/audiobookr:1.0.0` in the compose file.

## Troubleshooting

**Permission denied / read-only errors in a job log** — `PUID`/`PGID` don't
match your share ownership. Recheck step 2. The container deliberately never
chowns `/input` or `/output`; it adapts to your ids instead of rewriting your
library's permissions.

**"database is locked" or corruption on startup** — `/config` is on a network
mount. Move it to local storage.

**Progress bars don't update behind a reverse proxy** — the UI streams updates
over Server-Sent Events. Add `proxy_buffering off;` for `/events` in nginx.
Direct access on port 8684 is unaffected.

**A book fails with "no supported audio files"** — the folder contains formats
ffmpeg can't decode, or only non-audio files. Check the job log for what it saw.

**Container won't pull** — the GHCR package is still private; make it public or
add the registry credentials (step 1).

## A note on licensing

The published image bundles FFmpeg, which Alpine builds under GPL-3.0-or-later.
Distributing the image means distributing FFmpeg, so if you share it beyond
your own NAS, include the license text and a source offer. Running it privately
carries no such obligation. See [THIRD-PARTY-NOTICES.md](../THIRD-PARTY-NOTICES.md).
