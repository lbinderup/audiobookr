# Third-party notices

audioborker itself is licensed under the GNU General Public License v3.0 or
later — see [LICENSE](LICENSE). This file records the licenses of everything
it bundles, vendors, or ships alongside.

## Go dependencies (compiled into the binary)

All permissive; none impose copyleft on this project.

| Module | License |
|---|---|
| `modernc.org/sqlite` | BSD-3-Clause — © 2017 The Sqlite Authors |
| `modernc.org/libc` | BSD-3-Clause — © 2017 The Libc Authors |
| `modernc.org/memory` | BSD-3-Clause — © 2017 The Memory Authors |
| `modernc.org/mathutil` | BSD-3-Clause — © 2014 The mathutil Authors |
| `github.com/google/uuid` | BSD-3-Clause — © 2009, 2014 Google Inc. |
| `github.com/remyoudompheng/bigfft` | BSD-3-Clause — © 2012 The Go Authors |
| `golang.org/x/sys` | BSD-3-Clause — © 2009 The Go Authors |
| `github.com/dustin/go-humanize` | MIT — © 2005-2008 Dustin Sallings |
| `github.com/mattn/go-isatty` | MIT — © Yasuhiro MATSUMOTO |
| `github.com/ncruces/go-strftime` | MIT — © Nuno Cruces |

Full license texts are available in each module's repository and in the local
Go module cache (`go env GOMODCACHE`).

## Vendored browser assets (`internal/web/static/`)

| File | Project | License |
|---|---|---|
| `htmx.min.js` | [htmx](https://github.com/bigskysoftware/htmx) 2.0.7 | **0BSD** (Zero-Clause BSD) — no attribution required |
| `htmx-ext-sse.js` | [htmx-ext-sse](https://github.com/bigskysoftware/htmx-extensions) 2.2.3 | **0BSD** — © 2023 Alexander Petros |

0BSD grants permission to use, copy, modify and distribute for any purpose
without conditions; these entries are informational courtesy, not obligations.

## External programs (invoked as subprocesses, shipped in the Docker image)

audioborker calls these as separate processes via fork/exec with command-line
arguments — it does not link against them. Under the FSF's own reading
([GPL FAQ: MereAggregation](https://www.gnu.org/licenses/gpl-faq.html#MereAggregation),
[GPLPlugins](https://www.gnu.org/licenses/gpl-faq.html#GPLPlugins)) this keeps
them separate works.

| Program | License | Role |
|---|---|---|
| [FFmpeg](https://ffmpeg.org) (`ffmpeg`, `ffprobe`) | LGPL-2.1+ upstream; Alpine's build enables GPL components and `--enable-version3`, making it effectively **GPL-3.0-or-later** | probe, merge, transcode, verify |
| [tone](https://github.com/sandreas/tone) | Apache-2.0 — © sandreas | writes mp4 atoms and chapters |
| [fdkaac](https://github.com/nu774/fdkaac) (optional) | Zlib — © 2013-2014 nu774 (bundled parson: MIT; getopt: BSD-4-Clause; lpc: BSD-style, Xiph.Org) | optional AAC encoder |

### If you distribute the Docker image

Publishing the container image means **redistributing FFmpeg**, which Alpine
builds as GPL-3.0-or-later. You must then satisfy the GPL for that component:
ship the license text and provide corresponding source or a written offer for
it. Alpine's source packages (`https://gitlab.alpinelinux.org/alpine/aports`)
and FFmpeg's upstream release tarballs are the practical way to do this.
Since audioborker is itself GPL-3.0, the combination is license-consistent.

**Note on FDK-AAC:** the Fraunhofer FDK AAC library is *not* GPL-compatible
and grants no patent license, which is why no redistributable FFmpeg build
includes `libfdk_aac`. audioborker therefore uses the standalone `fdkaac`
program as an optional, separately-invoked encoder rather than linking it.

## Prior art

audioborker is an independent reimplementation inspired by
[bragibooks](https://github.com/djdembeck/bragibooks) and
[m4b-merge](https://github.com/djdembeck/m4b-merge) (both GPL-3.0). No code
was copied from either project; only publicly observable behaviour — API
request shapes, command-line usage, and reported bugs — informed this design.

Metadata is provided by [Audnexus](https://github.com/laxamentumtech/audnexus)
and Audible's public catalog API; neither is affiliated with this project.
