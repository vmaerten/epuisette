import logging
import shutil
import threading
import uuid
from collections.abc import Callable
from concurrent.futures import Future, ThreadPoolExecutor
from dataclasses import dataclass, field
from enum import StrEnum
from pathlib import Path

logger = logging.getLogger(__name__)

MAX_CONCURRENT_DOWNLOADS = 2


class JobStatus(StrEnum):
    QUEUED = "queued"
    DOWNLOADING = "downloading"
    MERGING = "merging"
    DONE = "done"
    FAILED = "failed"


class JobCancelled(Exception):
    pass


class DownloadFailed(Exception):
    """A failure whose message is fit to show to the user."""


@dataclass
class Job:
    id: str
    url: str
    preset: str
    status: JobStatus = JobStatus.QUEUED
    title: str | None = None
    extractor: str | None = None
    percent: float | None = None
    speed: float | None = None
    eta: float | None = None
    size: int | None = None
    error: str | None = None
    file: Path | None = None
    cancelled: threading.Event = field(default_factory=threading.Event)
    future: Future | None = None


# Downloads the job into the given directory and returns the final file.
# Must raise JobCancelled soon after `job.cancelled` is set.
Downloader = Callable[[Job, Path], Path]


class JobStore:
    """In-memory jobs, run by a small thread pool. Each job owns `directory/<job id>`."""

    def __init__(self, directory: Path, download: Downloader):
        self._directory = directory
        self._download = download
        self._jobs: dict[str, Job] = {}
        self._lock = threading.Lock()
        self._pool = ThreadPoolExecutor(max_workers=MAX_CONCURRENT_DOWNLOADS)

    def create(self, url: str, preset: str) -> Job:
        job = Job(id=uuid.uuid4().hex, url=url, preset=preset)
        with self._lock:
            self._jobs[job.id] = job
            job.future = self._pool.submit(self._run, job)
        return job

    def list(self) -> list[Job]:
        with self._lock:
            return list(self._jobs.values())

    def get(self, job_id: str) -> Job | None:
        with self._lock:
            return self._jobs.get(job_id)

    def delete(self, job_id: str) -> bool:
        with self._lock:
            job = self._jobs.pop(job_id, None)
            if job is None:
                return False
            job.cancelled.set()
            finished = job.status in (JobStatus.DONE, JobStatus.FAILED)
            # A running job removes its own directory, once yt-dlp has let go of the files.
            if finished or job.future.cancel():
                self._remove_files(job)
        return True

    def close(self) -> None:
        with self._lock:
            running = list(self._jobs.values())
            self._jobs.clear()
        for job in running:
            job.cancelled.set()
        self._pool.shutdown(wait=False, cancel_futures=True)

    def _run(self, job: Job) -> None:
        error = None
        try:
            if not job.cancelled.is_set():
                job.status = JobStatus.DOWNLOADING
                job.file = self._download(job, self._directory / job.id)
                job.size = job.file.stat().st_size
        except JobCancelled:
            pass
        except DownloadFailed as failure:
            error = str(failure)
        except Exception:
            logger.exception("Download %s crashed", job.id)
            error = "Unexpected error"

        # Same lock as delete(): a job deleted mid-download is cleaned up exactly once.
        with self._lock:
            if job.id not in self._jobs:
                self._remove_files(job)
            elif error:
                job.status, job.error = JobStatus.FAILED, error
            else:
                job.status, job.percent = JobStatus.DONE, 100.0
            job.speed = job.eta = None

    def _remove_files(self, job: Job) -> None:
        shutil.rmtree(self._directory / job.id, ignore_errors=True)
