from pathlib import Path

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

from video_downloader.info import InfoError, VideoInfo, fetch_info

STATIC_DIR = Path(__file__).parent / "static"

app = FastAPI(title="video-downloader")


class InfoRequest(BaseModel):
    url: str


@app.exception_handler(InfoError)
def handle_info_error(_: Request, error: InfoError) -> JSONResponse:
    return JSONResponse(
        status_code=error.status,
        content={"code": error.code, "message": error.message},
    )


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


# Sync on purpose: yt-dlp blocks, and FastAPI runs sync endpoints in a thread pool.
@app.post("/api/info")
def info(request: InfoRequest) -> VideoInfo:
    return fetch_info(request.url)


# Mounted last: a mount on "/" swallows every route declared after it.
app.mount("/", StaticFiles(directory=STATIC_DIR, html=True), name="static")
