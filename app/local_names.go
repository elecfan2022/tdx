package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// 通达信 .tnf 文件存放股票/指数代码 + 名称等元信息，格式不公开。
// 现行通达信版本最常见的格式是 314 字节/条；少数老版本可能是 50 字节/条。
//
// 这里走"试多种 layout，挑解析出最多合法名称的那种"的策略：
//   - 偏移 0..5：代码（6 位 ASCII 数字）
//   - 名称编码 GBK，长度多在 8-9 字节
//
// 文件位置：
//   T0002/hq_cache/shm.tnf  上海
//   T0002/hq_cache/szm.tnf  深圳
//   T0002/hq_cache/bjm.tnf  北交所

// loadTnfNames 扫描 hq_cache 下三个 .tnf 文件，返回 map[fullCode]name
// 任何失败都不报错，最多返回空 map（让 buildLocalCodes 走 sqlite 缓存兜底）
func loadTnfNames(tdxDir string) map[string]string {
	out := map[string]string{}
	files := map[string]string{
		"sh": "shm.tnf",
		"sz": "szm.tnf",
		"bj": "bjm.tnf",
	}
	for exch, fname := range files {
		path := filepath.Join(tdxDir, "T0002", "hq_cache", fname)
		for code, name := range parseTnfFile(path) {
			out[exch+code] = name
		}
	}
	return out
}

// tnfLayout 描述一种可能的 .tnf 记录布局
type tnfLayout struct {
	recordSize int
	headerSkip int
	codeOff    int
	codeLen    int
	nameOff    int
	nameLen    int
}

var tnfLayoutCandidates = []tnfLayout{
	// 主流 314 字节布局
	{314, 0, 0, 6, 23, 9},
	{314, 50, 0, 6, 23, 9},
	{314, 0, 0, 6, 8, 16},
	// 老式 50 字节布局
	{50, 0, 0, 6, 6, 8},
	{50, 0, 0, 6, 8, 16},
}

// parseTnfFile 多 layout 尝试，挑能解析出最多合法名称的版本
func parseTnfFile(path string) map[string]string {
	bs, err := os.ReadFile(path)
	if err != nil || len(bs) == 0 {
		return nil
	}
	var best map[string]string
	for _, layout := range tnfLayoutCandidates {
		m := tryParseTnf(bs, layout)
		if len(m) > len(best) {
			best = m
		}
	}
	return best
}

func tryParseTnf(bs []byte, l tnfLayout) map[string]string {
	if l.recordSize <= 0 {
		return nil
	}
	body := bs
	if l.headerSkip > 0 {
		if len(body) <= l.headerSkip {
			return nil
		}
		body = body[l.headerSkip:]
	}
	if len(body)%l.recordSize != 0 {
		return nil
	}
	n := len(body) / l.recordSize
	if n == 0 {
		return nil
	}
	out := make(map[string]string, n)
	decoder := simplifiedchinese.GBK.NewDecoder()
	for i := 0; i < n; i++ {
		rec := body[i*l.recordSize : (i+1)*l.recordSize]
		if l.codeOff+l.codeLen > len(rec) || l.nameOff+l.nameLen > len(rec) {
			return nil
		}
		code := string(rec[l.codeOff : l.codeOff+l.codeLen])
		if !isAllDigits(code) {
			continue
		}
		nameRaw := rec[l.nameOff : l.nameOff+l.nameLen]
		end := bytes.IndexByte(nameRaw, 0)
		if end < 0 {
			end = len(nameRaw)
		}
		if end == 0 {
			continue
		}
		nameUtf, _, err := transform.Bytes(decoder, nameRaw[:end])
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(nameUtf))
		if !looksLikeStockName(name) {
			continue
		}
		out[code] = name
	}
	return out
}

func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// looksLikeStockName 启发式判断：长度 1-12 个 rune，全部可打印，且包含至少一个
// 中文 / 字母 / 数字（避免乱码"看起来像名称"）
func looksLikeStockName(s string) bool {
	if s == "" {
		return false
	}
	runes := []rune(s)
	if len(runes) > 12 {
		return false
	}
	hasMeaning := false
	for _, r := range runes {
		if !unicode.IsPrint(r) {
			return false
		}
		if unicode.Is(unicode.Han, r) || unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasMeaning = true
		}
	}
	return hasMeaning
}
