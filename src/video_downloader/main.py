import asyncio
import logging
import os
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Literal

from fastapi import FastAPI, Request, Response
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, ConfigDict

from video_downloader.downloader import download
from video_downloader.errors import ApiError
from video_downloader.info import VideoInfo, fetch_info, require_web_url
from video_downloader.jobs import Job, JobStatus, JobStore

logger = logging.getLogger(__name__)

STATIC_DIR = Path(__file__).parent / "static"
DOWNLOADS_DIR = Path(os.environ.get("DOWNLOADS_DIR", "downloads"))
MAX_AGE_SECONDS = float(os.environ.get("DOWNLOADS_MAX_AGE_DAYS", "7")) * 86400
PURGE_INTERVAL_SECONDS = 3600

jobs = JobStore(DOWNLOADS_DIR, download)


async def _purge_periodically() -> None:
    while True:
        try:
            await asyncio.to_thread(jobs.purge, MAX_AGE_SECONDS)
        except Exception:
            # One failed pass must not end the loop for the life of the server.
            logger.exception("Purging old downloads failed")
        await asyncio.sleep(PURGE_INTERVAL_SECONDS)


@asynccontextmanager
async def lifespan(_: FastAPI):
    purge = asyncio.create_task(_purge_periodically())
    yield
    purge.cancel()
    # Otherwise running downloads keep the process alive after Ctrl+C.
    jobs.close()


app = FastAPI(title="video-downloader", lifespan=lifespan)


class InfoRequest(BaseModel):
    url: str


class DownloadRequest(BaseModel):
    url: str
    preset: Literal["best", "1080p", "720p", "audio"]


class JobView(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: str
    url: str
    preset: str
    status: JobStatus
    title: str | None
    extractor: str | None
    percent: float | None
    speed: float | None
    eta: float | None
    size: int | None
    error: str | None


@app.exception_handler(ApiError)
def handle_api_error(_: Request, error: ApiError) -> JSONResponse:
    return JSONResponse(
        status_code=error.status,
        content={"code": error.code, "message": error.message},
    )


def _find_job(job_id: str) -> Job:
    job = jobs.get(job_id)
    if job is None:
        raise ApiError(404, "not_found", "Unknown download")
    return job


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


# Sync on purpose: yt-dlp blocks, and FastAPI runs sync endpoints in a thread pool.
@app.post("/api/info")
def info(request: InfoRequest) -> VideoInfo:
    return fetch_info(request.url)


@app.post("/api/downloads", status_code=202)
def create_download(request: DownloadRequest) -> JobView:
    require_web_url(request.url)
    return JobView.model_validate(jobs.create(request.url, request.preset))


@app.get("/api/downloads")
def list_downloads() -> list[JobView]:
    return [JobView.model_validate(job) for job in jobs.list()]


@app.get("/api/downloads/{job_id}")
def get_download(job_id: str) -> JobView:
    return JobView.model_validate(_find_job(job_id))


@app.get("/api/downloads/{job_id}/file")
def get_download_file(job_id: str) -> FileResponse:
    job = _find_job(job_id)
    if job.status != JobStatus.DONE:
        raise ApiError(409, "not_ready", "This download has no file yet")
    # The path is the one the downloader reported, never built from the request.
    return FileResponse(job.file, filename=job.file.name)


@app.delete("/api/downloads/{job_id}", status_code=204)
def delete_download(job_id: str) -> Response:
    if not jobs.delete(job_id):
        raise ApiError(404, "not_found", "Unknown download")
    return Response(status_code=204)


# Mounted last: a mount on "/" swallows every route declared after it.
app.mount("/", StaticFiles(directory=STATIC_DIR, html=True), name="static")
