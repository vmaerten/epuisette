# Changelog

## 0.2.0

### ✨ Features

- Open **À propos d'Épuisette** from the app menu or the footer: who makes the app, its version, and links to sponsor it on GitHub, buy its author a coffee, read the code or report a problem. **Soutenir Épuisette…** in the app menu goes straight to GitHub Sponsors. ([#33](https://github.com/vmaerten/epuisette/pull/33))

### 📚 Documentation

- The install instructions run `brew trust vmaerten/tap` first: Homebrew now loads casks from a third-party tap only once it is trusted. ([#31](https://github.com/vmaerten/epuisette/pull/31))

### 🔧 Internal

- The repository page shows a Sponsor button, with GitHub Sponsors and Buy Me a Coffee. ([#32](https://github.com/vmaerten/epuisette/pull/32))
- Pin the Wails CLI in `.mise.toml`, so `task bindings` works with the pinned Go. ([#34](https://github.com/vmaerten/epuisette/pull/34))

## 0.1.0

### ✨ Features

- Paste a link from YouTube, X or any site yt-dlp supports, see its title, thumbnail and duration, and pick a format: the best quality, each resolution step from 1440p down to 360p that the video really has, or audio only. ([#23](https://github.com/vmaerten/epuisette/pull/23), [#20](https://github.com/vmaerten/epuisette/pull/20))
- Follow each download with its percent, speed and time left, across the video and audio files, then the merge. Two downloads run at a time and the others wait; cancel or retry any of them from its row. ([#25](https://github.com/vmaerten/epuisette/pull/25), [#26](https://github.com/vmaerten/epuisette/pull/26))
- Finished files land straight in the Downloads folder, next to existing files rather than over them, and **Afficher dans le Finder** selects them. ([#26](https://github.com/vmaerten/epuisette/pull/26))
- A banner names yt-dlp, ffmpeg or deno when one is missing, with the `brew install` command to run. ([#27](https://github.com/vmaerten/epuisette/pull/27))

### 📦 Distribution

- Épuisette is a macOS app: it runs only while its window is open, starts no server and opens no network port. ([#22](https://github.com/vmaerten/epuisette/pull/22), [#29](https://github.com/vmaerten/epuisette/pull/29))
- Install it with `brew install --cask vmaerten/tap/epuisette`, which also installs yt-dlp, ffmpeg and deno. ([#30](https://github.com/vmaerten/epuisette/pull/30))
