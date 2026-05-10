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
// LeftIdx/RightIdx 分型左右两侧邻接处理后 K 线在原始序列里的代表点下标
//   （顶取 HighIdx 即"那根 K 线的最高点所在原始 K 线"；底取 LowIdx）
//   前端用来在 hover 峰/谷 K 线时提示构成分型的另两根 K 线
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
	LeftIdx      int     `json:"leftIdx"`
	RightIdx     int     `json:"rightIdx"`
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
				LeftIdx:      prev.HighIdx,
				RightIdx:     next.HighIdx,
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
				LeftIdx:      prev.LowIdx,
				RightIdx:     next.LowIdx,
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
// 大幅跳空缺口豁免：
//
//	若 earlier.PeakIdx 到 later.PeakIdx 之间存在任意一对相邻 K 线，区间不
//	重叠（K[i+1].Low > K[i].High 或 K[i+1].High < K[i].Low）且缺口尺寸 >=
//	相邻两根 K 线幅度（High-Low）较大者的 2 倍，即视为大幅跳空缺口。缺口
//	本身在缠论里相当于一段"被压缩的剧烈走势"，规则 1、2 的根数限制放宽，
//	只要规则 3 通过即可成笔。较小的跳空（如盘中两根紧挨着的小不重叠区间）
//	不享受此豁免。规则 3 始终生效。
//	典型用例：上证指数 999999 1 分钟图，T(2007-05-29 15:00) 与 B(2007-05-30
//	09:31) 因 530 印花税政策跨夜跳空，缺口约 280 点远大于相邻 K 线 ~30 点
//	的幅度，达到 2 倍以上，判定成笔。
//
// 规则 2 的 PeakIdx 实现是从下面这个测例发现的 bug 修过来的：
// 用例参考《缠中说禅 · 教你炒股票 69：月线分段与上海大走势分析、预判》
//   - 标的：上证指数 999999 月线
//   - 顶分型 1991-01-31 与底分型 1991-05-31 应当成笔
//   - 旧实现用合并块边界 OrigEnd/OrigStart 计数，若 1991-01 因包含合并把
//     1991-02 吞入（OrigEnd=2），算出的 between=5-2-1=2 < 3，错判为不成笔
//   - 新实现用真实峰/谷的 PeakIdx 计数：5-1-1=3，正确成笔
func biRulesSatisfied(a, b Fractal, klines []KlineBar) bool {
	earlier, later := a, b
	if earlier.PeakIdx > later.PeakIdx {
		earlier, later = b, a
	}

	// 规则 3 始终需要：顶 K 线最高价必须高于底 K 线最高价
	var top, bottom Fractal
	if a.Type == "top" {
		top, bottom = a, b
	} else {
		top, bottom = b, a
	}
	if top.KHigh <= bottom.KHigh {
		return false
	}

	// 大幅跳空缺口豁免规则 1、2 的根数限制
	if hasLargeGapBetween(earlier, later, klines) {
		return true
	}

	// 无缺口：常规距离规则
	if absInt(a.Index-b.Index) < 3 {
		return false
	}
	between := later.PeakIdx - earlier.PeakIdx - 1
	return between >= 3
}

// hasLargeGapBetween 检测 earlier.PeakIdx ~ later.PeakIdx 之间是否存在大幅
// 跳空缺口：任意相邻两根 K 线区间不重叠（K[i+1].Low > K[i].High 或 K[i+1].High
// < K[i].Low），且缺口尺寸 >= 2 * max(K[i].幅度, K[i+1].幅度)。
func hasLargeGapBetween(earlier, later Fractal, klines []KlineBar) bool {
	if earlier.PeakIdx > later.PeakIdx {
		earlier, later = later, earlier
	}
	for i := earlier.PeakIdx; i < later.PeakIdx && i+1 < len(klines); i++ {
		a, b := klines[i], klines[i+1]
		var gap float64
		switch {
		case b.Low > a.High:
			gap = b.Low - a.High
		case b.High < a.Low:
			gap = a.Low - b.High
		default:
			continue // 区间重叠，无缺口
		}
		rangeA := a.High - a.Low
		rangeB := b.High - b.Low
		maxRange := rangeA
		if rangeB > maxRange {
			maxRange = rangeB
		}
		if gap >= 2*maxRange {
			return true
		}
	}
	return false
}

// moreExtreme 判断 fx 是否比 base 更极端（顶看更高、底看更低）
func moreExtreme(fx, base Fractal) bool {
	if fx.Type == "top" {
		return fx.Price > base.Price
	}
	return fx.Price < base.Price
}

// buildBi 按新笔规则与"笔的可修改性"把分型链转成笔
//
// 实现要点：
//  1. 第一对端点用"惰性确认"：维护 candA（先出现的候选）和 candB（候选的反向
//     分型），同向更极端时替换 candA 并复查 candA-candB 规则；规则一通过就把
//     这两个按时间序锁定成前两个端点。这避免了错把"小早顶/底"当首端点。
//
//  2. 锁定后切换到贪心 + 笔修正：
//     - 反向 fx：维护 pending 为最极端反向分型，规则通过即追加为新端点。
//     - 同向更极端 fx：触发"笔修正"，分三种情况——
//       * Case 1（pending 不比 prev 更极端）：仅延伸 last，pending 弃。
//          直观含义：上一笔还未走完，新的更极端端点出现，但中间反向分型不够强
//          以撼动 prev → 直接把 last 平移到 fx。
//       * Case 2（pending 比 prev 更极端）：笔的另一端也需修正。先把 prev
//         替换成 pending（顶/底"延伸"），丢弃旧 last，再把循环回退到 pending
//         那一根，从 pending 之后的所有分型重新按笔规则扫一遍，看哪些可以成笔。
//          直观含义：上一笔的 prev 端被新出现的、更极端的反向分型证伪 → 笔
//          的"边"被改写后，pending 之后的所有分型必须以新边为起点重新审视。
//          这正是"分型可修改性"——一旦笔的端点变了，后面挂在 pending 但因
//          rule 失败没成笔的分型，可能在新链路下成笔。
//       * Case 3（Case 2 回退重扫的兜底）：如果从新 prev（即原 pending，更极端
//         的反向分型）开始扫描到原触发 fx 这段范围内，没能再成任何一笔，则
//         以"放宽规则集"判定"新 prev → 触发 fx"是否成笔——
//           - 规则 3 必须满足（top.KHigh > bottom.KHigh，确保"上有下"）
//           - 规则 1 必须满足（处理后下标距离 ≥ 3），否则需大幅跳空缺口豁免
//           - 规则 2 不要求（之间没有中继分型是 Case 3 的前提条件）
//         规则 1 失败且无大幅缺口则不强制成笔。这避免新 prev 与触发 fx 紧挨着
//         又无缺口时硬连成笔；同时保留"经 Case 2 修正确认、相距足够远"或
//         "之间存在大幅跳空"的极值对的成笔机会。
//
// 参考：
//   《缠中说禅 · 教你炒股票 69：月线分段与上海大走势分析、预判》——相邻两分型
//   若不能成笔，二者必只取其一；取舍由后续更极端的同向分型来"延伸"哪一边决定。
//   案例：
//   - 上证 999999 月线，T(1992-05-29) 与 B(1992-11-30) 不能成笔；后来出现
//     B(1994-07-29) 更低的底，使 B(1992-11-30) 失效，T(1993-02-26) 取代
//     T(1992-05-29)。
//   - 301153 日线，B(2025-04-07) 与 T(2025-04-03) 之间无法成笔；T(2025-07-14)
//     更高的顶 + B(04-07) 更低的底触发 case 2，回退后从 B(04-07) 重新扫到
//     T(07-14)，得到 B(04-07)→T(06-25)→B(07-03)→T(07-14) 三笔。
func buildBi(fractals []Fractal, klines []KlineBar) []Bi {
	if len(fractals) < 2 {
		return nil
	}

	var endpoints []Fractal
	var candA, candB *Fractal // 仅在 endpoints 为空时使用
	var pending *Fractal      // 锁定后使用
	pendingIdx := -1          // pending 在 fractals 切片里的位置，case 2 回退用

	// Case 3 兜底：Case 2 触发时记录原触发 fx 的下标 + rewind 后的 endpoints 长度。
	// 主循环扫到 rewindTriggerIdx 时，若 endpoints 长度未增长（重扫期间没成新笔），
	// 就强制把触发 fx append 为端点。
	rewindTriggerIdx := -1
	rewindBaseLen := -1

	clearPending := func() {
		pending = nil
		pendingIdx = -1
	}
	setPending := func(fx Fractal, idx int) {
		tmp := fx
		pending = &tmp
		pendingIdx = idx
	}

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

		// Case 3 兜底：Case 2 重扫到达原触发 fx 时，若 endpoints 没增长，
		// 用放宽规则集判定 last → fx：规则 3 必须满足，规则 1 必须满足
		// （或大幅跳空缺口豁免），规则 2 不要求。
		if rewindTriggerIdx >= 0 && i == rewindTriggerIdx {
			if len(endpoints) == rewindBaseLen && len(endpoints) >= 1 {
				last := endpoints[len(endpoints)-1]
				if fx.Type != last.Type {
					var top, bottom Fractal
					if last.Type == "top" {
						top, bottom = last, fx
					} else {
						top, bottom = fx, last
					}
					rule3 := top.KHigh > bottom.KHigh
					rule1 := absInt(last.Index-fx.Index) >= 3
					gapExempt := hasLargeGapBetween(last, fx, klines)
					if rule3 && (rule1 || gapExempt) {
						endpoints = append(endpoints, fx)
						clearPending()
						rewindTriggerIdx = -1
						rewindBaseLen = -1
						continue
					}
				}
			}
			rewindTriggerIdx = -1
			rewindBaseLen = -1
		}

		switch {
		case len(endpoints) == 0:
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
					if candB != nil && biRulesSatisfied(*candB, *candA, klines) {
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
			if biRulesSatisfied(*candA, *candB, klines) {
				confirmFirstPair(*candA, *candB)
			}
			continue

		case len(endpoints) == 1:
			// case 2 修正可能把 endpoints 砍到只剩 1 个，这里走"准阶段二"逻辑
			last := &endpoints[0]
			if fx.Type == last.Type {
				if moreExtreme(fx, *last) {
					*last = fx
					clearPending()
				}
				continue
			}
			if pending == nil || moreExtreme(fx, *pending) {
				setPending(fx, i)
			}
			if biRulesSatisfied(*last, *pending, klines) {
				endpoints = append(endpoints, *pending)
				clearPending()
			}
			continue
		}

		// === 阶段二：已锁定至少 2 个端点 ===
		last := &endpoints[len(endpoints)-1]
		prev := &endpoints[len(endpoints)-2]

		if fx.Type == last.Type {
			if !moreExtreme(fx, *last) {
				continue // fx 不更极端，忽略
			}
			// fx 比 last 更极端 → 触发笔修正
			if pending != nil && moreExtreme(*pending, *prev) {
				// Case 2：pending 比 prev 更极端。先用 pending 顶替 prev、
				// 丢弃旧 last，再从 pending 之后的所有分型重新检查一遍是否
				// 成笔（用 i = pendingIdx，下一轮 i++ 自然跳到 pending 后第一根）。
				// 同时记录 rewindTriggerIdx = 当前 i（触发 fx 在 fractals 中的位置）
				// 与 rewindBaseLen = rewind 后 endpoints 长度，给 Case 3 用。
				savedIdx := pendingIdx
				rewindTriggerIdx = i
				rewindBaseLen = len(endpoints) - 1
				*prev = *pending
				endpoints = endpoints[:len(endpoints)-1]
				clearPending()
				i = savedIdx
				continue
			}
			// Case 1：仅延伸 last（pending 不构成对 prev 的威胁）
			*last = fx
			clearPending()
			continue
		}

		// 反向类型：维护 pending 为最极端，规则通过即追加为端点
		if pending == nil || moreExtreme(fx, *pending) {
			setPending(fx, i)
		}
		if biRulesSatisfied(*last, *pending, klines) {
			endpoints = append(endpoints, *pending)
			clearPending()
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
	bis := buildBi(fractals, klines)
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
