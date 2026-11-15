// Package ytdlp runs the yt-dlp binary and turns its output into what the page shows.
package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// One preset per step of the usual ladder, each named after the resolution it really gives.
var resolutionCaps = []int{1440, 1080, 720, 480, 360}

type Preset struct {
	ID         string `json:"id"`
	Resolution *int   `json:"resolution"`
}

type VideoInfo struct {
	Title     string   `json:"title"`
	Thumbnail string   `json:"thumbnail"`
	Duration  *float64 `json:"duration"`
	Uploader  string   `json:"uploader"`
	Extractor string   `json:"extractor"`
	Presets   []Preset `json:"presets"`
}

// Format is the part of a yt-dlp format the presets need. yt-dlp writes "none"
// for a missing stream and null for an unknown codec.
type Format struct {
	Vcodec *string `json:"vcodec"`
	Acodec *string `json:"acodec"`
	Width  *int    `json:"width"`
	Height *int    `json:"height"`
}

type rawInfo struct {
	Type         string   `json:"_type"`
	Title        string   `json:"title"`
	Thumbnail    string   `json:"thumbnail"`
	Duration     *float64 `json:"duration"`
	Uploader     string   `json:"uploader"`
	ExtractorKey string   `json:"extractor_key"`
	Formats      []Format `json:"formats"`
}

// RequireWebURL rejects search prefixes and bare words, which yt-dlp also accepts.
func RequireWebURL(url string) error {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return errNotWebURL
	}
	return nil
}

func Info(ctx context.Context, url string) (VideoInfo, error) {
	if err := RequireWebURL(url); err != nil {
		return VideoInfo{}, err
	}
	out, err := run(ctx,
		"--ignore-config", "--no-warnings", "--socket-timeout", "15",
		"-J", "--no-playlist", "--flat-playlist", "--", url)
	if err != nil {
		return VideoInfo{}, err
	}
	return parseInfo(out)
}

func parseInfo(out []byte) (VideoInfo, error) {
	var raw rawInfo
	if err := json.Unmarshal(out, &raw); err != nil {
		return VideoInfo{}, fmt.Errorf("read the yt-dlp output: %w", err)
	}
	if raw.Type == "playlist" {
		return VideoInfo{}, errPlaylist
	}
	return VideoInfo{
		Title:     raw.Title,
		Thumbnail: raw.Thumbnail,
		Duration:  raw.Duration,
		Uploader:  raw.Uploader,
		Extractor: raw.ExtractorKey,
		Presets:   BuildPresets(raw.Formats),
	}, nil
}

func BuildPresets(formats []Format) []Preset {
	var resolutions []int
	hasVideo, hasAudio := false, false
	for _, f := range formats {
		if !isNone(f.Vcodec) {
			hasVideo = true
			if r := resolution(f); r > 0 {
				resolutions = append(resolutions, r)
			}
		}
		if !isNone(f.Acodec) {
			hasAudio = true
		}
	}

	var presets []Preset
	if hasVideo {
		best := maxAtMost(resolutions, 0)
		presets = append(presets, Preset{ID: "best", Resolution: optional(best)})
		offered := map[int]bool{best: true}
		for _, limit := range resolutionCaps {
			capped := maxAtMost(resolutions, limit)
			if capped > 0 && !offered[capped] {
				presets = append(presets, Preset{ID: fmt.Sprintf("%dp", capped), Resolution: optional(capped)})
				offered[capped] = true
			}
		}
	}
	if hasAudio {
		presets = append(presets, Preset{ID: "audio"})
	}
	return presets
}

func isNone(codec *string) bool { return codec != nil && *codec == "none" }

// resolution is the smaller dimension, like yt-dlp's "res" sort: a vertical
// 1080x1920 video is 1080p.
func resolution(f Format) int {
	r := 0
	for _, d := range []*int{f.Width, f.Height} {
		if d != nil && *d > 0 && (r == 0 || *d < r) {
			r = *d
		}
	}
	return r
}

// maxAtMost returns the highest value not above limit, or the highest of all when limit is 0.
func maxAtMost(values []int, limit int) int {
	m := 0
	for _, v := range values {
		if v > m && (limit == 0 || v <= limit) {
			m = v
		}
	}
	return m
}

func optional(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func run(ctx context.Context, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errToolMissing
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, errorFromStderr(stderr.String())
		}
		return nil, fmt.Errorf("run yt-dlp: %w", err)
	}
	return stdout.Bytes(), nil
}
