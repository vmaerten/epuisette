package ytdlp

import (
	"context"
	"os/exec"
	"strings"
)

// The programs a download needs: yt-dlp itself, ffmpeg to merge and convert,
// and deno to read YouTube's player.
var requiredTools = []string{"yt-dlp", "ffmpeg", "deno"}

// MissingTools lists the required programs that are not on the PATH.
func MissingTools() []string {
	missing := []string{}
	for _, tool := range requiredTools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	return missing
}

func Version(ctx context.Context) (string, error) {
	out, err := run(ctx, "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
