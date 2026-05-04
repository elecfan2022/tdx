package main

import "fmt"

// 缠论基础结构：包含关系处理、分型识别、笔的构造（新笔规则）

// ProcessedKline 是经过包含关系处理后的合并 K 线
// OrigStartIdx/OrigEndIdx 记录该合并 K 线对应原始 K 线的下标范围
// HighIdx/LowIdx 记录在原始序列中真实达到本 K 线高/低的下标
type ProcessedKline struct {
	Open         float64
	High         float64
	Low          float64
	Close        float64
	Timestamp    int64
	OrigStartIdx int
	OrigEndIdx   int
	HighIdx      int
	LowIdx       int
}

// processContainment 按缠论规则处理 K 线包含关系
// 方向判定：以处理后序列中前两根的高点比较为准；首次包含无前序时默认向上
func processContainment(klines []KlineBar) []ProcessedKline {
	if len(klines) == 0 {
		return nil
	}
	out := []ProcessedKline{
		{
			Open:         klines[0].Open,
			High:         klines[0].High,
			Low:          klines[0].Low,
			Close:        klines[0].Close,
			Timestamp:    klines[0].Timestamp,
			OrigStartIdx: 0,
			OrigEndIdx:   0,
			HighIdx:      0,
			LowIdx:       0,
		},
	}
	for i := 1; i < len(klines); i++ {
		cur := klines[i]
		last := &out[len(out)-1]
		contains := (last.High >= cur.High && last.Low <= cur.Low) ||
			(cur.High >= last.High && cur.Low <= last.Low)
		if !contains {
			out = append(out, ProcessedKline{
				Open:         cur.Open,
				High:         cur.High,
				Low:          cur.Low,
				Close:        cur.Close,
				Timestamp:    cur.Timestamp,
				OrigStartIdx: i,
				OrigEndIdx:   i,
				HighIdx:      i,
				LowIdx:       i,
			})
			continue
		}
		// 方向判定：取处理后序列前一根 vs 当前 last
		dir := 1 // 默认向上
		if len(out) >= 2 {
			prev := out[len(out)-2]
			if last.High <= prev.High {
				dir = -1
			}
		}
		if dir == 1 {
			// 向上合并：高高低低取大
			if cur.High >= last.High {
				last.High = cur.High
				last.HighIdx = i
			}
			if cur.Low >= last.Low {
				last.Low = cur.Low
				last.LowIdx = i
			}
		} else {
			// 向下合并：高高低低取小
			if cur.High <= last.High {
				last.High = cur.High
				last.HighIdx = i
			}
			if cur.Low <= last.Low {
				last.Low = cur.Low
				last.LowIdx = i
			}
		}
		last.OrigEndIdx = i
		last.Close = cur.Close
	}
	return out
}

// Fractal 分型
// Index 为处理后序列中的中间根下标
// Timestamp/Price 指向原始 K 线序列中实际峰/谷那根
// PeakIdx 原始序列中达到峰（顶）/ 谷（底）那一根的下标，用于规则 2 计数
// KHigh/KLow 是 PeakIdx 那根原始 K 线的高低价区间，用于规则 3 比较
// IsEndpoint 该分型是否成为某条笔的端点（buildBi 完成后由 AnalyzeChan 回填）
type Fractal struct {
	Type         string  `json:"type"` // "top" / "bottom"
	Index        int     `json:"index"`
	Timestamp    int64   `json:"timestamp"`
	Price        float64 `json:"price"`
	OrigStartIdx int     `json:"origStartIdx"`
	OrigEndIdx   int     `json:"origEndIdx"`
	PeakIdx      int     `json:"peakIdx"`
	KHigh        float64 `json:"kHigh"`
	KLow         float64 `json:"kLow"`
	IsEndpoint   bool    `json:"isEndpoint"`
}

// findFractals 在处理后序列中识别顶/底分型
// 顶分型：中间根 high 高于左右、low 也高于左右
// 底分型：中间根 low 低于左右、high 也低于左右
func findFractals(p []ProcessedKline, klines []KlineBar) []Fractal {
	out := []Fractal{}
	for i := 1; i < len(p)-1; i++ {
		prev, cur, next := p[i-1], p[i], p[i+1]
		if cur.High > prev.High && cur.High > next.High &&
			cur.Low > prev.Low && cur.Low > next.Low {
			peak := cur.HighIdx
			out = append(out, Fractal{
				Type:         "top",
				Index:        i,
				Timestamp:    klines[peak].Timestamp,
				Price:        cur.High,
				OrigStartIdx: cur.OrigStartIdx,
				OrigEndIdx:   cur.OrigEndIdx,
				PeakIdx:      peak,
				KHigh:        klines[peak].High,
				KLow:         klines[peak].Low,
			})
		}
		if cur.Low < prev.Low && cur.Low < next.Low &&
			cur.High < prev.High && cur.High < next.High {
			peak := cur.LowIdx
			out = append(out, Fractal{
				Type:         "bottom",
				Index:        i,
				Timestamp:    klines[peak].Timestamp,
				Price:        cur.Low,
				OrigStartIdx: cur.OrigStartIdx,
				OrigEndIdx:   cur.OrigEndIdx,
				PeakIdx:      peak,
				KHigh:        klines[peak].High,
				KLow:         klines[peak].Low,
			})
		}
	}
	return out
}

// Bi 笔，由两个相邻分型构成
type Bi struct {
	From Fractal `json:"from"`
	To   Fractal `json:"to"`
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// biRulesSatisfied 检查两分型之间是否满足新笔规则：
//  1. 处理后序列中两分型不共用 K 线（中根下标差 >=3，等价于两分型之间隔
//     >=2 根处理后 K 线）
//  2. 顶分型最高 K 线和底分型最低 K 线之间（不含两端、不考虑包含关系），
//     原始 K 线 >=3 根 —— 用各自的 PeakIdx（实际峰/谷在原始序列里的下标）
//     来计数，包含合并不会"吃掉"距离
//  3. 顶分型最高 K 线的高低价区间至少有一部分高于底分型最低 K 线的区间
//     —— 即顶 K 线的最高价必须严格高于底 K 线的最高价（否则两 K 线区间
//     完全反置或重叠在底 K 线之内，不构成"上有下"的笔）
//
// 规则 2 的 PeakIdx 实现是从下面这个测例发现的 bug 修过来的：
// 用例参考《缠中说禅 · 教你炒股票 69：月线分段与上海大走势分析、预判》
//   - 标的：上证指数 999999 月线
//   - 顶分型 1991-01-31 与底分型 1991-05-31 应当成笔
//   - 旧实现用合并块边界 OrigEnd/OrigStart 计数，若 1991-01 因包含合并把
//     1991-02 吞入（OrigEnd=2），算出的 between=5-2-1=2 < 3，错判为不成笔
//   - 新实现用真实峰/谷的 PeakIdx 计数：5-1-1=3，正确成笔
func biRulesSatisfied(a, b Fractal) bool {
	if absInt(a.Index-b.Index) < 3 {
		return false
	}
	earlier, later := a, b
	if earlier.PeakIdx > later.PeakIdx {
		earlier, later = b, a
	}
	between := later.PeakIdx - earlier.PeakIdx - 1
	if between < 3 {
		return false
	}
	// 规则 3：顶 K 线最高价必须高于底 K 线最高价
	var top, bottom Fractal
	if a.Type == "top" {
		top, bottom = a, b
	} else {
		top, bottom = b, a
	}
	return top.KHigh > bottom.KHigh
}

// moreExtreme 判断 fx 是否比 base 更极端（顶看更高、底看更低）
func moreExtreme(fx, base Fractal) bool {
	if fx.Type == "top" {
		return fx.Price > base.Price
	}
	return fx.Price < base.Price
}

// buildBi 按新笔规则把分型链转成笔
//
// 实现要点：
//  1. 第一对端点用"惰性确认"，避免错把"小早顶/底"当首端点导致后续 pending 丢失。
//     具体：维护 candA（先出现的候选）和 candB（候选的反向分型），同向更极端
//     时替换 candA 并复查 candA-candB 规则，规则一通过就把这两个按时间序锁定
//     成前两个端点。
//  2. 锁定后切换到贪心模式：同向更极端就替换 last（清掉 pending），反向时
//     维护 pending 为最极端反向分型，规则通过即追加为新端点。
func buildBi(fractals []Fractal) []Bi {
	if len(fractals) < 2 {
		return nil
	}

	var endpoints []Fractal
	var candA, candB *Fractal // 仅在 endpoints 为空时使用
	var pending *Fractal      // 锁定后使用

	confirmFirstPair := func(a, b Fractal) {
		if a.Index < b.Index {
			endpoints = []Fractal{a, b}
		} else {
			endpoints = []Fractal{b, a}
		}
		candA = nil
		candB = nil
	}

	for i := 0; i < len(fractals); i++ {
		fx := fractals[i]

		if len(endpoints) == 0 {
			// === 阶段一：尚未确认任何端点 ===
			if candA == nil {
				tmp := fx
				candA = &tmp
				continue
			}
			if fx.Type == candA.Type {
				// 同类型：更极端则替换 candA，再复查与 candB 的规则
				if moreExtreme(fx, *candA) {
					tmp := fx
					candA = &tmp
					if candB != nil && biRulesSatisfied(*candB, *candA) {
						confirmFirstPair(*candA, *candB)
					}
				}
				continue
			}
			// 反向类型：维护 candB 为最极端
			if candB == nil {
				tmp := fx
				candB = &tmp
			} else if moreExtreme(fx, *candB) {
				tmp := fx
				candB = &tmp
			}
			if biRulesSatisfied(*candA, *candB) {
				confirmFirstPair(*candA, *candB)
			}
			continue
		}

		// === 阶段二：已锁定至少 2 个端点，走贪心算法 ===
		last := &endpoints[len(endpoints)-1]
		if fx.Type == last.Type {
			if moreExtreme(fx, *last) {
				*last = fx
				pending = nil
			}
			continue
		}
		if pending == nil {
			tmp := fx
			pending = &tmp
		} else if moreExtreme(fx, *pending) {
			tmp := fx
			pending = &tmp
		}
		if biRulesSatisfied(*last, *pending) {
			endpoints = append(endpoints, *pending)
			pending = nil
		}
	}

	if len(endpoints) < 2 {
		return nil
	}
	bis := make([]Bi, 0, len(endpoints)-1)
	for i := 0; i+1 < len(endpoints); i++ {
		bis = append(bis, Bi{From: endpoints[i], To: endpoints[i+1]})
	}
	return bis
}

// AnalyzeChan 把缠论分析输出聚合，供 GetKline 一次性返回
type ChanAnalysis struct {
	Fractals []Fractal `json:"fractals"`
	Bis      []Bi      `json:"bis"`
}

func AnalyzeChan(klines []KlineBar) ChanAnalysis {
	if len(klines) < 3 {
		return ChanAnalysis{Fractals: []Fractal{}, Bis: []Bi{}}
	}
	processed := processContainment(klines)
	fractals := findFractals(processed, klines)
	bis := buildBi(fractals)
	if fractals == nil {
		fractals = []Fractal{}
	}
	if bis == nil {
		bis = []Bi{}
	}
	// 回填 IsEndpoint：以 (Timestamp, Type) 为键，把所有笔端点对应的分型标记为
	// 端点，便于前端展示哪些分型实际成笔
	endpointKey := func(f Fractal) string {
		return fmt.Sprintf("%d-%s", f.Timestamp, f.Type)
	}
	endpointSet := make(map[string]bool)
	for _, bi := range bis {
		endpointSet[endpointKey(bi.From)] = true
		endpointSet[endpointKey(bi.To)] = true
	}
	for i := range fractals {
		if endpointSet[endpointKey(fractals[i])] {
			fractals[i].IsEndpoint = true
		}
	}
	return ChanAnalysis{Fractals: fractals, Bis: bis}
}
