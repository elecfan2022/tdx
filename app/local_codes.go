package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/glebarez/go-sqlite"
	"github.com/injoyai/tdx"
)

// loadLocalCodes 扫描通达信本地目录下 vipdoc/<exch>/lday，列出所有有日 K
// 文件的代码，构造 CodeModel 列表。仅返回代码与交易所，不含名称。
//
// 名称单独从 ./data/database/codes.db（之前实时拉过则会有）尝试合并补回。
// 这样首次纯本地启动也能拿到代码列表（毫秒级），有缓存时也能拿到名称。
func loadLocalCodes(tdxDir string) ([]*tdx.CodeModel, error) {
	if tdxDir == "" {
		return nil, fmt.Errorf("tdx dir is empty")
	}
	out := []*tdx.CodeModel{}
	for _, exch := range []string{"sh", "sz", "bj"} {
		dir := filepath.Join(tdxDir, "vipdoc", exch, "lday")
		entries, err := os.ReadDir(dir)
		if err != nil {
			// 该交易所目录不存在，跳过
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.EqualFold(filepath.Ext(name), ".day") {
				continue
			}
			base := strings.TrimSuffix(name, filepath.Ext(name))
			// 文件名形如 sh000001.day，剥离 ext 后应该是 8 字符
			if len(base) != 8 {
				continue
			}
			if !strings.EqualFold(base[:2], exch) {
				continue
			}
			code := base[2:]
			out = append(out, &tdx.CodeModel{
				Code:     code,
				Exchange: exch,
			})
		}
	}
	return out, nil
}

// loadCachedNames 尽力读取 ./data/database/codes.db 里之前实时拉取并缓存
// 的代码名称表。失败/不存在均返回空 map（不算错误）。
//
// 该 sqlite 是 tdx.NewCodes() 默认写入的位置；如果用户曾经联网启动过本应用
// 一次，文件就在那。
func loadCachedNames() map[string]string {
	out := map[string]string{}
	dbPath := "./data/database/codes.db"
	if _, err := os.Stat(dbPath); err != nil {
		return out
	}
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return out
	}
	defer db.Close()
	rows, err := db.Query("SELECT exchange, code, name FROM codes")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var exch, code, name string
		if err := rows.Scan(&exch, &code, &name); err != nil {
			continue
		}
		out[exch+code] = name
	}
	return out
}

// buildLocalCodes 扫描 + 合并名称，返回填好 ICodes 的实例
func buildLocalCodes(tdxDir string) (tdx.ICodes, int, error) {
	list, err := loadLocalCodes(tdxDir)
	if err != nil {
		return nil, 0, err
	}
	if len(list) == 0 {
		return nil, 0, fmt.Errorf("local tdx dir %q has no .day files", tdxDir)
	}
	nameMap := loadCachedNames()
	for _, cm := range list {
		if name, ok := nameMap[cm.FullCode()]; ok {
			cm.Name = name
		}
	}
	cb := tdx.NewCodesBase()
	cb.Update(list)
	return cb, len(list), nil
}
