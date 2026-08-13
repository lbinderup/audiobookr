# AGENTS.md

Guidance for AI coding agents working in this repository. Humans: see
[README.md](README.md) for what the app does and how to run it.

## What this is

A single-binary Go web app that converts audiobooks into standardized,
chapterized `.m4b` files with metadata from Audible/Audnexus. It is a
replacement for [bragibooks](https://github.com/djdembeck/bragibooks) and
deploys as a Docker container on a NAS alongside the *arr suite.

## Architecture

```
cmd/audiobookr/main.go   wiring, graceful shutdown, converter selection
internal/
  config/     env-derived process config (paths, ports, binary locations)
  store/      SQLite (modernc.org/sqlite, CGO off): settings, jobs, metadata cache
  metadata/   Provider interface; audible/ = catalog search, audnexus/ = books + chapters
  match/      filename normalization + candidate scoring (pure functions)
  scan/       input listing, junk filtering, natural sort, disc ordering, selection dedupe
  pathtmpl/   {author}/{title} output templates with safe segment dropping
  pipeline/   Converter interface; real.go = ffmpeg+tone, fake.go = simulation
  queue/      worker pool, job state machine, cancellation registry, SSE broker
  web/        net/http mux, handlers, embedded html/template + htmx assets
```

Data flow: **Import** (pick files) → **Match** (choose ASIN) → **Queue**
(persistent jobs) → **pipeline** (probe → plan → merge → chapters → tag →
move → verify → cleanup).

## Non-obvious invariants — do not break these

- **`pipeline.Converter` is the seam.** `FakeConverter` lets the entire web UX
  run on a machine without ffmpeg/tone. Keep the real and fake implementations
  interchangeable; never let queue code reach into ffmpeg specifics.
- **Jobs snapshot their inputs.** `metadata_json` / `options_json` on a job are
  point-in-time copies, so retries reproduce exactly and settings edits never
  mutate in-flight jobs. Do not "helpfully" re-read live settings in the pipeline.
- **Chapter decisions are computed in exactly one place.** The match screen's
  verdict line calls `pipeline.PlanChapters`, which calls the same
  `resolveChapters` the conversion runs. Never fork this logic for display.
- **Multi-file input must never produce zero chapters.** Fallback order is
  provider (runtime-validated) → file's own → file boundaries → single chapter.
- **ffmpeg concat lists need absolute paths.** The demuxer resolves relative
  entries against the list file's directory, not the working directory.
- **tone args must be `--flag=value` single tokens.** Values starting with `-`
  (the `----:` freeform atom prefix) break its parser otherwise, and
  `--meta-recording-date` needs a full date, not a bare year.
- **Never write scratch files into the input tree** (bragibooks wrote cover.jpg
  there). Covers/concat lists live in `/config/work/{jobID}`; the staged m4b
  lives under the output volume so the final move is an atomic rename.
- **Path templates drop empty segments.** A standalone book skips its
  `{series_name}` folder; separators collapse. Substitution is single-pass so
  values containing `{token}` text are never re-substituted.
- **`/config` must be local disk.** SQLite WAL breaks on NFS/SMB.
- **Create files with permissive modes (`0o666` / `0o777`), never `0o644`.**
  The container sets a umask (default 002) and the OS subtracts from these
  modes; a umask can only clear bits, so hardcoding 0644 would make output
  files non-group-writable no matter how the NAS is configured — which breaks
  shared media libraries where a human account must also manage the files.
- **Selections are deduped** (`scan.DedupeSelection`): choosing a folder and a
  file inside it is one job, not two. Enforced server-side, not just in the UI.

## Conventions

- Server-rendered `html/template` + htmx. **No build step, no npm.** htmx and
  its SSE extension are vendored in `internal/web/static/`.
- SSE events carry pre-rendered HTML fragments; the broker itself knows nothing
  about templates. Slow subscribers drop events (progress is idempotent).
- Keep the dependency list tiny — currently only `modernc.org/sqlite` and
  `github.com/google/uuid`. Prefer stdlib; justify any addition.
- Comments explain *why*, not *what*. Several comments reference specific
  bragibooks bugs this code exists to avoid — keep that context if you edit them.
- Errors surfaced to the user should say what to do about it, not just what failed.

## Development

```bash
go run ./cmd/audiobookr    # http://localhost:8684
go test ./...
gofmt -l .                 # must be empty
go vet ./...
```

On non-Linux dev machines the app enables template hot-reloading and
auto-detects the toolchain: it uses the real pipeline when ffmpeg, ffprobe and
tone all resolve, otherwise falls back to `CONVERTER=fake`. Override with
`CONVERTER`, `FFMPEG_PATH`, `FFPROBE_PATH`, `TONE_PATH`, `FDKAAC_PATH`.

Audio fixtures for pipeline work: `testdata/make-fixtures.ps1` generates
sine-tone books (multi-mp3, multi-disc m4a, single m4b).

`devdata/` is git-ignored scratch space: `devdata/config` (db + logs),
`devdata/input`, `devdata/output`, `devdata/tools` (drop `tone.exe` here and
it is picked up automatically).

## Testing expectations

- Pure logic (`pathtmpl`, `scan`, `match`, chapter resolution, bitrate snapping)
  is table-driven unit tested. Add cases there rather than testing via HTTP.
- API clients are tested against `httptest` servers using fixtures captured
  from the real Audnexus responses — keep those shapes accurate.
- Changes visible in the browser should be verified by actually driving the
  running app, not assumed. Real conversions can be verified with
  `tone dump <file> --format json --query '$.meta.chapters'`.

## External tools (runtime dependencies, invoked as subprocesses)

`ffmpeg`/`ffprobe` (decode, merge, transcode, verify), `tone` (writes the mp4
atoms ffmpeg cannot: narrator, series, `stik`, freeform ASIN, chapters in both
Nero and QuickTime formats), optional `fdkaac`. These are separate processes —
never link against them, and keep invocation details inside `internal/pipeline`.
