package main

import (
	"context"
	"fmt"
	"sync"
	"time"

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

// BiDiagnosis 笔成立诊断结果，给前端控制台打印用
type BiDiagnosis struct {
	FromFound bool    `json:"fromFound"`
	ToFound   bool    `json:"toFound"`
	From      Fractal `json:"from,omitempty"`
	To        Fractal `json:"to,omitempty"`
	IndexDist int     `json:"indexDist"` // 处理后序列下标差
	PeakDist  int     `json:"peakDist"`  // 真实峰/谷在原始序列的间隔（不含两端）
	Rule1     string  `json:"rule1"`     // 距离 ≥ 3
	Rule2     string  `json:"rule2"`     // PeakIdx 间隔 ≥ 3
	Rule3     string  `json:"rule3"`     // 顶 KHigh > 底 KHigh
	AllPass   bool    `json:"allPass"`
	Note      string  `json:"note"`
}

// DiagnoseBi 取两个日期上的分型，逐条规则跑一遍，告诉前端每条结果与数值
// 日期格式：YYYY-MM-DD（按上海时区比对，规避时间戳精确匹配的时区差）
func (a *App) DiagnoseBi(code, period, fromDate, toDate string) (*BiDiagnosis, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	formatDate := func(ts int64) string {
		return time.UnixMilli(ts).In(loc).Format("2006-01-02")
	}

	resp, err := a.GetKline(code, period, 5000)
	if err != nil {
		return nil, err
	}

	diag := &BiDiagnosis{}
	var fromFx, toFx *Fractal
	for i := range resp.Fractals {
		d := formatDate(resp.Fractals[i].Timestamp)
		if d == fromDate {
			fromFx = &resp.Fractals[i]
		}
		if d == toDate {
			toFx = &resp.Fractals[i]
		}
	}
	diag.FromFound = fromFx != nil
	diag.ToFound = toFx != nil
	if fromFx == nil || toFx == nil {
		diag.Note = fmt.Sprintf("未找到对应日期的分型（输入了 from=%s, to=%s；列表里日期是用上海时区显示的，请用同样格式输入）",
			fromDate, toDate)
		return diag, nil
	}
	diag.From = *fromFx
	diag.To = *toFx

	if fromFx.Type == toFx.Type {
		diag.Note = "两个分型类型相同（都是顶或都是底），不能成笔"
		return diag, nil
	}

	diag.IndexDist = absInt(fromFx.Index - toFx.Index)
	earlier, later := *fromFx, *toFx
	if earlier.PeakIdx > later.PeakIdx {
		earlier, later = *toFx, *fromFx
	}
	diag.PeakDist = later.PeakIdx - earlier.PeakIdx - 1

	rule1Pass := diag.IndexDist >= 3
	rule2Pass := diag.PeakDist >= 3
	var top, bottom Fractal
	if fromFx.Type == "top" {
		top, bottom = *fromFx, *toFx
	} else {
		top, bottom = *toFx, *fromFx
	}
	rule3Pass := top.KHigh > bottom.KHigh

	diag.Rule1 = fmt.Sprintf("处理后下标距离 %d ≥ 3 → %v", diag.IndexDist, rule1Pass)
	diag.Rule2 = fmt.Sprintf("PeakIdx 间隔 %d ≥ 3 → %v", diag.PeakDist, rule2Pass)
	diag.Rule3 = fmt.Sprintf("顶 KHigh %.2f > 底 KHigh %.2f → %v", top.KHigh, bottom.KHigh, rule3Pass)
	diag.AllPass = rule1Pass && rule2Pass && rule3Pass

	if !diag.AllPass {
		fail := []string{}
		if !rule1Pass {
			fail = append(fail, "规则1")
		}
		if !rule2Pass {
			fail = append(fail, "规则2")
		}
		if !rule3Pass {
			fail = append(fail, "规则3")
		}
		diag.Note = fmt.Sprintf("不满足：%v", fail)
	} else {
		diag.Note = "三条规则都满足。若仍不成笔，可能在 buildBi 阶段被 pending/replace 流程淘汰。"
	}
	return diag, nil
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
