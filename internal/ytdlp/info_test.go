package ytdlp

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// Fixtures are real `yt-dlp -J` outputs, trimmed with jq to the fields read here.
func TestParseInfoBuildsPresetsFromRecordedOutputs(t *testing.T) {
	tests := []struct {
		fixture   string
		extractor string
		presets   []string
	}{
		{"youtube-4k", "Youtube", []string{"best:2160", "1440p:1440", "1080p:1080", "720p:720", "480p:480", "360p:360", "audio"}},
		// The best quality is 1080p, so there is no separate 1080p preset.
		{"youtube-1080p", "Youtube", []string{"best:1080", "720p:720", "480p:480", "360p:360", "audio"}},
		// X serves HLS variants with unknown codecs next to a muxed MP4.
		{"x-post", "Twitter", []string{"best:720", "360p:360", "audio"}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			info, err := parseInfo(readFixture(t, tt.fixture+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if info.Extractor != tt.extractor || info.Title == "" || info.Duration == nil {
				t.Errorf("metadata = %q, %q, %v", info.Extractor, info.Title, info.Duration)
			}
			if got := presetLabels(info.Presets); !reflect.DeepEqual(got, tt.presets) {
				t.Errorf("presets = %v, want %v", got, tt.presets)
			}
		})
	}
}

func TestBuildPresets(t *testing.T) {
	none, h264, unknown := ptr("none"), ptr("avc1"), (*string)(nil)
	tests := []struct {
		name    string
		formats []Format
		want    []string
	}{
		{
			name: "vertical video uses the smaller dimension",
			formats: []Format{
				{Vcodec: h264, Acodec: h264, Width: ptr(720), Height: ptr(1280)},
				{Vcodec: h264, Acodec: h264, Width: ptr(1080), Height: ptr(1920)},
			},
			want: []string{"best:1080", "720p:720", "audio"},
		},
		{
			name: "a step falls back to the nearest lower resolution",
			formats: []Format{
				{Vcodec: h264, Acodec: none, Width: ptr(1920), Height: ptr(1080)},
				{Vcodec: h264, Acodec: none, Width: ptr(1024), Height: ptr(576)},
				{Vcodec: none, Acodec: h264},
			},
			want: []string{"best:1080", "576p:576", "audio"},
		},
		{
			name: "storyboards and audio only give the audio preset alone",
			formats: []Format{
				{Vcodec: none, Acodec: none, Width: ptr(320), Height: ptr(180)},
				{Vcodec: none, Acodec: h264},
			},
			want: []string{"audio"},
		},
		{
			name:    "an unknown codec counts as video",
			formats: []Format{{Vcodec: unknown, Acodec: unknown}},
			want:    []string{"best", "audio"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := presetLabels(BuildPresets(tt.formats)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("presets = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseInfoRejectsPlaylists(t *testing.T) {
	if _, err := parseInfo(readFixture(t, "playlist.json")); err != errPlaylist {
		t.Errorf("err = %v, want the playlist error", err)
	}
}

func TestInfoRejectsWhatIsNotALink(t *testing.T) {
	if _, err := Info(t.Context(), "ytsearch:big buck bunny"); err != errNotWebURL {
		t.Errorf("err = %v, want the not-a-link error", err)
	}
}

func TestInfoReportsAMissingYtdlp(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := Info(t.Context(), "https://example.com/watch"); err != errToolMissing {
		t.Errorf("err = %v, want the tool-missing error", err)
	}
}

// Fixtures are the real stderr of yt-dlp for each case.
func TestErrorFromStderr(t *testing.T) {
	tests := []struct {
		fixture, code, message string
	}{
		{"unsupported", "unsupported_url", "No video found at this address"},
		{"unavailable", "unavailable", "This video is unavailable"},
		{"no-video", "unavailable", "No video could be found in this tweet"},
		{"network", "extraction_failed", "Unable to download webpage: HTTP Error 500: Domain Not Found (caused by <HTTPError 500: Domain Not Found>)"},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			got := errorFromStderr(string(readFixture(t, tt.fixture+".stderr")))
			if got.Code != tt.code || got.Message != tt.message {
				t.Errorf("error = %+v, want %s %q", got, tt.code, tt.message)
			}
		})
	}
}

// presetLabels writes each preset as "id:resolution", or "id" without a resolution.
func presetLabels(presets []Preset) []string {
	var labels []string
	for _, p := range presets {
		label := p.ID
		if p.Resolution != nil {
			label += ":" + strconv.Itoa(*p.Resolution)
		}
		labels = append(labels, label)
	}
	return labels
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func ptr[T any](v T) *T { return &v }
