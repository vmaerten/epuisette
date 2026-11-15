import os
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Literal

from fastapi import FastAPI, Request, Response
from fastapi.responses import JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, ConfigDict

from video_downloader.downloader import download
from video_downloader.errors import ApiError
from video_downloader.info import VideoInfo, fetch_info, require_web_url
from video_downloader.jobs import Job, JobStatus, JobStore

STATIC_DIR = Path(__file__).parent / "static"
DOWNLOADS_DIR = Path(os.environ.get("DOWNLOADS_DIR", "downloads"))

jobs = JobStore(DOWNLOADS_DIR, download)


@asynccontextmanager
async def lifespan(_: FastAPI):
    yield
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


@app.delete("/api/downloads/{job_id}", status_code=204)
def delete_download(job_id: str) -> Response:
    if not jobs.delete(job_id):
        raise ApiError(404, "not_found", "Unknown download")
    return Response(status_code=204)


# Mounted last: a mount on "/" swallows every route declared after it.
app.mount("/", StaticFiles(directory=STATIC_DIR, html=True), name="static")
