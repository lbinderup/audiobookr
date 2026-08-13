# audiobookr

Self-hosted audiobook converter for the *arr crowd. Point it at a folder of
raw audiobooks (mp3, m4a, m4b, flac, ogg, opus, wma, wav — anything ffmpeg
decodes), match each book against Audible/Audnexus, and get back a clean,
chapterized **`.m4b`** in your library — tagged with author, narrator, series,
cover art and an Audible ASIN, ready for Plex or Prologue.

A spiritual successor to [bragibooks](https://github.com/djdembeck/bragibooks),
rebuilt in Go around ffmpeg + [tone](https://github.com/sandreas/tone), with
the queue UX it always deserved.

## What it does

- **Import** — browse your input volume (newest first, junk like `@eaDir`
  and `.DS_Store` hidden), tick the books to process. Multi-disc layouts
  (`CD1`, `Disc 2`, …) and numbered files are merged in the right order
  (natural sort: `file_2` before `file_10`).
- **Match** — every selection is auto-searched against the Audible catalog
  with scored candidates (cover, narrator, runtime). Custom search, manual
  ASIN entry and per-book region selection included. Nothing is queued until
  you confirm.
- **Convert** — a persistent queue with **live progress**, per-job logs,
  cancel and retry. Jobs interrupted by a restart are marked as such — nothing
  ever hangs in "Processing" forever. Already-AAC sources are stream-copied
  without re-encoding; everything else is transcoded to AAC at the source
  bitrate (snapped to 64–320 kbps, overridable).
- **Chapters (always embedded)** — Audnexus chapter data when its runtime
  matches your audio (validated!), otherwise chapters from file boundaries
  with cleaned filename titles. A multi-file book never ends up chapterless.
  Optionally writes the classic `Book.chapters.txt` sidecar next to the m4b.
- **Output where you want it** — a dedicated `/output` volume with a safe
  path template (`{author}/{series_name}/{title}/{title} [{asin}]` by
  default; also `{narrator}`, `{subtitle}`, `{series_position}`, `{year}`).
  Missing variables drop cleanly — a standalone book simply skips its series
  folder level, with no ` - Title` stubs and no empty folders.
- **Source cleanup you control** — after a verified conversion: leave the
  sources, move them to a completed folder (validated to be *outside* the
  input tree), or delete them (only the audio files it consumed; strangers'
  files are left alone). Global default + per-book override.

## Quick start

```bash
docker build -t audiobookr .
```

Then adapt [docker-compose.example.yml](docker-compose.example.yml):

```yaml
services:
  audiobookr:
    image: audiobookr:latest
    environment:
      - PUID=1000   # must read /input and write /output
      - PGID=1000
      - TZ=Europe/Copenhagen
    volumes:
      - /appdata/audiobookr:/config
      - /downloads/audiobooks:/input
      - /library/audiobooks:/output
      - /downloads/processed:/completed   # optional, for "move" cleanup
    ports:
      - "8684:8684"
    restart: unless-stopped
```

Open `http://nas:8684`, check **Settings**, then **Import → Match → Queue**.

Multi-arch build (amd64 + arm64):

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t audiobookr:latest .
```

## Configuration

Runtime settings (directories, path template, cleanup mode, metadata region,
bitrate, encoder, worker concurrency, chapters.txt sidecar) live in the web
UI under **Settings** and persist in `/config/audiobookr.db`.

Environment variables (container-level):

| Variable | Default | Purpose |
|---|---|---|
| `PUID` / `PGID` | `1000` | Runtime user; use ids that own your media |
| `TZ` | `Etc/UTC` | Timestamps in logs/UI |
| `PORT` | `8684` | HTTP port |
| `CONFIG_DIR` / `INPUT_DIR` / `OUTPUT_DIR` | `/config` `/input` `/output` | Volume mount points |
| `CONVERTER` | `real` | `fake` simulates conversions (UI development) |
| `FFMPEG_PATH` / `FFPROBE_PATH` / `TONE_PATH` / `FDKAAC_PATH` | on `PATH` | Binary overrides |

### Encoders

The default encoder is ffmpeg's native `aac`, which is transparent for spoken
word at the 64k+ bitrates this app uses. The image also ships the standalone
`fdkaac` encoder (better psychoacoustics at low bitrates) — select it under
Settings → Conversion. No redistributable ffmpeg build may bundle libfdk_aac,
which is why it's a separate binary.

### Notes

- **No authentication.** This is a single-user LAN tool; put it behind your
  reverse proxy / VPN like the rest of your *arr stack. For SSE progress
  updates through nginx, disable proxy buffering for `/events`
  (`proxy_buffering off;`).
- `/config` should be local disk, not an NFS/SMB mount — SQLite WAL and
  network filesystems don't mix.
- Audible catalog search is an unofficial API. If it ever breaks, manual
  ASIN entry keeps working (metadata comes from Audnexus).
- Region matters: an ASIN from audible.co.uk does not exist in the `us`
  region. Set your default region in Settings and per-book on Match.

## Development

```bash
go run ./cmd/audiobookr   # http://localhost:8684
go test ./...
```

On a non-Linux dev machine the app defaults to `CONVERTER=fake` (simulated
conversions, full UI) and dev-mode template reloading. For the real pipeline
you need `ffmpeg`/`ffprobe` and [tone](https://github.com/sandreas/tone) on
the PATH (or `TONE_PATH`), then run with `CONVERTER=real`. Fixture books:
`testdata/make-fixtures.ps1`.

Architecture in one paragraph: `internal/scan` lists and orders input files;
`internal/metadata` talks to the Audible catalog (search) and Audnexus
(books + chapters) behind a provider interface; `internal/match` normalizes
names and scores candidates; `internal/queue` runs a persistent job queue
(SQLite via `internal/store`) with worker goroutines and an SSE broker;
`internal/pipeline` probes with ffprobe, merges/transcodes with ffmpeg,
resolves chapters, tags with tone, then verifies the result with ffprobe
before moving it into the library; `internal/web` is a server-rendered
html/template + htmx UI. The `pipeline.Converter` interface separates the
queue from the conversion, which is what makes the fake converter possible.
"# audiobookr" 
