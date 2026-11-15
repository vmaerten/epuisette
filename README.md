# Épuisette

macOS app to download videos with [yt-dlp](https://github.com/yt-dlp/yt-dlp): paste a link, pick a format, and the file lands in your Downloads folder. The interface is in French.

It runs only while its window is open, starts no server and opens no network port. It is built with [Wails](https://wails.io) v3: a Go binary that shows the page in the system's WebKit.

## Install

On a Mac with Apple Silicon and macOS 12 or later:

```sh
brew trust vmaerten/tap
brew install --cask vmaerten/tap/epuisette
```

Homebrew loads casks from a third-party tap only once you trust it, hence the first line.

This also installs what the app runs: [yt-dlp](https://github.com/yt-dlp/yt-dlp), [ffmpeg](https://ffmpeg.org/) to merge and convert, and [deno](https://deno.com/), which yt-dlp uses to read YouTube's player. The app looks for them in the Homebrew folders and shows a banner at startup if one is missing.

The app is signed ad hoc, not notarised; the cask removes the quarantine flag so that it opens without a Gatekeeper prompt. The first download may make macOS ask for access to the Downloads folder.

## Build from source

You need Go 1.26 or later, the Xcode command line tools, [task](https://taskfile.dev/), the Wails CLI, and the three tools above:

```sh
brew install go go-task yt-dlp ffmpeg deno
xcode-select --install
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
task install
```

`task install` builds the app, signs it ad hoc and copies it to `~/Applications/Épuisette.app`.

## Formats

| Preset | Result |
|---|---|
| Best quality | Highest resolution, in an MP4. Above 1080p on YouTube this is VP9 or AV1, which some players such as QuickTime refuse. |
| 1440p down to 360p | The resolutions the video really has, one per step. H.264 and AAC in an MP4 when the site offers them. |
| Audio only | AAC in an M4A. |

Playlists are not supported.

## Limits

- Two downloads run at a time, the others wait.
- The list lives in memory: it is empty again after a restart, and the files stay in the Downloads folder. Quitting the app cancels the downloads still running.
- A file never overwrites another: a second download with the same name gets " (1)" added.
- Sites change often. When a link that used to work fails, update yt-dlp first: `brew upgrade yt-dlp`.

## Development

```sh
task dev        # build and open the app from the sources
task test       # go test
task lint       # gofmt and go vet
task bindings   # regenerate frontend/bindings after changing a Service method
task package    # build bin/Épuisette.app
```

The page in `frontend/` is plain HTML, CSS and JavaScript, with no npm and no build step. It calls the Go methods of `Service` through the generated `frontend/bindings`, which are committed.

`internal/ytdlp` runs yt-dlp; `VD_LIVE=1 go test ./internal/ytdlp -run Live` checks it against YouTube. `internal/jobs` queues the downloads.

## Changelog and releases

Each PR that changes what a user sees adds its entry to `## Unreleased` in `CHANGELOG.md`, under one of these headings, in this order: `### 💥 Breaking changes`, `### ✨ Features`, `### 🐛 Fixes`, `### 📦 Distribution`, `### 📚 Documentation`, `### 🔧 Internal`.

To release, rename `## Unreleased` to `## X.Y.Z` in a commit on `main`, then tag that commit and push the tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The `Release` workflow then runs `task release` on a macOS runner:

1. builds `Épuisette.app` stamped with the version, zips it into `epuisette_X.Y.Z_darwin_arm64.zip` with `checksums.txt`, and checks the checksum, the signature and the version of the extracted app;
2. publishes a GitHub release whose notes are that `CHANGELOG.md` section, as a draft until both files are uploaded;
3. points `Casks/epuisette.rb` in [vmaerten/homebrew-tap](https://github.com/vmaerten/homebrew-tap) at the new archive, unless the tap already offers a newer version.

It needs the `HOMEBREW_TAP_TOKEN` secret: a fine-grained token with `contents: write` on `vmaerten/homebrew-tap` only. An existing release is never overwritten. If the cask push fails after publication, rerun it alone with `task release:tap TAG=v0.1.0`. A prerelease (`v0.2.0-rc.1`) is published from a Mac with `task release TAG=v0.2.0-rc.1` and leaves the cask alone.

`task release:build TAG=vX.Y.Z` builds and checks the archive in `dist/` without publishing anything. Go and Task are pinned in `.mise.toml`, with checksums in `mise.lock`.

## From the command line

There is no custom CLI: yt-dlp is the CLI. These aliases give the same results as the app's presets:

```sh
# 1080p H.264 MP4, like the app's "1080p" preset. Replace 1080 with 720, or drop ":1080" for the best quality.
alias ytd='yt-dlp -f "bv*+ba/b" -S "res:1080,vcodec:h264,acodec:aac" --merge-output-format mp4 --remux-video mp4 --no-playlist'

# Audio only, AAC in an M4A.
alias yta='yt-dlp -f "ba[acodec^=aac]/ba[acodec^=mp4a.40.]/ba/b" -x --audio-format m4a --no-playlist'
```

```sh
ytd "https://www.youtube.com/watch?v=..."
```
