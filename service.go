package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/vmaerten/video-downloader/internal/jobs"
	"github.com/vmaerten/video-downloader/internal/ytdlp"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Service holds the methods the page calls through the generated bindings.
type Service struct {
	jobs *jobs.Store
}

type Config struct {
	YtdlpVersion string `json:"ytdlpVersion"`
}

func (s *Service) Config(ctx context.Context) (Config, error) {
	out, err := exec.CommandContext(ctx, "yt-dlp", "--version").Output()
	if err != nil {
		return Config{}, fmt.Errorf("read the yt-dlp version: %w", err)
	}
	return Config{YtdlpVersion: strings.TrimSpace(string(out))}, nil
}

func (s *Service) Info(ctx context.Context, url string) (ytdlp.VideoInfo, error) {
	return ytdlp.Info(ctx, url)
}

func (s *Service) StartDownload(url, preset string) (jobs.Job, error) {
	if err := ytdlp.RequireWebURL(url); err != nil {
		return jobs.Job{}, err
	}
	if err := ytdlp.ValidatePreset(preset); err != nil {
		return jobs.Job{}, err
	}
	return s.jobs.Create(url, preset), nil
}

func (s *Service) ListDownloads() []jobs.Job {
	return s.jobs.List()
}

// DeleteDownload cancels a download, or forgets a finished one without touching its file.
func (s *Service) DeleteDownload(id string) {
	s.jobs.Delete(id)
}

// RevealDownload selects the downloaded file in the Finder.
func (s *Service) RevealDownload(id string) error {
	job, ok := s.jobs.Get(id)
	if !ok || job.File == "" {
		return errors.New("this download has no file")
	}
	return application.Get().Env.OpenFileManager(job.File, true)
}

// ServiceShutdown stops the downloads still running when the app quits.
func (s *Service) ServiceShutdown() error {
	s.jobs.Close()
	return nil
}
