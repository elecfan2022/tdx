package main

import (
	"context"
	"fmt"
	"sync"

	tdx "github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

// App 是 Wails 绑定的根对象
type App struct {
	ctx context.Context

	mu      sync.Mutex
	client  *tdx.Client
	dialErr error

	// 代码字典（含名称），首次启动会从行情服务器下载并缓存到本地 sqlite
	codesMu    sync.RWMutex
	codes      tdx.ICodes
	codesReady bool
	codesErr   error
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 后台拉取股票代码字典，不阻塞窗口启动
	go a.initCodes()
}

func (a *App) initCodes() {
	cs, err := tdx.NewCodes()
	a.codesMu.Lock()
	defer a.codesMu.Unlock()
	if err != nil {
		a.codesErr = err
		return
	}
	a.codes = cs
	a.codesReady = true
}

// ensureClient 惰性建立到通达信行情服务器的连接，复用单连接
func (a *App) ensureClient() (*tdx.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil {
		return a.client, nil
	}
	a.dialErr = nil
	cli, err := tdx.DialDefault()
	if err != nil {
		a.dialErr = err
		return nil, err
	}
	a.client = cli
	return cli, nil
}

// KlineBar 是给前端的 K 线结构（价格已换算成元）
// Timestamp 单位毫秒，KLineChart 直接吃这个字段
type KlineBar struct {
	Timestamp int64   `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
	Turnover  float64 `json:"turnover"`
}

// periodToType 把前端周期字符串映射为 tdx 协议 Type 字节
func periodToType(period string) (uint8, error) {
	switch period {
	case "1m":
		return protocol.TypeKlineMinute, nil
	case "5m":
		return protocol.TypeKline5Minute, nil
	case "30m":
		return protocol.TypeKline30Minute, nil
	case "day":
		return protocol.TypeKlineDay, nil
	case "week":
		return protocol.TypeKlineWeek, nil
	case "month":
		return protocol.TypeKlineMonth, nil
	default:
		return 0, fmt.Errorf("unknown period: %s", period)
	}
}

// KlineWithChan 给前端的完整数据包：原始 K 线 + 缠论分析（分型 + 笔）
type KlineWithChan struct {
	Klines   []KlineBar `json:"klines"`
	Fractals []Fractal  `json:"fractals"`
	Bis      []Bi       `json:"bis"`
}

// GetKline 拉取最近 count 根 K 线，附带缠论分型与笔
// TDX 单次上限 800，超过会自动分页（最多 24000 根）
func (a *App) GetKline(code string, period string, count int) (*KlineWithChan, error) {
	if count <= 0 {
		count = 5000
	}
	if count > 24000 {
		count = 24000
	}
	t, err := periodToType(period)
	if err != nil {
		return nil, err
	}
	cli, err := a.ensureClient()
	if err != nil {
		return nil, fmt.Errorf("连接行情服务器失败: %w", err)
	}

	// 指数（如上证 999999、000001、深成指 399001 等）和股票请求帧一样，
	// 但服务端响应的解析方式不同，需要分别走 GetIndex / GetKline
	isIndex := protocol.IsIndex(protocol.AddPrefix(code))

	const batch = uint16(800)
	collected := []*protocol.Kline{}
	for offset := uint16(0); int(offset) < count; offset += batch {
		size := batch
		if int(offset)+int(size) > count {
			size = uint16(count - int(offset))
		}
		var resp *protocol.KlineResp
		if isIndex {
			resp, err = cli.GetIndex(t, code, offset, size)
		} else {
			resp, err = cli.GetKline(t, code, offset, size)
		}
		if err != nil {
			return nil, fmt.Errorf("拉取K线失败: %w", err)
		}
		// 协议返回的是按时间正序的一段，把它接到已收集片段的前面
		collected = append(resp.List, collected...)
		// 实际返回不足 size，说明已经拿到最早数据
		if resp.Count < size {
			break
		}
	}

	out := make([]KlineBar, 0, len(collected))
	for _, k := range collected {
		out = append(out, KlineBar{
			Timestamp: k.Time.UnixMilli(),
			Open:      k.Open.Float64(),
			High:      k.High.Float64(),
			Low:       k.Low.Float64(),
			Close:     k.Close.Float64(),
			Volume:    k.Volume,
			Turnover:  k.Amount.Float64(),
		})
	}
	chan_ := AnalyzeChan(out)
	return &KlineWithChan{
		Klines:   out,
		Fractals: chan_.Fractals,
		Bis:      chan_.Bis,
	}, nil
}

// StockInfo 给前端的个股标识
type StockInfo struct {
	Code     string `json:"code"`     // 6 位
	FullCode string `json:"fullCode"` // sz000001
	Name     string `json:"name"`     // 平安银行；尚未就绪时为空字符串
}

// GetStockInfo 返回 6 位代码对应的全代码与名称
// 若代码字典还在加载中，name 会为空，前端可定时再调一次
func (a *App) GetStockInfo(code string) StockInfo {
	full := protocol.AddPrefix(code)
	info := StockInfo{Code: code, FullCode: full}
	a.codesMu.RLock()
	defer a.codesMu.RUnlock()
	if a.codesReady && a.codes != nil {
		info.Name = a.codes.GetName(full)
	}
	return info
}

// Status 给状态栏的实时信息
type Status struct {
	Connected   bool   `json:"connected"`
	CodesReady  bool   `json:"codesReady"`
	CodesError  string `json:"codesError"`
	DialError   string `json:"dialError"`
	StockCount  int    `json:"stockCount"`
}

func (a *App) GetStatus() Status {
	a.mu.Lock()
	connected := a.client != nil
	dialErr := ""
	if a.dialErr != nil {
		dialErr = a.dialErr.Error()
	}
	a.mu.Unlock()

	a.codesMu.RLock()
	defer a.codesMu.RUnlock()
	s := Status{
		Connected:  connected,
		CodesReady: a.codesReady,
		DialError:  dialErr,
	}
	if a.codesErr != nil {
		s.CodesError = a.codesErr.Error()
	}
	if a.codesReady && a.codes != nil {
		s.StockCount = len(a.codes.GetStockCodes())
	}
	return s
}
