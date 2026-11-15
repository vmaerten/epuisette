import threading
import time

import pytest
from fastapi.testclient import TestClient

from video_downloader import main
from video_downloader.jobs import DownloadFailed, JobCancelled, JobStatus, JobStore


def wait_until(condition, timeout=2.0):
    deadline = time.monotonic() + timeout
    while not condition():
        assert time.monotonic() < deadline, "condition not met in time"
        time.sleep(0.005)


class FakeDownloader:
    """Blocks each job until the test opens its gate, like a download in flight."""

    def __init__(self):
        self.gates = {}
        self.failures = {}
        self.started = []

    def gate(self, url):
        return self.gates.setdefault(url, threading.Event())

    def __call__(self, job, directory):
        self.started.append(job.url)
        directory.mkdir(parents=True)
        (directory / "video.mp4.part").write_bytes(b"...")
        while not self.gate(job.url).wait(0.005):
            if job.cancelled.is_set():
                raise JobCancelled()
        if job.url in self.failures:
            raise self.failures[job.url]
        file = directory / "video.mp4"
        file.write_bytes(b"abc")
        return file


@pytest.fixture
def downloader():
    return FakeDownloader()


@pytest.fixture
def store(tmp_path, downloader):
    store = JobStore(tmp_path, downloader)
    yield store
    store.close()


def test_third_job_waits_until_a_slot_frees(store, downloader):
    first, second, third = (store.create(url, "best") for url in ("a", "b", "c"))

    wait_until(lambda: downloader.started == ["a", "b"])
    assert [job.status for job in store.list()] == [
        JobStatus.DOWNLOADING,
        JobStatus.DOWNLOADING,
        JobStatus.QUEUED,
    ]

    downloader.gate("a").set()

    wait_until(lambda: third.status == JobStatus.DOWNLOADING)
    assert (first.status, first.percent, first.size) == (JobStatus.DONE, 100.0, 3)
    assert second.status == JobStatus.DOWNLOADING


@pytest.mark.parametrize(
    ("failure", "message"),
    [
        (DownloadFailed("This video is unavailable"), "This video is unavailable"),
        (RuntimeError("boom"), "Unexpected error"),
    ],
    ids=["readable failure", "crash"],
)
def test_failed_download_keeps_a_readable_error(store, downloader, failure, message):
    downloader.failures["a"] = failure
    downloader.gate("a").set()

    job = store.create("a", "best")

    wait_until(lambda: job.status == JobStatus.FAILED)
    assert job.error == message


def test_deleting_a_running_job_stops_it_and_removes_its_files(store, downloader, tmp_path):
    job = store.create("a", "best")
    wait_until(lambda: (tmp_path / job.id).exists())

    assert store.delete(job.id)

    wait_until(lambda: not (tmp_path / job.id).exists())
    assert store.get(job.id) is None


def test_deleting_a_queued_job_never_starts_it(store, downloader):
    for url in ("a", "b"):
        store.create(url, "best")
    queued = store.create("c", "best")
    wait_until(lambda: downloader.started == ["a", "b"])

    assert store.delete(queued.id)
    for url in ("a", "b"):
        downloader.gate(url).set()

    wait_until(lambda: all(job.status == JobStatus.DONE for job in store.list()))
    assert downloader.started == ["a", "b"]


def test_deleting_a_finished_job_removes_its_file(store, downloader, tmp_path):
    downloader.gate("a").set()
    job = store.create("a", "best")
    wait_until(lambda: job.status == JobStatus.DONE)

    assert store.delete(job.id)

    assert not (tmp_path / job.id).exists()


def test_downloads_api_follows_a_job_from_creation_to_deletion(store, downloader, monkeypatch):
    monkeypatch.setattr(main, "jobs", store)
    client = TestClient(main.app)

    rejected = client.post("/api/downloads", json={"url": "ytsearch:cats", "preset": "best"})
    assert (rejected.status_code, rejected.json()["code"]) == (422, "unsupported_url")

    created = client.post("/api/downloads", json={"url": "https://example.com/v", "preset": "720p"})
    assert created.status_code == 202
    job_id = created.json()["id"]

    downloader.gate("https://example.com/v").set()
    wait_until(lambda: client.get(f"/api/downloads/{job_id}").json()["status"] == "done")
    assert client.get("/api/downloads").json() == [
        {
            "id": job_id,
            "url": "https://example.com/v",
            "preset": "720p",
            "status": "done",
            "title": None,
            "extractor": None,
            "percent": 100.0,
            "speed": None,
            "eta": None,
            "size": 3,
            "error": None,
        }
    ]

    assert client.delete(f"/api/downloads/{job_id}").status_code == 204
    missing = client.get(f"/api/downloads/{job_id}")
    assert (missing.status_code, missing.json()["code"]) == (404, "not_found")
    assert client.delete(f"/api/downloads/{job_id}").status_code == 404
