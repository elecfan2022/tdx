package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/injoyai/tdx/protocol"
)

const watchlistPath = "./data/watchlist.json"

// WatchItem 自选股一项
type WatchItem struct {
	Code     string `json:"code"`     // 6 位
	FullCode string `json:"fullCode"` // sz000001
	Name     string `json:"name"`
}

var codeRe = regexp.MustCompile(`^\d{6}$`)

type watchStore struct {
	mu    sync.Mutex
	items []WatchItem
}

var watch = &watchStore{}

func (w *watchStore) load() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.items != nil {
		return
	}
	w.items = []WatchItem{}
	bs, err := os.ReadFile(watchlistPath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(bs, &w.items)
}

func (w *watchStore) save() error {
	if err := os.MkdirAll(filepath.Dir(watchlistPath), 0755); err != nil {
		return err
	}
	bs, err := json.MarshalIndent(w.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(watchlistPath, bs, 0644)
}

// GetWatchlist 返回自选股列表，必要时补充最新名称
func (a *App) GetWatchlist() []WatchItem {
	watch.load()
	watch.mu.Lock()
	defer watch.mu.Unlock()

	a.codesMu.RLock()
	codes := a.codes
	ready := a.codesReady
	a.codesMu.RUnlock()

	out := make([]WatchItem, len(watch.items))
	for i, it := range watch.items {
		if it.Name == "" && ready && codes != nil {
			it.Name = codes.GetName(it.FullCode)
			watch.items[i].Name = it.Name
		}
		out[i] = it
	}
	return out
}

// AddToWatchlist 把代码加入自选股，去重
func (a *App) AddToWatchlist(code string) ([]WatchItem, error) {
	if !codeRe.MatchString(code) {
		return nil, errors.New("股票代码必须是 6 位数字")
	}
	full := protocol.AddPrefix(code)

	a.codesMu.RLock()
	name := ""
	if a.codesReady && a.codes != nil {
		name = a.codes.GetName(full)
	}
	a.codesMu.RUnlock()

	watch.load()
	watch.mu.Lock()
	for _, it := range watch.items {
		if it.Code == code {
			watch.mu.Unlock()
			return a.GetWatchlist(), nil
		}
	}
	watch.items = append(watch.items, WatchItem{Code: code, FullCode: full, Name: name})
	err := watch.save()
	watch.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return a.GetWatchlist(), nil
}

// ReorderWatchlist 按给定代码顺序重排自选股
// codes 应是当前自选股代码的一个排列；不在列表中的代码忽略，
// 漏掉的旧条目追加到末尾以避免数据丢失
func (a *App) ReorderWatchlist(codes []string) ([]WatchItem, error) {
	watch.load()
	watch.mu.Lock()
	defer watch.mu.Unlock()

	byCode := make(map[string]WatchItem, len(watch.items))
	for _, it := range watch.items {
		byCode[it.Code] = it
	}

	seen := make(map[string]bool, len(codes))
	out := make([]WatchItem, 0, len(watch.items))
	for _, c := range codes {
		if it, ok := byCode[c]; ok && !seen[c] {
			out = append(out, it)
			seen[c] = true
		}
	}
	for _, it := range watch.items {
		if !seen[it.Code] {
			out = append(out, it)
		}
	}

	watch.items = out
	if err := watch.save(); err != nil {
		return nil, err
	}
	cp := make([]WatchItem, len(watch.items))
	copy(cp, watch.items)
	return cp, nil
}

// RemoveFromWatchlist 删除自选股
func (a *App) RemoveFromWatchlist(code string) ([]WatchItem, error) {
	watch.load()
	watch.mu.Lock()
	out := watch.items[:0]
	for _, it := range watch.items {
		if it.Code != code {
			out = append(out, it)
		}
	}
	watch.items = out
	err := watch.save()
	watch.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return a.GetWatchlist(), nil
}
