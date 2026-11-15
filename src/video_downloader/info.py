import re
from typing import Any

import yt_dlp
from pydantic import BaseModel
from yt_dlp.utils import DownloadError, ExtractorError, UnsupportedError

from video_downloader.errors import ApiError

YDL_OPTS = {
    "quiet": True,
    "no_warnings": True,
    "noplaylist": True,
    # Without this a playlist URL resolves every entry before we can reject it.
    "extract_flat": "in_playlist",
    "socket_timeout": 15,
}

# One preset per step of the usual ladder, each named after the resolution it really gives.
RESOLUTION_CAPS = (1440, 1080, 720, 480, 360)
PRESET_ID = re.compile(r"best|audio|\d{3,4}p")


class Preset(BaseModel):
    id: str
    resolution: int | None


class VideoInfo(BaseModel):
    title: str | None
    thumbnail: str | None
    duration: float | None
    uploader: str | None
    extractor: str | None
    presets: list[Preset]


def _resolution(fmt: dict[str, Any]) -> int | None:
    # Smallest dimension, like yt-dlp's `res` sort: a vertical 1080x1920 video is 1080p.
    dimensions = [d for d in (fmt.get("width"), fmt.get("height")) if d]
    return min(dimensions) if dimensions else None


def build_presets(formats: list[dict[str, Any]]) -> list[Preset]:
    # yt-dlp reports a missing stream as the string "none" and an unknown codec as None.
    # Storyboards have dimensions but neither stream.
    videos = [f for f in formats if f.get("vcodec") != "none"]
    has_audio = any(f.get("acodec") != "none" for f in formats)

    presets = []
    if videos:
        resolutions = [r for f in videos if (r := _resolution(f))]
        best = max(resolutions, default=None)
        presets.append(Preset(id="best", resolution=best))
        offered = {best}
        for cap in RESOLUTION_CAPS:
            capped = max((r for r in resolutions if r <= cap), default=None)
            if capped and capped not in offered:
                presets.append(Preset(id=f"{capped}p", resolution=capped))
                offered.add(capped)
    if has_audio:
        presets.append(Preset(id="audio", resolution=None))
    return presets


def to_api_error(error: DownloadError) -> ApiError:
    # exc_info holds the extractor's own error, or the network error it wrapped.
    cause = error.exc_info[1] if error.exc_info else None
    if isinstance(cause, UnsupportedError):
        return ApiError(422, "unsupported_url", "No video found at this address")
    if isinstance(cause, ExtractorError) and cause.expected:
        return ApiError(404, "unavailable", cause.orig_msg)
    return ApiError(502, "extraction_failed", str(error).removeprefix("ERROR: "))


def require_web_url(url: str) -> None:
    # yt-dlp also accepts search prefixes and bare words; only real links are allowed here.
    if not url.startswith(("http://", "https://")):
        raise ApiError(422, "unsupported_url", "Not a web address")


def require_preset(preset: str) -> None:
    if not PRESET_ID.fullmatch(preset):
        raise ApiError(422, "invalid_preset", "Unknown format")


def fetch_info(url: str) -> VideoInfo:
    require_web_url(url)

    try:
        with yt_dlp.YoutubeDL(YDL_OPTS) as ydl:
            info = ydl.extract_info(url, download=False)
    except DownloadError as error:
        raise to_api_error(error) from error

    if info.get("_type") == "playlist":
        raise ApiError(422, "playlist", "Playlists are not supported")

    return VideoInfo(
        title=info.get("title"),
        thumbnail=info.get("thumbnail"),
        duration=info.get("duration"),
        uploader=info.get("uploader"),
        extractor=info.get("extractor_key"),
        presets=build_presets(info.get("formats") or []),
    )
