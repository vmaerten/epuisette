package ytdlp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var presetID = regexp.MustCompile(`^(best|audio|\d{3,4}p)$`)

// ValidatePreset accepts the presets BuildPresets can return.
func ValidatePreset(preset string) error {
	if !presetID.MatchString(preset) {
		return &Error{Code: "invalid_preset", Message: "Unknown format"}
	}
	return nil
}

func formatArgs(preset string) []string {
	if preset == "audio" {
		return []string{"-f", "ba[acodec^=aac]/ba[acodec^=mp4a.40.]/ba/b", "-x", "--audio-format", "m4a"}
	}
	resolution := "res"
	if preset != "best" {
		resolution = "res:" + strings.TrimSuffix(preset, "p")
	}
	return []string{
		"-f", "bv*+ba/b",
		// Resolution first, then the codecs every player reads. On YouTube H.264
		// stops at 1080p, so "best" on a 4K video is VP9 or AV1 inside the MP4.
		"-S", resolution + ",vcodec:h264,acodec:aac",
		"--merge-output-format", "mp4", "--remux-video", "mp4",
	}
}

// Download saves url into dir, calls report as it goes, and returns the final file.
// Cancelling ctx stops yt-dlp and the ffmpeg it may be running.
func Download(ctx context.Context, url, preset, dir string, report func(Progress)) (string, error) {
	if err := RequireWebURL(url); err != nil {
		return "", err
	}
	if err := ValidatePreset(preset); err != nil {
		return "", err
	}

	args := []string{
		"--ignore-config", "--no-warnings", "--socket-timeout", "15", "--no-playlist",
		// --print implies quiet, which hides progress unless --progress asks for it.
		"--newline", "--progress", "--progress-delta", "0.5",
		"--progress-template", "download:" + lineDownload + "%(progress.{status,downloaded_bytes,total_bytes,total_bytes_estimate,speed})j",
		"--progress-template", "postprocess:" + linePost + "%(progress.{status,postprocessor})j",
		"--print", "before_dl:" + lineMeta + "%(.{title,extractor_key})j",
		"--print", "before_dl:" + lineSizes + "%(requested_formats.:.{filesize,filesize_approx}|null)j",
		"--print", "after_move:" + lineFile + "%(filepath)j",
		"-P", dir, "-o", "%(title).200B.%(ext)s",
	}
	args = append(args, formatArgs(preset)...)
	args = append(args, "--", url)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	// yt-dlp runs ffmpeg as a child that it does not stop when it is itself
	// stopped: its own process group lets a cancel reach both.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second

	// One pipe for stdout and stderr: progress goes to the first, post-processing
	// and errors to the second, and their order matters.
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer r.Close()
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		w.Close()
		if errors.Is(err, exec.ErrNotFound) {
			return "", errToolMissing
		}
		return "", fmt.Errorf("start yt-dlp: %w", err)
	}
	w.Close()

	t := &tracker{report: report}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		t.line(scanner.Text())
	}
	err = cmd.Wait()
	// Whatever is left of the group, such as an ffmpeg still merging.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)

	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", errorFromStderr(t.lastError)
	}
	return checkFile(t.file, dir)
}

// checkFile refuses a missing file, and one outside dir: its name comes from
// the video title.
func checkFile(file, dir string) (string, error) {
	if file == "" {
		return "", &Error{Code: "extraction_failed", Message: "yt-dlp finished without a file"}
	}
	rel, err := filepath.Rel(dir, file)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("download landed outside its directory: %s", file)
	}
	return file, nil
}
