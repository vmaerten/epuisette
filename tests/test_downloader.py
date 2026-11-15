import pytest
from yt_dlp.utils import DownloadCancelled

from video_downloader.downloader import ProgressTracker
from video_downloader.jobs import Job, JobStatus


def new_job():
    return Job(id="1", url="https://example.com/v", preset="best", status=JobStatus.DOWNLOADING)


def downloading(info, downloaded, total, speed=100):
    return {
        "status": "downloading",
        "info_dict": info,
        "downloaded_bytes": downloaded,
        "total_bytes": total,
        "speed": speed,
    }


def test_progress_counts_video_and_audio_as_one_download():
    job = new_job()
    tracker = ProgressTracker(job)
    info = {
        "title": "Big Buck Bunny",
        "extractor_key": "Youtube",
        "requested_formats": [{"filesize": 900}, {"filesize_approx": 100}],
    }

    tracker.on_download(downloading(info, downloaded=450, total=900))
    assert (job.percent, job.eta) == (45.0, 5.5)

    tracker.on_download({"status": "finished", "info_dict": info, "total_bytes": 900})
    tracker.on_download(downloading(info, downloaded=50, total=100))

    assert (job.percent, job.eta, job.speed) == (95.0, 0.5, 100)
    assert (job.title, job.extractor) == ("Big Buck Bunny", "Youtube")


def test_progress_falls_back_to_the_current_file_when_sizes_are_unknown():
    job = new_job()

    ProgressTracker(job).on_download(downloading({"title": "Clip"}, downloaded=30, total=120))

    assert job.percent == 25.0


def test_postprocessing_marks_the_job_as_merging_except_for_the_final_move():
    job = new_job()
    tracker = ProgressTracker(job)

    tracker.on_postprocess({"status": "started", "postprocessor": "MoveFiles"})
    assert job.status == JobStatus.DOWNLOADING

    tracker.on_postprocess({"status": "started", "postprocessor": "Merger"})
    assert job.status == JobStatus.MERGING


@pytest.mark.parametrize("hook", ["on_download", "on_postprocess"])
def test_cancelled_job_aborts_yt_dlp_from_its_hooks(hook):
    job = new_job()
    job.cancelled.set()

    with pytest.raises(DownloadCancelled):
        getattr(ProgressTracker(job), hook)({"status": "started", "postprocessor": "Merger"})
