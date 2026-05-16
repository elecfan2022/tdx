package main

import (
	"fmt"
	"strings"
	"time"
)

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

// ============================================================
//  线段 (Segment)
// ============================================================
//
// 算法概览（向上线段为例）：
//
//  1. 笔序列 = bis 转 SeqElem 列表，每根笔 → 1 个元素，带 Direction (up/down)
//  2. 主扫描从 segStart+1 起，遍历笔序列：
//     - 反向笔（向下笔）进 第一CS，用"前包后"包含合并
//     - 同向笔（向上笔）当前留作占位（Q5 待定）
//     - 第一CS 出 顶分型 [a, b, c] 后：
//       · 判 第一种 / 第二种情况（看 a-b 是否有缺口）
//       · 第一种情况：判 subcase 1a / 1b（看破坏笔后第 3 笔是否在破坏笔范围内）
//         - 1a：双 CS 验证 + 破点兜底
//         - 1b：直接终止
//       · 第二种情况：第二CS 单层验证
//
//  3. 终止后形成 Segment：可能附带 另一转折点（subcase 1a 的 CS-B 分型 / 不适用其他场景）

// SeqElem 笔序列 / 特征序列元素（结构共用）
type SeqElem struct {
	Direction     string  // "up" / "down"
	High          float64 // 上下端价格中较高者
	Low           float64 // 较低者
	FromPrice     float64 // 起点价
	ToPrice       float64 // 终点价
	FromTimestamp int64
	ToTimestamp   int64
	BiStartIdx    int // 在 bis 切片中起始笔下标
	BiEndIdx      int // 末笔下标（合并后可能 > BiStartIdx）
}

// biToSeqElem 笔 → SeqElem
func biToSeqElem(b Bi, idx int) SeqElem {
	dir := "up"
	if b.From.Type == "top" {
		dir = "down"
	}
	high, low := b.From.Price, b.To.Price
	if low > high {
		high, low = low, high
	}
	return SeqElem{
		Direction:     dir,
		High:          high,
		Low:           low,
		FromPrice:     b.From.Price,
		ToPrice:       b.To.Price,
		FromTimestamp: b.From.Timestamp,
		ToTimestamp:   b.To.Timestamp,
		BiStartIdx:    idx,
		BiEndIdx:      idx,
	}
}

// buildBiSeq 把笔列表转笔序列
func buildBiSeq(bis []Bi) []SeqElem {
	out := make([]SeqElem, len(bis))
	for i, b := range bis {
		out[i] = biToSeqElem(b, i)
	}
	return out
}

// seqContained 判断两元素是否互为包含
//
//	返回 (frontContainsBack, backContainsFront)
func seqContained(a, b SeqElem) (bool, bool) {
	fcb := a.High >= b.High && a.Low <= b.Low
	bcf := b.High >= a.High && b.Low <= a.Low
	return fcb, bcf
}

// seqMergeReverse 反向 CS 元素合并（与线段方向相反的 CS）
//
//	向上线段 CS（下行笔）取高高；向下线段 CS（上行笔）取低低
//	起点/终点价沿合并后的 high/low 设定（保持方向）
func seqMergeReverse(a, b SeqElem, segDirection string) SeqElem {
	out := SeqElem{
		Direction:  a.Direction,
		BiStartIdx: a.BiStartIdx,
		BiEndIdx:   b.BiEndIdx,
	}
	if segDirection == "up" {
		// CS 由下行笔组成，向下方向 → 高高（取较高）
		out.High = maxF(a.High, b.High)
		out.Low = maxF(a.Low, b.Low)
	} else {
		out.High = minF(a.High, b.High)
		out.Low = minF(a.Low, b.Low)
	}
	// 反向笔（CS 元素）：起点 = high, 终点 = low（向下笔）
	// 向下线段则相反
	if out.Direction == "down" {
		out.FromPrice = out.High
		out.ToPrice = out.Low
	} else {
		out.FromPrice = out.Low
		out.ToPrice = out.High
	}
	out.FromTimestamp = a.FromTimestamp
	out.ToTimestamp = b.ToTimestamp
	return out
}

// seqMergeForward 同向 CS 元素合并（与线段方向相同）
//
//	向上线段 CS-B（上行笔）取低低；向下线段 CS-B（下行笔）取高高
//	（与 reverse 方向相反的合并规则）
func seqMergeForward(a, b SeqElem, segDirection string) SeqElem {
	out := SeqElem{
		Direction:  a.Direction,
		BiStartIdx: a.BiStartIdx,
		BiEndIdx:   b.BiEndIdx,
	}
	if segDirection == "up" {
		out.High = minF(a.High, b.High)
		out.Low = minF(a.Low, b.Low)
	} else {
		out.High = maxF(a.High, b.High)
		out.Low = maxF(a.Low, b.Low)
	}
	if out.Direction == "up" {
		out.FromPrice = out.Low
		out.ToPrice = out.High
	} else {
		out.FromPrice = out.High
		out.ToPrice = out.Low
	}
	out.FromTimestamp = a.FromTimestamp
	out.ToTimestamp = b.ToTimestamp
	return out
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// seqIsTop 顶分型：b.high > a.high && b.high > c.high && b.low > c.low
func seqIsTop(a, b, c SeqElem) bool {
	return b.High > a.High && b.High > c.High && b.Low > c.Low
}

// seqIsBottom 底分型：b.low < a.low && b.low < c.low && b.high < c.high
func seqIsBottom(a, b, c SeqElem) bool {
	return b.Low < a.Low && b.Low < c.Low && b.High < c.High
}

// seqHasGap 判 a-b 间是否有缺口
//
//	顶分型：a.high < b.low → 有缺口（b 区间完全在 a 上方）
//	底分型：a.low  > b.high → 有缺口
func seqHasGap(a, b SeqElem, fractalType string) bool {
	if fractalType == "top" {
		return a.High < b.Low
	}
	return a.Low > b.High
}

// Segment 线段
//
//	From/To = 主端点（起点和终点）
//	AnotherTransition = subcase 1a 的 CS-B 衍生的另一转折点（可空）
//	TerminationCase = 1（第一种）/ 2（第二种）/ 0（未终止）
//	Subcase = 1（1a）/ 2（1b）/ 0（不适用）
type Segment struct {
	From              Fractal  `json:"from"`
	To                Fractal  `json:"to"`
	Direction         string   `json:"direction"`
	AnotherTransition *Fractal `json:"anotherTransition,omitempty"`
	TerminationCase   int      `json:"terminationCase"`
	Subcase           int      `json:"subcase"`
	TriggerBiIdx      int      `json:"triggerBiIdx"` // 仅 subcase 1a 触发段时设置 (>0)；下一段 CS 扫描从此值-1 起
}

// segmentEndResult 内部封装 findSegmentEnd 的返回
type segmentEndResult struct {
	confirmed              bool
	endBiIdx               int      // 段终止笔下标（在 biSeq 中）
	transition             Fractal  // 主转折点
	anotherTransition      *Fractal // 另一转折点（可空）
	termCase               int
	subcase                int
	triggerBiIdx           int // 仅 subcase 1a CS-A fractal 触发时设置（>0）；下一段 CS 扫描从此 -1 起
	anotherTransitionBiIdx int // 仅 subcase 1a CS-B fractal 触发时设置（>0）；派生中间段后下一段 segStart 用此值
}

// buildSegments 从笔列表构建线段列表
//
//	subcase 1a 触发路径分支：
//	  CS-A fractal: 设 triggerBiIdx → 下一段 scanFromBi = trigger - 1
//	  CS-B fractal: 设 anotherTransitionBiIdx → 派生中间段（方向相反，From=主转折点，
//	                To=另一转折点），下一段 segStart = anotherTransitionBiIdx，不回退
//	  break_end:    都不设 → 下一段自然扫描 segStart + 1
func buildSegments(bis []Bi) []Segment {
	if len(bis) < 3 {
		return nil
	}
	biSeq := buildBiSeq(bis)
	var segments []Segment
	segStart := 0
	prevTriggerBiIdx := 0 // 上一段 CS-A fractal trigger 下标；0 表示不回退

	for segStart < len(biSeq)-2 {
		direction := biSeq[segStart].Direction

		scanFromBi := segStart + 1
		if prevTriggerBiIdx > 0 && prevTriggerBiIdx-1 > scanFromBi {
			scanFromBi = prevTriggerBiIdx - 1
		}

		result := findSegmentEnd(biSeq, segStart, scanFromBi, direction)
		if !result.confirmed {
			break
		}
		if result.endBiIdx <= segStart {
			break // 防御
		}
		seg := Segment{
			From: Fractal{
				Type:      directionFromType(direction, true),
				Timestamp: biSeq[segStart].FromTimestamp,
				Price:     biSeq[segStart].FromPrice,
			},
			To:                result.transition,
			Direction:         direction,
			AnotherTransition: result.anotherTransition,
			TerminationCase:   result.termCase,
			Subcase:           result.subcase,
			TriggerBiIdx:      result.triggerBiIdx,
		}
		segments = append(segments, seg)

		// CS-B fractal 触发：派生中间段
		if result.anotherTransitionBiIdx > 0 && result.anotherTransition != nil {
			oppositeDir := "down"
			if direction == "down" {
				oppositeDir = "up"
			}
			midSeg := Segment{
				From:            result.transition,
				To:              *result.anotherTransition,
				Direction:       oppositeDir,
				TerminationCase: 3, // 3 = CS-B 派生中间段
				Subcase:         0,
				TriggerBiIdx:    0,
			}
			segments = append(segments, midSeg)
			segStart = result.anotherTransitionBiIdx
			prevTriggerBiIdx = 0 // 中间段后不触发回退
			continue
		}

		segStart = result.endBiIdx + 1
		prevTriggerBiIdx = result.triggerBiIdx
	}
	return segments
}

// directionFromType 从段方向得出起点分型类型
//
//	向上段起点 = bottom（底）；向下段起点 = top（顶）
func directionFromType(direction string, isStart bool) string {
	if direction == "up" {
		if isStart {
			return "bottom"
		}
		return "top"
	}
	if isStart {
		return "top"
	}
	return "bottom"
}

// findSegmentEnd 主扫描：找段终止点（新算法）
//
//	主扫描始终用 前包后 合并 反向笔 进 csA。每加入一个反向笔后，若 csA 长度
//	≥ 2，立即取 csA[-2]=a, csA[-1]=b 做"破坏笔识别检查"：
//
//	  • a.high >= b.low (无缺口) && b.high > a.high (b 结构上高于 a)
//	      → 潜在 第一种情况 → 立即调 handleCase1
//	  • a.high <  b.low (有缺口) && b.high > a.high
//	      → 潜在 第二种情况 → 立即调 handleCase2
//	  • b.high <= a.high (b 不高于 a)
//	      → 第三种情况，不调用任何 handle，继续主扫描下一根反向笔
//
//	若 handleCase1/2 返回 not confirmed，段未终止，主扫描继续，等下一根
//	反向笔进入 csA 后再做识别（滑动窗口）。
//	向下段对称（无缺口 = a.low <= b.high；b 结构上低于 a = b.low < a.low）。
//
//	scanFromBi: 主扫描起始的笔下标。一般 = segStart + 1，但若上一段是
//	subcase 1a 终止，则下一段从 触发笔下标-1 开始（由 buildSegments 计算后传入）。
func findSegmentEnd(biSeq []SeqElem, segStart int, scanFromBi int, direction string) segmentEndResult {
	var csA []SeqElem // 第一CS，反向笔，前包后

	for j := scanFromBi; j < len(biSeq); j++ {
		elem := biSeq[j]

		if elem.Direction == direction {
			// 同向笔：Q5 占位，暂不动作
			continue
		}

		// 反向笔：加入 第一CS 并应用前包后包含
		csA = addToCSFrontContains(csA, elem, direction)

		if len(csA) < 2 {
			continue
		}
		a := csA[len(csA)-2]
		b := csA[len(csA)-1]

		var noGap, bHigher bool
		if direction == "up" {
			noGap = a.High >= b.Low
			bHigher = b.High > a.High
		} else {
			noGap = a.Low <= b.High
			bHigher = b.Low < a.Low
		}

		if !bHigher {
			// 第三种情况：b 不符合结构条件，继续扫描
			continue
		}

		if noGap {
			// 潜在 第一种情况
			result := handleCase1(biSeq, b.BiStartIdx, b, direction, a)
			if result.confirmed {
				return result
			}
		} else {
			// 潜在 第二种情况
			result := handleCase2(biSeq, b.BiStartIdx, b, direction, a)
			if result.confirmed {
				return result
			}
		}
		// 不 confirmed → 段延续，继续主扫描
	}

	return segmentEndResult{confirmed: false}
}

// addToCSFrontContains 把 elem 加入 cs，应用"前包后"包含合并
//
//	若上一元素 a 完全包含新元素 b（a.high >= b.high && a.low <= b.low），合并
//	否则追加
func addToCSFrontContains(cs []SeqElem, elem SeqElem, segDirection string) []SeqElem {
	if len(cs) == 0 {
		return append(cs, elem)
	}
	last := cs[len(cs)-1]
	fcb, _ := seqContained(last, elem)
	if fcb {
		cs[len(cs)-1] = seqMergeReverse(last, elem, segDirection)
		return cs
	}
	return append(cs, elem)
}

// filterCSBInRange 从 csB 中剔除 BiStartIdx 严格落在 (lower, upper) 之间的元素。
//
//	subcase 1a 双 CS 验证里，当 CS-A 发生 前包后 合并（合并区间 oldEnd → newEnd），
//	区间内夹的同向笔在 缠论 包含处理 意义上被吸收，不应再作为 CS-B 的独立元素。
//	入参 lower = 合并前 csA[-1].BiEndIdx；upper = 合并后 csA[-1].BiEndIdx（= elem.BiEndIdx）
func filterCSBInRange(csB []SeqElem, lower, upper int) []SeqElem {
	if len(csB) == 0 || upper-lower <= 1 {
		return csB
	}
	out := csB[:0]
	for _, e := range csB {
		if e.BiStartIdx <= lower || e.BiStartIdx >= upper {
			out = append(out, e)
		}
	}
	return out
}

// addToCSBoth 把 elem 加入 cs，应用"前后都可以"包含合并
func addToCSBoth(cs []SeqElem, elem SeqElem, segDirection string, isReverse bool) []SeqElem {
	if len(cs) == 0 {
		return append(cs, elem)
	}
	last := cs[len(cs)-1]
	fcb, bcf := seqContained(last, elem)
	if fcb || bcf {
		if isReverse {
			cs[len(cs)-1] = seqMergeReverse(last, elem, segDirection)
		} else {
			cs[len(cs)-1] = seqMergeForward(last, elem, segDirection)
		}
		return cs
	}
	return append(cs, elem)
}

// fractalPointPrice 取分型 b 那一笔的极值点价格
func fractalPointPrice(b SeqElem, fractalType string) float64 {
	if fractalType == "top" {
		return b.High
	}
	return b.Low
}

// makeFractalAt 用 b 那一笔的极值点构造一个 Fractal
func makeFractalAt(b SeqElem, fractalType string) Fractal {
	var ts int64
	var price float64
	if fractalType == "top" {
		price = b.High
		// 向下笔（CS 元素）的 high = From，时间戳取 FromTimestamp
		if b.Direction == "down" {
			ts = b.FromTimestamp
		} else {
			ts = b.ToTimestamp
		}
	} else {
		price = b.Low
		if b.Direction == "down" {
			ts = b.ToTimestamp
		} else {
			ts = b.FromTimestamp
		}
	}
	return Fractal{
		Type:      fractalType,
		Timestamp: ts,
		Price:     price,
	}
}

// handleCase1 第一种情况处理（顶/底分型，无缺口）
//
//	新算法：在 csA = [..., a, b] 识别 潜在第一种 后被立即调用（顶分型 还未确定形成）。
//	  subcase 1a (3rd笔 ⊂ breaking笔) → dualCSConfirm 双 CS 验证
//	  subcase 1b (3rd笔 ⊄ breaking笔) → **验证 顶分型 [a, b, c] 是否真的成立**
//	    成立 → 终止；不成立 → not confirmed（主扫描继续）
//	a 参数：csA 中 b 前面的元素，用于 顶分型 验证
func handleCase1(biSeq []SeqElem, breakingIdx int, b SeqElem, direction string, a SeqElem) segmentEndResult {
	fractalType := "top"
	if direction == "down" {
		fractalType = "bottom"
	}

	// thirdBi 取 b 之后第一根反向笔（b.BiEndIdx+2，跳 1 根同向笔）。
	// 当 b 是合并元素时，breakingIdx+2 仍落在 b 内部组成笔上，是错位。
	if b.BiEndIdx+2 >= len(biSeq) {
		// 数据不足，无法判定 subcase 也无法验证 顶分型 → not confirmed
		return segmentEndResult{confirmed: false}
	}

	breakingBi := b // 用合并后的 b（不是 biSeq[breakingIdx] = b 的首根子笔）
	thirdBi := biSeq[b.BiEndIdx+2]
	// 3rd 笔 ⊂ breaking笔 ?
	fcb, _ := seqContained(breakingBi, thirdBi)
	if !fcb {
		// Subcase 1b：验证 顶分型 [a, b, c] 是否成立
		// c 就是 thirdBi（视作单根 SeqElem 比较）
		c := thirdBi
		var fractalOK bool
		if direction == "up" {
			// 顶分型：b.high > a.high && b.high > c.high && b.low > c.low
			// b.high > a.high 已在 findSegmentEnd 识别阶段保证
			fractalOK = b.High > c.High && b.Low > c.Low
		} else {
			// 底分型：b.low < a.low && b.low < c.low && b.high < c.high
			fractalOK = b.Low < c.Low && b.High < c.High
		}
		if !fractalOK {
			// 顶/底分型 实际不成立 → not confirmed
			return segmentEndResult{confirmed: false}
		}
		return segmentEndResult{
			confirmed:  true,
			endBiIdx:   breakingIdx - 1,
			transition: makeFractalAt(b, fractalType),
			termCase:   1,
			subcase:    2,
		}
	}
	// Subcase 1a：双 CS 验证
	return subcase1aDualCS(biSeq, breakingIdx, b, direction)
}

// subcase1aDualCS Subcase 1a 双 CS 验证
//
//	CS-A 起点 = 3rd 笔（biSeq[b.BiEndIdx+2]，= b 之后第一根反向笔）, 前包后, 找段方向相反分型
//	CS-B 起点 = 3rd 笔之后的第一根同向笔, 不做包含, 找段方向相反对应分型
//	破点：破 breaking.结束点（向上段：跌破 breaking.To.Price）→ 终止
//	      破 breaking.开始点（向上段：突破 breaking.From.Price）→ 段延续
//
//	注意：用合并后的 originalB 作 breakingBi，并以 originalB.BiEndIdx+2 起 CS-A，
//	防止 b 是合并元素时取到 b 内部子笔。
func subcase1aDualCS(biSeq []SeqElem, breakingIdx int, originalB SeqElem, direction string) segmentEndResult {
	breakingBi := originalB
	breakingHigh := breakingBi.High
	breakingLow := breakingBi.Low
	breakingStart := breakingBi.FromPrice // 破坏笔起始点
	// breakingEnd := breakingBi.ToPrice
	_ = breakingStart

	mainTransition := makeFractalAt(originalB, ternaryString(direction == "up", "top", "bottom"))

	csA := []SeqElem{biSeq[originalB.BiEndIdx+2]}
	var csB []SeqElem

	for j := originalB.BiEndIdx + 3; j < len(biSeq); j++ {
		elem := biSeq[j]

		// 第一步：CS 更新 + 分型检查（分型优先于破点）
		if elem.Direction != direction {
			// 反向笔 → CS-A 前包后
			oldLen := len(csA)
			oldEnd := csA[oldLen-1].BiEndIdx // 合并前的 BiEndIdx（用于检测合并范围）
			csA = addToCSFrontContains(csA, elem, direction)
			if len(csA) == oldLen {
				// 发生合并：剔除 CS-B 中落在合并区间内部的元素
				csB = filterCSBInRange(csB, oldEnd, elem.BiEndIdx)
			}
			if len(csA) >= 3 {
				a, bm, c := csA[len(csA)-3], csA[len(csA)-2], csA[len(csA)-1]
				if (direction == "up" && seqIsTop(a, bm, c)) ||
					(direction == "down" && seqIsBottom(a, bm, c)) {
					return segmentEndResult{
						confirmed:    true,
						endBiIdx:     breakingIdx - 1,
						transition:   mainTransition,
						termCase:     1,
						subcase:      1,
						triggerBiIdx: j,
					}
				}
			}
		} else {
			// 同向笔 → CS-B 不做包含
			csB = append(csB, elem)
			if len(csB) >= 3 {
				a, bm, c := csB[len(csB)-3], csB[len(csB)-2], csB[len(csB)-1]
				// 找段方向相反的分型（向上段找底，向下段找顶）
				opposite := ternaryString(direction == "up", "bottom", "top")
				ok := false
				if opposite == "bottom" {
					ok = seqIsBottom(a, bm, c)
				} else {
					ok = seqIsTop(a, bm, c)
				}
				if ok {
					anotherFx := makeFractalAt(bm, opposite)
					return segmentEndResult{
						confirmed:              true,
						endBiIdx:               breakingIdx - 1,
						transition:             mainTransition,
						anotherTransition:      &anotherFx,
						termCase:               1,
						subcase:                1,
						anotherTransitionBiIdx: bm.BiStartIdx, // CS-B 不做包含，bm 是单根笔；buildSegments 据此派生中间段并把下一段 segStart 设到这里
					}
				}
			}
		}

		// 第二步：破点检查（仅在没出现分型时作为兜底）
		if direction == "up" {
			if elem.High > breakingHigh {
				return segmentEndResult{confirmed: false} // 破开始点 → 段延续
			}
			if elem.Low < breakingLow {
				// 破结束点 → 终止（不设 triggerBiIdx：break_end 无 trio 结构，下一段自然扫描）
				return segmentEndResult{
					confirmed:  true,
					endBiIdx:   breakingIdx - 1,
					transition: mainTransition,
					termCase:   1,
					subcase:    1,
				}
			}
		} else {
			if elem.Low < breakingLow {
				return segmentEndResult{confirmed: false}
			}
			if elem.High > breakingHigh {
				return segmentEndResult{
					confirmed:  true,
					endBiIdx:   breakingIdx - 1,
					transition: mainTransition,
					termCase:   1,
					subcase:    1,
				}
			}
		}
	}

	return segmentEndResult{confirmed: false}
}

// handleCase2 第二种情况处理（顶/底分型，有缺口）
//
//	入口验证：先验证 第一CS 三元素 [a, b, c=biSeq[b.BiEndIdx+2]] 是否构成
//	顶分型/底分型（与 subcase 1b 同款验证）。未构成 → not confirmed，主扫描继续。
//	构成后才启动 第二CS。
//
//	第二CS 起点 = b 之后第一根同向笔，前后都可以包含，找段方向相反分型
//	（不区分该分型的 第一/第二种情况，出现即终止）
//	循环顺序：cs2 更新 + 分型检查 在前；破点检查 在后（分型优先于破点）。
//	单根笔 high 超过 b.high（向上段）/ low 跌破 b.low（向下段）→ 段延续。
func handleCase2(biSeq []SeqElem, breakingIdx int, b SeqElem, direction string, a SeqElem) segmentEndResult {
	fractalType := ternaryString(direction == "up", "top", "bottom")
	mainTransition := makeFractalAt(b, fractalType)
	opposite := ternaryString(direction == "up", "bottom", "top")

	// 入口验证：第一CS 三元素 顶/底分型
	//
	// 注意：c 必须取 b 之后的下一根反向笔（= b.BiEndIdx+2，跳过 1 根同向笔），
	// 而不是 biSeq[breakingIdx+2]。当 b 是合并元素时 breakingIdx+2 仍落在 b 内
	// 部，会把 b 的组成笔当成 c。
	_ = breakingIdx
	if b.BiEndIdx+2 >= len(biSeq) {
		return segmentEndResult{confirmed: false}
	}
	c1 := biSeq[b.BiEndIdx+2]
	var firstCSFractalOK bool
	if direction == "up" {
		firstCSFractalOK = seqIsTop(a, b, c1)
	} else {
		firstCSFractalOK = seqIsBottom(a, b, c1)
	}
	if !firstCSFractalOK {
		return segmentEndResult{confirmed: false}
	}

	var cs2 []SeqElem

	for j := b.BiEndIdx + 1; j < len(biSeq); j++ {
		elem := biSeq[j]

		// 第一步：cs2 更新 + 分型检查（分型优先于破点）
		if elem.Direction == direction {
			cs2 = addToCSBoth(cs2, elem, direction, false)
			if len(cs2) >= 3 {
				a2, bm, c2 := cs2[len(cs2)-3], cs2[len(cs2)-2], cs2[len(cs2)-1]
				ok := false
				if opposite == "bottom" {
					ok = seqIsBottom(a2, bm, c2)
				} else {
					ok = seqIsTop(a2, bm, c2)
				}
				if ok {
					return segmentEndResult{
						confirmed:  true,
						endBiIdx:   b.BiStartIdx - 1,
						transition: mainTransition,
						termCase:   2,
						subcase:    0,
					}
				}
			}
		}

		// 第二步：破点检查（仅在没出现分型时作为兜底）
		if direction == "up" && elem.High > b.High {
			return segmentEndResult{confirmed: false}
		}
		if direction == "down" && elem.Low < b.Low {
			return segmentEndResult{confirmed: false}
		}
	}

	return segmentEndResult{confirmed: false}
}

// ternaryString 模拟三目运算符
func ternaryString(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// ============================================================
//  线段诊断
// ============================================================

// CSAItem 线段诊断中输出的 CS 元素（合并后）
type CSAItem struct {
	High       float64 `json:"high"`
	Low        float64 `json:"low"`
	FromTs     int64   `json:"fromTs"`
	ToTs       int64   `json:"toTs"`
	BiStartIdx int     `json:"biStartIdx"`
	BiEndIdx   int     `json:"biEndIdx"`
}

// DualCSDiag subcase 1a 双 CS 诊断
//
//	Trigger 枚举：
//	  "csA_fractal"  - CS-A 出现段方向相反分型，段终止
//	  "csB_fractal"  - CS-B 出现 opposite 分型，段终止 + 另一转折点
//	  "break_end"    - 破破坏笔结束点，段终止
//	  "break_start"  - 破破坏笔开始点，段延续 (return not confirmed)
//	  "exhausted"    - 数据扫完无任何信号
type DualCSDiag struct {
	CSA               []CSAItem `json:"csA"`
	CSB               []CSAItem `json:"csB"`
	Trigger           string    `json:"trigger"`
	TriggerBiIdx      int       `json:"triggerBiIdx"`
	TriggerFractalIdx [3]int    `json:"triggerFractalIdx,omitempty"`
	AnotherTransition *Fractal  `json:"anotherTransition,omitempty"`
	BreakingHigh      float64   `json:"breakingHigh"`
	BreakingLow       float64   `json:"breakingLow"`
}

// SegmentDiag 段诊断结果
type SegmentDiag struct {
	Found             bool        `json:"found"`
	Note              string      `json:"note"`
	SegFrom           *Fractal    `json:"segFrom,omitempty"`
	SegTo             *Fractal    `json:"segTo,omitempty"`
	Direction         string      `json:"direction,omitempty"`
	TerminationCase   int         `json:"terminationCase"`
	Subcase           int         `json:"subcase"`
	CSA               []CSAItem   `json:"csA,omitempty"`        // 终止时 CS-A 的所有元素（前包后合并后）
	FractalIdx        [3]int      `json:"fractalIdx,omitempty"` // 顶/底分型 a, b, c 在 CSA 中的下标
	HasGap            bool        `json:"hasGap"`
	GapDescription    string      `json:"gapDescription,omitempty"`
	AnotherTransition *Fractal    `json:"anotherTransition,omitempty"`
	DualCS            *DualCSDiag `json:"dualCS,omitempty"` // 仅 subcase 1a 时填充
}

// csAToItems 把 csA SeqElem 列表转 CSAItem 列表（诊断输出用）
func csAToItems(cs []SeqElem) []CSAItem {
	out := make([]CSAItem, len(cs))
	for i, e := range cs {
		out[i] = CSAItem{
			High:       e.High,
			Low:        e.Low,
			FromTs:     e.FromTimestamp,
			ToTs:       e.ToTimestamp,
			BiStartIdx: e.BiStartIdx,
			BiEndIdx:   e.BiEndIdx,
		}
	}
	return out
}

// traceSubcase1aDualCS 复刻 subcase1aDualCS 的逻辑但记录完整 trace
//
//	镜像生产代码的判定顺序（CS 更新+分型检查 在前，破点检查 在后）。
//	无论哪种 Trigger 都会填充 CSA / CSB 的最终状态。
//	b 是合并后的破坏笔（与 subcase1aDualCS 的 originalB 一致），CS-A 起点
//	和循环起点都用 b.BiEndIdx，避免 b 是合并元素时取到 b 内部子笔。
func traceSubcase1aDualCS(biSeq []SeqElem, b SeqElem, direction string) *DualCSDiag {
	if b.BiEndIdx+2 >= len(biSeq) {
		return nil
	}
	diag := &DualCSDiag{
		Trigger:           "exhausted",
		TriggerBiIdx:      -1,
		TriggerFractalIdx: [3]int{-1, -1, -1},
		BreakingHigh:      b.High,
		BreakingLow:       b.Low,
	}

	csA := []SeqElem{biSeq[b.BiEndIdx+2]}
	var csB []SeqElem

	for j := b.BiEndIdx + 3; j < len(biSeq); j++ {
		elem := biSeq[j]

		// 第一步：CS 更新 + 分型检查
		if elem.Direction != direction {
			oldLen := len(csA)
			oldEnd := csA[oldLen-1].BiEndIdx
			csA = addToCSFrontContains(csA, elem, direction)
			if len(csA) == oldLen {
				csB = filterCSBInRange(csB, oldEnd, elem.BiEndIdx)
			}
			if len(csA) >= 3 {
				a, bm, c := csA[len(csA)-3], csA[len(csA)-2], csA[len(csA)-1]
				if (direction == "up" && seqIsTop(a, bm, c)) ||
					(direction == "down" && seqIsBottom(a, bm, c)) {
					diag.Trigger = "csA_fractal"
					diag.TriggerBiIdx = j
					diag.TriggerFractalIdx = [3]int{len(csA) - 3, len(csA) - 2, len(csA) - 1}
					diag.CSA = csAToItems(csA)
					diag.CSB = csAToItems(csB)
					return diag
				}
			}
		} else {
			csB = append(csB, elem)
			if len(csB) >= 3 {
				a, bm, c := csB[len(csB)-3], csB[len(csB)-2], csB[len(csB)-1]
				opposite := ternaryString(direction == "up", "bottom", "top")
				ok := false
				if opposite == "bottom" {
					ok = seqIsBottom(a, bm, c)
				} else {
					ok = seqIsTop(a, bm, c)
				}
				if ok {
					anotherFx := makeFractalAt(bm, opposite)
					diag.Trigger = "csB_fractal"
					diag.TriggerBiIdx = j
					diag.TriggerFractalIdx = [3]int{len(csB) - 3, len(csB) - 2, len(csB) - 1}
					diag.AnotherTransition = &anotherFx
					diag.CSA = csAToItems(csA)
					diag.CSB = csAToItems(csB)
					return diag
				}
			}
		}

		// 第二步：破点检查
		if direction == "up" {
			if elem.High > diag.BreakingHigh {
				diag.Trigger = "break_start"
				diag.TriggerBiIdx = j
				diag.CSA = csAToItems(csA)
				diag.CSB = csAToItems(csB)
				return diag
			}
			if elem.Low < diag.BreakingLow {
				diag.Trigger = "break_end"
				diag.TriggerBiIdx = j
				diag.CSA = csAToItems(csA)
				diag.CSB = csAToItems(csB)
				return diag
			}
		} else {
			if elem.Low < diag.BreakingLow {
				diag.Trigger = "break_start"
				diag.TriggerBiIdx = j
				diag.CSA = csAToItems(csA)
				diag.CSB = csAToItems(csB)
				return diag
			}
			if elem.High > diag.BreakingHigh {
				diag.Trigger = "break_end"
				diag.TriggerBiIdx = j
				diag.CSA = csAToItems(csA)
				diag.CSB = csAToItems(csB)
				return diag
			}
		}
	}

	diag.CSA = csAToItems(csA)
	diag.CSB = csAToItems(csB)
	return diag
}

// diagnoseSegmentTrace 给定一条 Segment 的位置，重跑 第一CS 扫描，捕获终止时 CS-A 状态
//
//	新算法：识别在 csA size==2 时做 (csA[-2]=a, csA[-1]=b)。终止时 b.BiStartIdx
//	对应原 bis 中的"破坏笔"索引（= endBiIdx + 1）。
//	返回 (csA, [a, b, c=-1]在 csA 中的下标, found)
//	c 在新算法主扫描中并不存在（没有 3 元素顶分型）；保留位置占位为 -1。
//
//	scanFromBi: 与 findSegmentEnd 一致的主扫描起点。
func diagnoseSegmentTrace(biSeq []SeqElem, scanFromBi, endBiIdx int, direction string) ([]SeqElem, [3]int, bool) {
	var csA []SeqElem
	targetBiIdx := endBiIdx + 1 // 破坏笔下标 = b.BiStartIdx

	for j := scanFromBi; j < len(biSeq); j++ {
		elem := biSeq[j]
		if elem.Direction == direction {
			continue
		}
		csA = addToCSFrontContains(csA, elem, direction)
		if len(csA) >= 2 && csA[len(csA)-1].BiStartIdx == targetBiIdx {
			return csA, [3]int{len(csA) - 2, len(csA) - 1, -1}, true
		}
	}
	return csA, [3]int{-1, -1, -1}, false
}

// findSegmentByStartTs 在已构建的 segments 中按起点时间戳前缀匹配找到目标线段，
// 返回索引；找不到返回 -1
func findSegmentByStartTs(segments []Segment, startTsPrefix string, loc *time.Location) int {
	if loc == nil {
		loc, _ = time.LoadLocation("Asia/Shanghai")
	}
	for i, s := range segments {
		ts := s.From.Timestamp
		formatted := time.UnixMilli(ts).In(loc).Format("2006-01-02 15:04:05")
		if strings.HasPrefix(formatted, startTsPrefix) {
			return i
		}
	}
	return -1
}

// DiagnoseSegmentFromBis 从 bis + 段起点日期前缀做段诊断
//
//	startDatePrefix: 形如 "2021-02-05" 或 "2021-02-05 09:30"
func DiagnoseSegmentFromBis(bis []Bi, startDatePrefix string) SegmentDiag {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	segments := buildSegments(bis)
	idx := findSegmentByStartTs(segments, startDatePrefix, loc)
	if idx < 0 {
		// 列出所有线段起点日期供参考
		dates := make([]string, 0, len(segments))
		for _, s := range segments {
			dates = append(dates, time.UnixMilli(s.From.Timestamp).In(loc).Format("2006-01-02"))
		}
		datesStr := "无"
		if len(dates) > 0 {
			datesStr = strings.Join(dates, ", ")
		}
		return SegmentDiag{
			Found: false,
			Note:  fmt.Sprintf("未找到起点匹配 %q 的线段。共 %d 段，起点日期：%s", startDatePrefix, len(segments), datesStr),
		}
	}

	seg := segments[idx]
	biSeq := buildBiSeq(bis)

	// 找 segStart：bis[k].From.Timestamp == seg.From.Timestamp
	segStart := -1
	for k := range bis {
		if bis[k].From.Timestamp == seg.From.Timestamp {
			segStart = k
			break
		}
	}
	// 找 endBiIdx：bis[k].To.Timestamp == seg.To.Timestamp
	endBiIdx := -1
	for k := range bis {
		if bis[k].To.Timestamp == seg.To.Timestamp {
			endBiIdx = k
			break
		}
	}

	diag := SegmentDiag{
		Found:             true,
		SegFrom:           &seg.From,
		SegTo:             &seg.To,
		Direction:         seg.Direction,
		TerminationCase:   seg.TerminationCase,
		Subcase:           seg.Subcase,
		AnotherTransition: seg.AnotherTransition,
	}

	// 派生中间段（来自上一段 CS-B fractal）：无独立 CS-A 扫描，直接返回基本信息
	if seg.TerminationCase == 3 {
		diag.Note = "派生中间段：由上一段 subcase 1a CS-B 顶/底分型 同步确认，无独立 CS 扫描轨迹"
		return diag
	}

	if segStart < 0 || endBiIdx < 0 {
		diag.Note = "段起止笔在 bis 中索引失败"
		return diag
	}

	// 与 buildSegments 一致地计算本段扫描起点（依据上一段 TriggerBiIdx）
	scanFromBi := segStart + 1
	if idx > 0 {
		prevTrigger := segments[idx-1].TriggerBiIdx
		if prevTrigger > 0 && prevTrigger-1 > scanFromBi {
			scanFromBi = prevTrigger - 1
		}
	}

	csA, fxIdx, traced := diagnoseSegmentTrace(biSeq, scanFromBi, endBiIdx, seg.Direction)
	diag.CSA = make([]CSAItem, len(csA))
	for i, e := range csA {
		diag.CSA[i] = CSAItem{
			High:       e.High,
			Low:        e.Low,
			FromTs:     e.FromTimestamp,
			ToTs:       e.ToTimestamp,
			BiStartIdx: e.BiStartIdx,
			BiEndIdx:   e.BiEndIdx,
		}
	}

	if traced {
		diag.FractalIdx = fxIdx
		a, b := csA[fxIdx[0]], csA[fxIdx[1]]
		fractalType := "top"
		if seg.Direction == "down" {
			fractalType = "bottom"
		}
		diag.HasGap = seqHasGap(a, b, fractalType)
		if diag.HasGap {
			if fractalType == "top" {
				diag.GapDescription = fmt.Sprintf("顶分型 第二种情况：a.high=%.2f < b.low=%.2f（缺口 %.2f）", a.High, b.Low, b.Low-a.High)
			} else {
				diag.GapDescription = fmt.Sprintf("底分型 第二种情况：a.low=%.2f > b.high=%.2f（缺口 %.2f）", a.Low, b.High, a.Low-b.High)
			}
		} else {
			if fractalType == "top" {
				diag.GapDescription = fmt.Sprintf("顶分型 第一种情况：a.high=%.2f ≥ b.low=%.2f（区间重叠）", a.High, b.Low)
			} else {
				diag.GapDescription = fmt.Sprintf("底分型 第一种情况：a.low=%.2f ≤ b.high=%.2f（区间重叠）", a.Low, b.High)
			}
		}
	} else {
		diag.Note = "未能复现终止时的 第一CS 顶分型；CSA 输出的是扫描结束时的状态"
	}

	// 若为 subcase 1a，补充双 CS 内部 trace
	//
	// b 取 trace 出的 csA[fxIdx[1]]（合并后的破坏笔元素），与生产代码
	// subcase1aDualCS 拿到的 originalB 一致；不能用 biSeq[breakingIdx]
	// 那是 b 的首根子笔。
	if seg.TerminationCase == 1 && seg.Subcase == 1 && traced && fxIdx[1] >= 0 && fxIdx[1] < len(csA) {
		b := csA[fxIdx[1]]
		diag.DualCS = traceSubcase1aDualCS(biSeq, b, seg.Direction)
	}

	return diag
}

// AnalyzeChan 把缠论分析输出聚合，供 GetKline 一次性返回
type ChanAnalysis struct {
	Fractals []Fractal `json:"fractals"`
	Bis      []Bi      `json:"bis"`
	Segments []Segment `json:"segments"`
}

func AnalyzeChan(klines []KlineBar) ChanAnalysis {
	if len(klines) < 3 {
		return ChanAnalysis{Fractals: []Fractal{}, Bis: []Bi{}, Segments: []Segment{}}
	}
	processed := processContainment(klines)
	fractals := findFractals(processed, klines)
	bis := buildBi(fractals, klines)
	segments := buildSegments(bis)
	if fractals == nil {
		fractals = []Fractal{}
	}
	if bis == nil {
		bis = []Bi{}
	}
	if segments == nil {
		segments = []Segment{}
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
	return ChanAnalysis{Fractals: fractals, Bis: bis, Segments: segments}
}
