import pytest
from fastapi.testclient import TestClient
from yt_dlp.utils import DownloadError, ExtractorError, UnsupportedError

from video_downloader import info
from video_downloader.main import app

client = TestClient(app)

# Format lists recorded from real extractions, trimmed to the fields the presets read.
STORYBOARD = {"vcodec": "none", "acodec": "none", "width": 320, "height": 180}
AUDIO_ONLY = {"vcodec": "none", "acodec": "opus", "width": None, "height": None}

YOUTUBE_4K = [
    STORYBOARD,
    AUDIO_ONLY,
    {"vcodec": "avc1.42001E", "acodec": "mp4a.40.2", "width": 640, "height": 360},
    {"vcodec": "vp9", "acodec": "none", "width": 1280, "height": 720},
    {"vcodec": "vp9", "acodec": "none", "width": 1920, "height": 1080},
    {"vcodec": "vp9", "acodec": "none", "width": 2560, "height": 1440},
    {"vcodec": "vp9", "acodec": "none", "width": 3840, "height": 2160},
]

# X serves HLS variants with unknown codecs next to a muxed MP4.
X_720P = [
    {"vcodec": None, "acodec": None, "width": 320, "height": 180},
    {"vcodec": None, "acodec": None, "width": 640, "height": 360},
    {"vcodec": None, "acodec": None, "width": 1280, "height": 720},
    {"vcodec": "avc1.4d0020", "acodec": "mp4a.40.2", "width": 1280, "height": 720},
]

VERTICAL = [
    {"vcodec": "avc1", "acodec": "mp4a", "width": 720, "height": 1280},
    {"vcodec": "avc1", "acodec": "mp4a", "width": 1080, "height": 1920},
]


@pytest.mark.parametrize(
    ("formats", "expected"),
    [
        (
            YOUTUBE_4K,
            [
                ("best", 2160),
                ("1440p", 1440),
                ("1080p", 1080),
                ("720p", 720),
                ("360p", 360),
                ("audio", None),
            ],
        ),
        (X_720P, [("best", 720), ("360p", 360), ("audio", None)]),
        (VERTICAL, [("best", 1080), ("720p", 720), ("audio", None)]),
        ([STORYBOARD, AUDIO_ONLY], [("audio", None)]),
    ],
    ids=["named after the result", "no duplicate of the best", "vertical video", "audio only"],
)
def test_build_presets(formats, expected):
    presets = info.build_presets(formats)

    assert [(p.id, p.resolution) for p in presets] == expected


def fake_ydl(monkeypatch, result):
    class FakeYoutubeDL:
        def __init__(self, params):
            pass

        def __enter__(self):
            return self

        def __exit__(self, *exc):
            return False

        def extract_info(self, url, download):
            if isinstance(result, Exception):
                raise result
            return result

    monkeypatch.setattr(info.yt_dlp, "YoutubeDL", FakeYoutubeDL)


def download_error(cause):
    # Same shape as what YoutubeDL raises: the cause travels in exc_info.
    return DownloadError(f"ERROR: {cause}", (type(cause), cause, None))


def test_info_returns_metadata_and_presets(monkeypatch):
    fake_ydl(
        monkeypatch,
        {
            "title": "Big Buck Bunny",
            "thumbnail": "https://example.com/thumb.webp",
            "duration": 635,
            "uploader": "Blender",
            "extractor_key": "Youtube",
            "formats": X_720P,
        },
    )

    response = client.post("/api/info", json={"url": "https://example.com/watch"})

    assert response.status_code == 200
    assert response.json() == {
        "title": "Big Buck Bunny",
        "thumbnail": "https://example.com/thumb.webp",
        "duration": 635,
        "uploader": "Blender",
        "extractor": "Youtube",
        "presets": [
            {"id": "best", "resolution": 720},
            {"id": "360p", "resolution": 360},
            {"id": "audio", "resolution": None},
        ],
    }


@pytest.mark.parametrize(
    ("result", "status", "code", "message"),
    [
        (
            download_error(UnsupportedError("https://example.com/")),
            422,
            "unsupported_url",
            "No video found at this address",
        ),
        (
            download_error(ExtractorError("This video is unavailable", expected=True)),
            404,
            "unavailable",
            "This video is unavailable",
        ),
        (
            download_error(OSError("HTTP Error 500")),
            502,
            "extraction_failed",
            "HTTP Error 500",
        ),
        ({"_type": "playlist", "entries": []}, 422, "playlist", "Playlists are not supported"),
    ],
    ids=["unsupported url", "unavailable video", "broken extraction", "playlist"],
)
def test_info_maps_failures_to_readable_errors(monkeypatch, result, status, code, message):
    fake_ydl(monkeypatch, result)

    response = client.post("/api/info", json={"url": "https://example.com/watch"})

    assert response.status_code == status
    assert response.json() == {"code": code, "message": message}


def test_info_rejects_what_is_not_a_link(monkeypatch):
    fake_ydl(monkeypatch, AssertionError("yt-dlp must not be called"))

    response = client.post("/api/info", json={"url": "ytsearch:big buck bunny"})

    assert response.status_code == 422
    assert response.json()["code"] == "unsupported_url"
