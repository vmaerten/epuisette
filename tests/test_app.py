from fastapi.testclient import TestClient

from video_downloader.main import app

client = TestClient(app)


def test_healthz_is_not_shadowed_by_the_static_mount():
    response = client.get("/healthz")

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_root_serves_the_index_page():
    response = client.get("/")

    assert response.status_code == 200
    assert "<title>video-downloader</title>" in response.text
