from pathlib import Path
from typing import Any

import yt_dlp
from yt_dlp.utils import DownloadCancelled, DownloadError

from video_downloader.info import YDL_OPTS, to_api_error
from video_downloader.jobs import DownloadFailed, Job, JobCancelled, JobStatus


def format_options(preset: str) -> dict[str, Any]:
    if preset == "audio":
        return {
            "format": "ba[acodec^=aac]/ba[acodec^=mp4a.40.]/ba/b",
            "postprocessors": [{"key": "FFmpegExtractAudio", "preferredcodec": "m4a"}],
        }
    resolution = "res" if preset == "best" else f"res:{preset.removesuffix('p')}"
    return {
        "format": "bv*+ba/b",
        # Resolution first, then the codecs every player reads. On YouTube H.264 stops
        # at 1080p, so "best" on a 4K video is VP9 or AV1 inside the MP4.
        "format_sort": [resolution, "vcodec:h264", "acodec:aac"],
        "merge_output_format": "mp4",
        "postprocessors": [{"key": "FFmpegVideoRemuxer", "preferedformat": "mp4"}],
    }


def _expected_bytes(info: dict[str, Any]) -> int | None:
    formats = info.get("requested_formats") or [info]
    sizes = [f.get("filesize") or f.get("filesize_approx") for f in formats]
    return sum(sizes) if all(sizes) else None


class ProgressTracker:
    """Feeds a job from yt-dlp's hooks, and stops yt-dlp when the job is cancelled."""

    def __init__(self, job: Job):
        self._job = job
        self._finished_bytes = 0

    def on_download(self, update: dict[str, Any]) -> None:
        job = self._job
        if job.cancelled.is_set():
            raise DownloadCancelled()

        info = update.get("info_dict") or {}
        job.title = info.get("title") or job.title
        job.extractor = info.get("extractor_key") or job.extractor

        if update["status"] == "finished":
            self._finished_bytes += update.get("total_bytes") or update.get("downloaded_bytes") or 0
            return
        if update["status"] != "downloading":
            return

        # Video and audio are separate files: count across both when their sizes are
        # known, so the bar does not run to 100% twice.
        done = update.get("downloaded_bytes") or 0
        total = update.get("total_bytes") or update.get("total_bytes_estimate")
        if expected := _expected_bytes(info):
            done, total = self._finished_bytes + done, expected

        speed = update.get("speed")
        job.percent = min(100.0, done / total * 100) if total else None
        job.speed = speed
        job.eta = max(0.0, (total - done) / speed) if total and speed else None

    def on_postprocess(self, update: dict[str, Any]) -> None:
        job = self._job
        if job.cancelled.is_set():
            raise DownloadCancelled()
        # MoveFiles runs after every download, even when there is nothing to merge.
        if update["status"] == "started" and update["postprocessor"] != "MoveFiles":
            job.status = JobStatus.MERGING
            job.speed = job.eta = None


def download(job: Job, directory: Path) -> Path:
    progress = ProgressTracker(job)
    options = {
        **YDL_OPTS,
        **format_options(job.preset),
        "outtmpl": str(directory / "%(title).200B.%(ext)s"),
        "progress_hooks": [progress.on_download],
        "postprocessor_hooks": [progress.on_postprocess],
    }
    try:
        with yt_dlp.YoutubeDL(options) as ydl:
            info = ydl.extract_info(job.url, download=True)
    except DownloadCancelled as cancelled:
        raise JobCancelled() from cancelled
    except DownloadError as error:
        raise DownloadFailed(to_api_error(error).message) from error

    if info.get("_type") == "playlist":
        raise DownloadFailed("Playlists are not supported")
    return Path(info["requested_downloads"][0]["filepath"])
