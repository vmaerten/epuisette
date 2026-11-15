package ytdlp

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestMissingToolsListsWhatIsNotOnThePath(t *testing.T) {
	bin := t.TempDir()
	// A stand-in for yt-dlp: LookPath only needs an executable file.
	if err := os.WriteFile(filepath.Join(bin, "yt-dlp"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	if got := MissingTools(); !slices.Equal(got, []string{"ffmpeg", "deno"}) {
		t.Errorf("missing = %v, want ffmpeg and deno", got)
	}
}
