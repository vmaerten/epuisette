package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Service holds the methods the page calls through the generated bindings.
type Service struct{}

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
