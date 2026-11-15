package jobs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/vmaerten/video-downloader/internal/ytdlp"
)

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// fakeDownloader blocks each download until the test opens its gate, like a
// download in flight, and writes a partial file meanwhile.
type fakeDownloader struct {
	mu       sync.Mutex
	gates    map[string]chan struct{}
	failures map[string]error
	started  []string
	name     string
}

func newFakeDownloader() *fakeDownloader {
	return &fakeDownloader{gates: map[string]chan struct{}{}, failures: map[string]error{}, name: "video.mp4"}
}

func (f *fakeDownloader) gate(url string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gates[url] == nil {
		f.gates[url] = make(chan struct{})
	}
	return f.gates[url]
}

func (f *fakeDownloader) startedURLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.started)
}

func (f *fakeDownloader) download(ctx context.Context, url, _, dir string, report func(ytdlp.Progress)) (string, error) {
	f.mu.Lock()
	f.started = append(f.started, url)
	failure := f.failures[url]
	f.mu.Unlock()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, f.name+".part"), []byte("..."), 0o644); err != nil {
		return "", err
	}
	half := 50.0
	report(ytdlp.Progress{Title: "Title of " + url, Percent: &half})

	select {
	case <-f.gate(url):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if failure != nil {
		return "", failure
	}
	file := filepath.Join(dir, f.name)
	return file, os.WriteFile(file, []byte("abc"), 0o644)
}

func newStore(t *testing.T) (*Store, *fakeDownloader, string, string) {
	t.Helper()
	work, dest := t.TempDir(), t.TempDir()
	fake := newFakeDownloader()
	store, err := New(work, dest, fake.download)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store, fake, work, dest
}

func status(s *Store, id string) Status {
	job, _ := s.Get(id)
	return job.Status
}

func TestThirdJobWaitsUntilASlotFrees(t *testing.T) {
	store, fake, _, dest := newStore(t)
	first := store.Create("a", "best")
	second := store.Create("b", "best")
	waitUntil(t, func() bool { return len(fake.startedURLs()) == 2 })
	third := store.Create("c", "best")

	time.Sleep(50 * time.Millisecond)
	if got := status(store, third.ID); got != Queued {
		t.Fatalf("third job = %s, want queued", got)
	}

	close(fake.gate("a"))

	waitUntil(t, func() bool { return status(store, third.ID) == Downloading })
	done, _ := store.Get(first.ID)
	if done.Status != Done || *done.Percent != 100 || done.Size != 3 || done.Title != "Title of a" {
		t.Errorf("first job = %+v", done)
	}
	if done.File != filepath.Join(dest, "video.mp4") {
		t.Errorf("file = %q, want it in the destination folder", done.File)
	}
	if got := status(store, second.ID); got != Downloading {
		t.Errorf("second job = %s, want downloading", got)
	}
}

func TestFailedDownloadKeepsAReadableError(t *testing.T) {
	tests := []struct {
		name    string
		failure error
		want    string
	}{
		{"error from yt-dlp", &ytdlp.Error{Code: "unavailable", Message: "This video is unavailable"}, "This video is unavailable"},
		{"anything else", errors.New("boom"), "Unexpected error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, fake, _, _ := newStore(t)
			fake.failures["a"] = tt.failure
			close(fake.gate("a"))

			job := store.Create("a", "best")

			waitUntil(t, func() bool { return status(store, job.ID) == Failed })
			if got, _ := store.Get(job.ID); got.Error != tt.want {
				t.Errorf("error = %q, want %q", got.Error, tt.want)
			}
		})
	}
}

func TestDeletingARunningJobStopsItAndRemovesItsFiles(t *testing.T) {
	store, _, work, _ := newStore(t)
	job := store.Create("a", "best")
	waitUntil(t, func() bool { _, err := os.Stat(filepath.Join(work, job.ID)); return err == nil })

	store.Delete(job.ID)

	waitUntil(t, func() bool { _, err := os.Stat(filepath.Join(work, job.ID)); return os.IsNotExist(err) })
	if _, ok := store.Get(job.ID); ok {
		t.Error("deleted job is still listed")
	}
}

func TestDeletingAQueuedJobNeverStartsIt(t *testing.T) {
	store, fake, _, _ := newStore(t)
	store.Create("a", "best")
	store.Create("b", "best")
	waitUntil(t, func() bool { return len(fake.startedURLs()) == 2 })
	queued := store.Create("c", "best")

	store.Delete(queued.ID)
	close(fake.gate("a"))
	close(fake.gate("b"))

	waitUntil(t, func() bool {
		jobs := store.List()
		return len(jobs) == 2 && jobs[0].Status == Done && jobs[1].Status == Done
	})
	got := fake.startedURLs()
	slices.Sort(got)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("started = %v, want a and b only", got)
	}
}

func TestDeletingAFinishedJobKeepsItsFile(t *testing.T) {
	store, fake, _, _ := newStore(t)
	close(fake.gate("a"))
	job := store.Create("a", "best")
	waitUntil(t, func() bool { return status(store, job.ID) == Done })
	done, _ := store.Get(job.ID)

	store.Delete(job.ID)

	if _, err := os.Stat(done.File); err != nil {
		t.Errorf("file of a forgotten job: %v", err)
	}
}

func TestFinishedFilesNeverOverwriteEachOther(t *testing.T) {
	store, fake, _, dest := newStore(t)
	if err := os.WriteFile(filepath.Join(dest, "video.mp4"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	close(fake.gate("a"))
	close(fake.gate("b"))

	a := store.Create("a", "best")
	waitUntil(t, func() bool { return status(store, a.ID) == Done })
	b := store.Create("b", "best")
	waitUntil(t, func() bool { return status(store, b.ID) == Done })

	gotA, _ := store.Get(a.ID)
	gotB, _ := store.Get(b.ID)
	if filepath.Base(gotA.File) != "video (1).mp4" || filepath.Base(gotB.File) != "video (2).mp4" {
		t.Errorf("files = %q, %q", gotA.File, gotB.File)
	}
	if mine, _ := os.ReadFile(filepath.Join(dest, "video.mp4")); string(mine) != "mine" {
		t.Errorf("existing file was overwritten: %q", mine)
	}
}

func TestNewWipesWhatAPreviousRunLeft(t *testing.T) {
	work := t.TempDir()
	leftover := filepath.Join(work, "abc", "video.mp4.part")
	if err := os.MkdirAll(filepath.Dir(leftover), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leftover, []byte("..."), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := New(work, t.TempDir(), newFakeDownloader().download); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Errorf("leftover still there: %v", err)
	}
}
