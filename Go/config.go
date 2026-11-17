package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// config is saved to ~/Library/Application Support/hey-gui/config.json.
type config struct {
	HeyPath     string  `json:"hey_path"`
	NewestFirst bool    `json:"newest_first"`
	UnreadOnly  bool    `json:"unread_only"`
	ListWidth   float32 `json:"list_width"`   // dp
	WindowW     float32 `json:"window_width"` // dp
	WindowH     float32 `json:"window_height"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "hey-gui", "config.json")
}

func loadConfig() *config {
	c := &config{NewestFirst: true, ListWidth: 400, WindowW: 1200, WindowH: 800}
	if data, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(data, c)
	}
	return c
}

func (c *config) save() {
	p := configPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	data, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(p, data, 0o644)
}
