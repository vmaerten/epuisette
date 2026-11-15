package ytdlp

import (
	"regexp"
	"strings"
)

// Error reaches the page as `error.cause`, which picks its text by Code.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

var (
	errNotWebURL   = &Error{Code: "unsupported_url", Message: "Not a web address"}
	errPlaylist    = &Error{Code: "playlist", Message: "Playlists are not supported"}
	errToolMissing = &Error{Code: "tool_missing", Message: "yt-dlp is not installed"}
)

// "ERROR: [youtube] abc123: This video is unavailable" keeps only the last part.
var extractorPrefix = regexp.MustCompile(`^\[[^\]]+\] [^:]+: `)

// errorFromStderr maps the last "ERROR:" line of yt-dlp to an Error.
func errorFromStderr(stderr string) *Error {
	var line string
	for l := range strings.Lines(stderr) {
		if after, ok := strings.CutPrefix(strings.TrimSpace(l), "ERROR: "); ok {
			line = after
		}
	}
	if line == "" {
		return &Error{Code: "extraction_failed", Message: "yt-dlp failed without an error message"}
	}

	message := extractorPrefix.ReplaceAllString(line, "")
	switch {
	case strings.HasPrefix(line, "Unsupported URL:"):
		return &Error{Code: "unsupported_url", Message: "No video found at this address"}
	// yt-dlp asks for a bug report when it does not expect the failure, and a
	// network failure says nothing about the video itself.
	case strings.Contains(line, "please report this issue"),
		strings.Contains(line, "Unable to download"):
		return &Error{Code: "extraction_failed", Message: message}
	default:
		return &Error{Code: "unavailable", Message: message}
	}
}
