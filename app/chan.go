package main

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
type Fractal struct {
	Type         string  `json:"type"` // "top" / "bottom"
	Index        int     `json:"index"`
	Timestamp    int64   `json:"timestamp"`
	Price        float64 `json:"price"`
	OrigStartIdx int     `json:"origStartIdx"`
	OrigEndIdx   int     `json:"origEndIdx"`
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
//  1. 处理后序列中不共用 K 线（中间根下标差至少 3，即两分型之间隔 >=1 根处理后 K 线）
//  2. 顶分型最高 K 线和底分型最低 K 线之间（不含两端），原始 K 线 >=3 根
func biRulesSatisfied(a, b Fractal) bool {
	if absInt(a.Index-b.Index) < 3 {
		return false
	}
	earlier, later := a, b
	if earlier.Index > later.Index {
		earlier, later = b, a
	}
	between := later.OrigStartIdx - earlier.OrigEndIdx - 1
	return between >= 3
}

// buildBi 按新笔规则把分型链转成笔
//   - 同向分型只保留更极端的（更高的顶 / 更低的底）
//   - 反向分型作为候选 pending，更极端时更新；满足规则即确认为下一个端点
func buildBi(fractals []Fractal) []Bi {
	if len(fractals) < 2 {
		return nil
	}
	endpoints := []Fractal{fractals[0]}
	var pending *Fractal

	for i := 1; i < len(fractals); i++ {
		fx := fractals[i]
		last := &endpoints[len(endpoints)-1]
		if fx.Type == last.Type {
			// 同向：替换为更极端者，并清空 pending
			replace := false
			if fx.Type == "top" && fx.Price > last.Price {
				replace = true
			}
			if fx.Type == "bottom" && fx.Price < last.Price {
				replace = true
			}
			if replace {
				*last = fx
				pending = nil
			}
			continue
		}
		// 反向：维护 pending 为最极端的反向分型
		if pending == nil {
			tmp := fx
			pending = &tmp
		} else {
			if (fx.Type == "top" && fx.Price > pending.Price) ||
				(fx.Type == "bottom" && fx.Price < pending.Price) {
				tmp := fx
				pending = &tmp
			}
		}
		// pending 是否满足规则即可确认成笔
		if biRulesSatisfied(*last, *pending) {
			endpoints = append(endpoints, *pending)
			pending = nil
		}
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
	return ChanAnalysis{Fractals: fractals, Bis: bis}
}
