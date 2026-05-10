package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const settingsPath = "./data/settings.json"

// Settings 用户配置；持久化到 ./data/settings.json
type Settings struct {
	TdxDir string `json:"tdxDir"` // 本地通达信安装目录，如 "D:\\new_tdx"
}

type settingsStore struct {
	mu     sync.RWMutex
	loaded bool
	data   Settings
}

var settings = &settingsStore{}

func (s *settingsStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return
	}
	s.loaded = true
	bs, err := os.ReadFile(settingsPath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(bs, &s.data)
}

func (s *settingsStore) save() error {
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}
	bs, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, bs, 0644)
}

// GetSettings 取当前设置（首次会从磁盘读）
func (a *App) GetSettings() Settings {
	settings.load()
	settings.mu.RLock()
	defer settings.mu.RUnlock()
	return settings.data
}

// SetTdxDir 修改通达信目录并落盘
func (a *App) SetTdxDir(dir string) (Settings, error) {
	settings.load()
	settings.mu.Lock()
	settings.data.TdxDir = dir
	err := settings.save()
	out := settings.data
	settings.mu.Unlock()
	if err != nil {
		return Settings{}, err
	}
	return out, nil
}
