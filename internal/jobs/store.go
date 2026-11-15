// Package jobs queues downloads, runs two at a time and moves each finished
// file to the destination folder.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vmaerten/video-downloader/internal/ytdlp"
)

const maxConcurrentDownloads = 2

type Status string

const (
	Queued      Status = "queued"
	Downloading Status = "downloading"
	Merging     Status = "merging"
	Done        Status = "done"
	Failed      Status = "failed"
)

type Job struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Preset    string   `json:"preset"`
	Status    Status   `json:"status"`
	Title     string   `json:"title"`
	Extractor string   `json:"extractor"`
	Percent   *float64 `json:"percent"`
	Speed     *float64 `json:"speed"`
	ETA       *float64 `json:"eta"`
	Size      int64    `json:"size"`
	Error     string   `json:"error"`
	File      string   `json:"-"`
}

// DownloadFunc downloads url into dir and returns the final file. It must
// return soon after ctx is cancelled.
type DownloadFunc func(ctx context.Context, url, preset, dir string, report func(ytdlp.Progress)) (string, error)

type entry struct {
	job    Job
	cancel context.CancelFunc
	done   chan struct{}
}

type Store struct {
	workDir  string
	destDir  string
	download DownloadFunc
	slots    chan struct{}

	mu      sync.Mutex
	entries map[string]*entry
	order   []string
}

// New wipes workDir, which only holds unfinished downloads left by a previous run.
func New(workDir, destDir string, download DownloadFunc) (*Store, error) {
	if err := os.RemoveAll(workDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		workDir:  workDir,
		destDir:  destDir,
		download: download,
		slots:    make(chan struct{}, maxConcurrentDownloads),
		entries:  map[string]*entry{},
	}, nil
}

func (s *Store) Create(url, preset string) Job {
	ctx, cancel := context.WithCancel(context.Background())
	job := Job{ID: newID(), URL: url, Preset: preset, Status: Queued}
	e := &entry{job: job, cancel: cancel, done: make(chan struct{})}
	s.mu.Lock()
	s.entries[job.ID] = e
	s.order = append(s.order, job.ID)
	s.mu.Unlock()

	go s.run(ctx, e)
	return job
}

// List returns the jobs in the order they were created.
func (s *Store) List() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]Job, 0, len(s.order))
	for _, id := range s.order {
		jobs = append(jobs, s.entries[id].job)
	}
	return jobs
}

func (s *Store) Get(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return Job{}, false
	}
	return e.job, true
}

// Delete cancels a queued or running job and forgets any job. The file of a
// finished job stays where it is.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	e, ok := s.entries[id]
	if ok {
		delete(s.entries, id)
		s.order = removeID(s.order, id)
	}
	s.mu.Unlock()
	if ok {
		e.cancel()
	}
}

// Close cancels every job and waits for them to stop, so no yt-dlp outlives the app.
func (s *Store) Close() {
	s.mu.Lock()
	entries := make([]*entry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}
	s.mu.Unlock()

	for _, e := range entries {
		e.cancel()
	}
	for _, e := range entries {
		<-e.done
	}
}

func (s *Store) run(ctx context.Context, e *entry) {
	defer close(e.done)
	defer e.cancel()

	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return
	}

	dir := filepath.Join(s.workDir, e.job.ID)
	defer os.RemoveAll(dir)

	s.update(e, func(j *Job) { j.Status = Downloading })
	file, err := s.download(ctx, e.job.URL, e.job.Preset, dir, func(p ytdlp.Progress) {
		s.update(e, func(j *Job) {
			j.Title, j.Extractor = p.Title, p.Extractor
			j.Percent, j.Speed, j.ETA = p.Percent, p.Speed, p.ETA
			if p.Merging {
				j.Status = Merging
			}
		})
	})
	if ctx.Err() != nil {
		return
	}
	if err == nil {
		file, err = moveToUniqueName(file, s.destDir)
	}

	s.update(e, func(j *Job) {
		j.Speed, j.ETA = nil, nil
		if err != nil {
			j.Status, j.Error = Failed, failureMessage(err)
			return
		}
		full := 100.0
		j.Status, j.Percent, j.File = Done, &full, file
		if info, statErr := os.Stat(file); statErr == nil {
			j.Size = info.Size()
		}
	})
}

func (s *Store) update(e *entry, change func(*Job)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(&e.job)
}

func failureMessage(err error) string {
	var ytdlpErr *ytdlp.Error
	if errors.As(err, &ytdlpErr) {
		return ytdlpErr.Message
	}
	log.Printf("download failed: %v", err)
	return "Unexpected error"
}

// moveToUniqueName moves file into dir, adding " (1)", " (2)"... to its name
// when dir already has a file by that name.
func moveToUniqueName(file, dir string) (string, error) {
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(filepath.Base(file), ext)
	for i := 0; i < 1000; i++ {
		name := base + ext
		if i > 0 {
			name = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		dest := filepath.Join(dir, name)
		// A hard link fails if dest exists, so two downloads cannot overwrite each other.
		err := os.Link(file, dest)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("move the file to %s: %w", dir, err)
		}
		return dest, os.Remove(file)
	}
	return "", fmt.Errorf("no free name for %s in %s", base+ext, dir)
}

func removeID(ids []string, id string) []string {
	for i, v := range ids {
		if v == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
