from pathlib import Path

from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles

STATIC_DIR = Path(__file__).parent / "static"

app = FastAPI(title="video-downloader")


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


# Mounted last: a mount on "/" swallows every route declared after it.
app.mount("/", StaticFiles(directory=STATIC_DIR, html=True), name="static")
