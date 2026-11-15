# video-downloader

Personal web app to download videos with [yt-dlp](https://github.com/yt-dlp/yt-dlp): paste a URL, pick a format, get the file. The interface is in French.

It is meant for one person on their own machine. There is no authentication, so it listens on localhost only: do not expose it to the internet.

## Requirements

- [uv](https://docs.astral.sh/uv/), which installs Python 3.13 and the dependencies.
- [ffmpeg](https://ffmpeg.org/), to merge the video and audio streams and to convert audio.
- [deno](https://deno.com/), the JavaScript runtime yt-dlp looks for by default to read YouTube's player.
- [task](https://taskfile.dev/), optional, for the shortcuts below.

On macOS: `brew install uv ffmpeg deno go-task`.

## Run

```sh
uv sync
task dev
```

Then open http://127.0.0.1:8787. Without `task`:

```sh
uv run uvicorn video_downloader.main:app --port 8787
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `DOWNLOADS_DIR` | `./downloads` | Where files are written, one sub-directory per download. |
| `DOWNLOADS_MAX_AGE_DAYS` | `7` | Finished downloads older than this are deleted, at startup and every hour. |

Only sub-directories named like a download id are ever deleted, so `DOWNLOADS_DIR` can point at a folder that holds other things.

## Formats

| Preset | Result |
|---|---|
| Best quality | Highest resolution, in an MP4. Above 1080p on YouTube this is VP9 or AV1, which some players such as QuickTime refuse. |
| 1440p down to 360p | The resolutions the video really has, one per step. H.264 and AAC in an MP4 when the site offers them. |
| Audio only | AAC in an M4A. |

Playlists are not supported.

## Limits

- Downloads are tracked in memory. After a restart the list is empty; the files stay in `DOWNLOADS_DIR` until they expire.
- Two downloads run at a time, the others wait.
- Sites change often. When a link that used to work fails, update yt-dlp first: `uv lock --upgrade-package yt-dlp && uv sync`.

## Development

```sh
task lint   # ruff check and format check
task fmt    # fix and format
task test   # pytest
```

## From the command line

There is no custom CLI: yt-dlp is the CLI. Install it with `brew install yt-dlp`, then these aliases give the same results as the app's presets:

```sh
# 1080p H.264 MP4, like the app's "1080p" preset. Replace 1080 with 720, or drop ":1080" for the best quality.
alias ytd='yt-dlp -f "bv*+ba/b" -S "res:1080,vcodec:h264,acodec:aac" --merge-output-format mp4 --remux-video mp4 --no-playlist'

# Audio only, AAC in an M4A.
alias yta='yt-dlp -f "ba[acodec^=aac]/ba[acodec^=mp4a.40.]/ba/b" -x --audio-format m4a --no-playlist'
```

```sh
ytd "https://www.youtube.com/watch?v=..."
```
