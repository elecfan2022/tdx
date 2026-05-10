package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/injoyai/tdx/protocol"
)

// 通达信本地数据 .day 文件格式（32 字节/条）：
//   bytes  0- 3  uint32 LE  日期 YYYYMMDD（如 20250407）
//   bytes  4- 7  uint32 LE  开盘价 × 100（分）
//   bytes  8-11  uint32 LE  最高价 × 100
//   bytes 12-15  uint32 LE  最低价 × 100
//   bytes 16-19  uint32 LE  收盘价 × 100
//   bytes 20-23  float32 LE 成交额（元）
//   bytes 24-27  uint32 LE  成交量（股；指数为手 × 100）
//   bytes 28-31  保留字段
const tdxDayRecordSize = 32

// readTdxDayFile 解析单个通达信 .day 文件，按时间正序返回 KlineBar
func readTdxDayFile(path string) ([]KlineBar, error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bs) == 0 {
		return nil, nil
	}
	if len(bs)%tdxDayRecordSize != 0 {
		return nil, fmt.Errorf("invalid .day file size %d (not multiple of %d)", len(bs), tdxDayRecordSize)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	n := len(bs) / tdxDayRecordSize
	out := make([]KlineBar, 0, n)
	for i := 0; i < n; i++ {
		rec := bs[i*tdxDayRecordSize : (i+1)*tdxDayRecordSize]
		date := binary.LittleEndian.Uint32(rec[0:4])
		if date < 19900101 || date > 21000101 {
			// 跳过明显坏掉的记录
			continue
		}
		open := float64(int32(binary.LittleEndian.Uint32(rec[4:8]))) / 100
		high := float64(int32(binary.LittleEndian.Uint32(rec[8:12]))) / 100
		low := float64(int32(binary.LittleEndian.Uint32(rec[12:16]))) / 100
		close_ := float64(int32(binary.LittleEndian.Uint32(rec[16:20]))) / 100
		amount := float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[20:24])))
		volume := int64(binary.LittleEndian.Uint32(rec[24:28]))
		y := int(date / 10000)
		m := int(date/100) % 100
		d := int(date % 100)
		// 用 15:00（收盘时间）作为日 K 时间戳，与服务端 GetKlineDay 对齐风格
		t := time.Date(y, time.Month(m), d, 15, 0, 0, 0, loc)
		out = append(out, KlineBar{
			Timestamp: t.UnixMilli(),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close_,
			Volume:    volume,
			Turnover:  amount,
		})
	}
	return out, nil
}

// tdxDayFilePath 给定 6 位代码，返回通达信本地 .day 文件路径
//   D:\new_tdx\vipdoc\sh\lday\sh999999.day
//   D:\new_tdx\vipdoc\sz\lday\sz000001.day
func tdxDayFilePath(tdxDir, code string) (string, error) {
	full := protocol.AddPrefix(code)
	if len(full) < 8 {
		return "", fmt.Errorf("invalid code: %s", code)
	}
	exch := full[:2]
	switch exch {
	case "sh", "sz", "bj":
	default:
		return "", fmt.Errorf("unknown exchange %q in code %q", exch, code)
	}
	return filepath.Join(tdxDir, "vipdoc", exch, "lday", full+".day"), nil
}

// aggregateDailyToWeekly 把日 K 聚合为周 K（周一-周日，按 ISO 周）
// 注意：A 股一周通常 5 个交易日，节假日导致更少；按周起始日（周一）打 bucket
func aggregateDailyToWeekly(daily []KlineBar) []KlineBar {
	return aggregateBy(daily, func(t time.Time) (int, int, int) {
		y, w := t.ISOWeek()
		return y, w, 0
	})
}

// aggregateDailyToMonthly 把日 K 聚合为月 K
func aggregateDailyToMonthly(daily []KlineBar) []KlineBar {
	return aggregateBy(daily, func(t time.Time) (int, int, int) {
		return t.Year(), int(t.Month()), 0
	})
}

// aggregateBy 通用聚合：用 keyFn 把每根 K 线打到一个分组，每组聚合成一根 K 线
func aggregateBy(daily []KlineBar, keyFn func(time.Time) (int, int, int)) []KlineBar {
	if len(daily) == 0 {
		return nil
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	type bucket struct {
		first  *KlineBar
		last   *KlineBar
		high   float64
		low    float64
		volume int64
		amount float64
		key    [3]int
	}
	var (
		buckets []*bucket
		curKey  [3]int
		curBkt  *bucket
	)
	for i := range daily {
		k := daily[i]
		t := time.UnixMilli(k.Timestamp).In(loc)
		a, b, c := keyFn(t)
		key := [3]int{a, b, c}
		if curBkt == nil || key != curKey {
			curKey = key
			curBkt = &bucket{
				first:  &daily[i],
				last:   &daily[i],
				high:   k.High,
				low:    k.Low,
				volume: k.Volume,
				amount: k.Turnover,
				key:    key,
			}
			buckets = append(buckets, curBkt)
			continue
		}
		curBkt.last = &daily[i]
		if k.High > curBkt.high {
			curBkt.high = k.High
		}
		if k.Low < curBkt.low {
			curBkt.low = k.Low
		}
		curBkt.volume += k.Volume
		curBkt.amount += k.Turnover
	}
	out := make([]KlineBar, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, KlineBar{
			Timestamp: b.last.Timestamp,
			Open:      b.first.Open,
			High:      b.high,
			Low:       b.low,
			Close:     b.last.Close,
			Volume:    b.volume,
			Turnover:  b.amount,
		})
	}
	return out
}
