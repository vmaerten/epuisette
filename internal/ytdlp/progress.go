package ytdlp

import (
	"encoding/json"
	"strings"
)

// Progress is what the page shows for a running download.
type Progress struct {
	Title     string
	Extractor string
	Percent   *float64
	Speed     *float64
	ETA       *float64
	Merging   bool
}

// The download command prints these prefixes, each followed by JSON.
const (
	lineMeta     = "VD_META "
	lineSizes    = "VD_SIZES "
	lineDownload = "VD_DL "
	linePost     = "VD_PP "
	lineFile     = "VD_FILE "
)

type downloadLine struct {
	Status          string   `json:"status"`
	DownloadedBytes float64  `json:"downloaded_bytes"`
	TotalBytes      float64  `json:"total_bytes"`
	TotalEstimate   float64  `json:"total_bytes_estimate"`
	Speed           *float64 `json:"speed"`
}

type postLine struct {
	Status        string `json:"status"`
	Postprocessor string `json:"postprocessor"`
}

// tracker turns the lines of one download into Progress updates.
type tracker struct {
	report   func(Progress)
	progress Progress
	// Video and audio are separate files: with both sizes known, the percent
	// covers the two so the bar does not run to 100% twice.
	expected      float64
	finishedBytes float64
	file          string
	lastError     string
}

func (t *tracker) line(line string) {
	switch {
	case strings.HasPrefix(line, lineMeta):
		var meta struct {
			Title        string `json:"title"`
			ExtractorKey string `json:"extractor_key"`
		}
		if json.Unmarshal([]byte(line[len(lineMeta):]), &meta) == nil {
			t.progress.Title, t.progress.Extractor = meta.Title, meta.ExtractorKey
			t.report(t.progress)
		}
	case strings.HasPrefix(line, lineSizes):
		var sizes []struct {
			Filesize       float64 `json:"filesize"`
			FilesizeApprox float64 `json:"filesize_approx"`
		}
		if json.Unmarshal([]byte(line[len(lineSizes):]), &sizes) == nil {
			t.expected = 0
			for _, s := range sizes {
				size := max(s.Filesize, s.FilesizeApprox)
				if size == 0 {
					t.expected = 0
					break
				}
				t.expected += size
			}
		}
	case strings.HasPrefix(line, lineDownload):
		var d downloadLine
		if json.Unmarshal([]byte(line[len(lineDownload):]), &d) == nil {
			t.download(d)
		}
	case strings.HasPrefix(line, linePost):
		var p postLine
		// MoveFiles runs after every download, even when there is nothing to merge.
		if json.Unmarshal([]byte(line[len(linePost):]), &p) == nil && p.Status == "started" && p.Postprocessor != "MoveFiles" {
			t.progress.Merging = true
			t.progress.Speed, t.progress.ETA = nil, nil
			t.report(t.progress)
		}
	case strings.HasPrefix(line, lineFile):
		_ = json.Unmarshal([]byte(line[len(lineFile):]), &t.file)
	case strings.HasPrefix(line, "ERROR: "):
		t.lastError = line
	}
}

func (t *tracker) download(d downloadLine) {
	if d.Status == "finished" {
		t.finishedBytes += max(d.TotalBytes, d.DownloadedBytes)
		return
	}
	if d.Status != "downloading" {
		return
	}

	done, total := d.DownloadedBytes, d.TotalBytes
	if total == 0 {
		total = d.TotalEstimate
	}
	if t.expected > 0 {
		done, total = t.finishedBytes+done, t.expected
	}

	t.progress.Percent, t.progress.ETA = nil, nil
	if total > 0 {
		percent := min(100, done/total*100)
		t.progress.Percent = &percent
		if d.Speed != nil && *d.Speed > 0 {
			eta := max(0, (total-done) / *d.Speed)
			t.progress.ETA = &eta
		}
	}
	t.progress.Speed = d.Speed
	t.report(t.progress)
}
