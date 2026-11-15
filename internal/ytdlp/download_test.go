package ytdlp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// replay feeds a recorded download, in the order the shared pipe gives, to a tracker.
func replay(t *testing.T, fixture string) (*tracker, []Progress) {
	t.Helper()
	var updates []Progress
	tr := &tracker{report: func(p Progress) { updates = append(updates, p) }}
	for line := range strings.Lines(string(readFixture(t, fixture))) {
		tr.line(strings.TrimRight(line, "\n"))
	}
	return tr, updates
}

func percents(updates []Progress) []int {
	var out []int
	for _, u := range updates {
		if u.Percent != nil && !u.Merging {
			out = append(out, int(*u.Percent))
		}
	}
	return out
}

func TestTrackerCountsVideoAndAudioAsOneDownload(t *testing.T) {
	tr, updates := replay(t, "download-split.txt")

	// 28 MB of video then 31 MB of audio: the second file starts near 48%, not at 0.
	if got, want := percents(updates), []int{0, 24, 47}; !slices.Equal(got, want) {
		t.Errorf("percents = %v, want %v", got, want)
	}
	last := updates[len(updates)-1]
	if !last.Merging || last.Speed != nil || last.ETA != nil {
		t.Errorf("last update = %+v, want merging without speed", last)
	}
	if last.Title != "Big Buck Bunny 60fps 4K - Official Blender Foundation Short Film" || last.Extractor != "Youtube" {
		t.Errorf("metadata = %q, %q", last.Title, last.Extractor)
	}
	if tr.file != "/jobs/abc/Big Buck Bunny 60fps 4K - Official Blender Foundation Short Film.mp4" {
		t.Errorf("file = %q", tr.file)
	}
}

func TestTrackerFallsBackToTheEstimateOfTheCurrentFile(t *testing.T) {
	tr, updates := replay(t, "download-hls.txt")

	if got, want := percents(updates), []int{0}; !slices.Equal(got, want) {
		t.Errorf("percents = %v, want %v", got, want)
	}
	if !strings.HasSuffix(tr.file, "theat....mp4") {
		t.Errorf("file = %q", tr.file)
	}
}

func TestTrackerTreatsAudioExtractionAsMerging(t *testing.T) {
	tr, updates := replay(t, "download-audio.txt")

	if !updates[len(updates)-1].Merging || tr.file != "/jobs/abc/Me at the zoo.m4a" {
		t.Errorf("last update = %+v, file = %q", updates[len(updates)-1], tr.file)
	}
}

func TestFormatArgsSortByResolutionThenCodec(t *testing.T) {
	tests := map[string]string{
		"best":  "res,vcodec:h264,acodec:aac",
		"1080p": "res:1080,vcodec:h264,acodec:aac",
		"576p":  "res:576,vcodec:h264,acodec:aac",
	}
	for preset, sort := range tests {
		args := formatArgs(preset)
		if i := slices.Index(args, "-S"); i < 0 || args[i+1] != sort {
			t.Errorf("%s: args = %v, want -S %s", preset, args, sort)
		}
	}
	if args := formatArgs("audio"); !slices.Contains(args, "-x") || slices.Contains(args, "-S") {
		t.Errorf("audio: args = %v, want an audio extraction", args)
	}
}

func TestDownloadRejectsAnUnknownPreset(t *testing.T) {
	_, err := Download(t.Context(), "https://example.com/v", "hd", t.TempDir(), func(Progress) {})
	var e *Error
	if !errors.As(err, &e) || e.Code != "invalid_preset" {
		t.Errorf("err = %v, want invalid_preset", err)
	}
}

func TestCheckFileRefusesAFileOutsideItsDirectory(t *testing.T) {
	for _, file := range []string{"/jobs/other/x.mp4", "/jobs/abc", ""} {
		if _, err := checkFile(file, "/jobs/abc"); err == nil {
			t.Errorf("%q: no error", file)
		}
	}
	if _, err := checkFile("/jobs/abc/x.mp4", "/jobs/abc"); err != nil {
		t.Error(err)
	}
}

// Runs the real yt-dlp against YouTube: VD_LIVE=1 go test ./internal/ytdlp -run Live -v
func TestLiveDownloadAndCancel(t *testing.T) {
	if os.Getenv("VD_LIVE") == "" {
		t.Skip("set VD_LIVE=1 to download from YouTube")
	}

	t.Run("audio of a short clip", func(t *testing.T) {
		dir := t.TempDir()
		file, err := Download(t.Context(), "https://www.youtube.com/watch?v=jNQXAC9IVRw", "audio", dir, func(Progress) {})
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Ext(file) != ".m4a" {
			t.Errorf("file = %q", file)
		}
	})

	t.Run("cancel while ffmpeg merges", func(t *testing.T) {
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		_, err := Download(ctx, "https://www.youtube.com/watch?v=aqz-KE-bpKQ", "1080p", dir, func(p Progress) {
			if p.Merging {
				cancel()
			}
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		// Every process of the download names the job directory in its arguments.
		if out, _ := exec.Command("pgrep", "-fl", dir).Output(); len(out) > 0 {
			t.Errorf("processes left after cancel:\n%s", out)
		}
	})
}
