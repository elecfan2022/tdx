<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch, computed } from 'vue'
import { init, dispose, registerOverlay, LineType, type Chart, type KLineData } from 'klinecharts'

// 自定义镜像版 simpleAnnotation —— 画在数据点下方，箭头朝上指向 K 线最低点
// 给"底分型"用
registerOverlay({
  name: 'simpleAnnotationDown',
  totalStep: 2,
  styles: {
    line: { style: 'dashed' as any },
  },
  createPointFigures: (params: any) => {
    const { overlay, coordinates } = params
    const text = overlay.extendData ?? ''
    const startX = coordinates[0].x
    const startY = coordinates[0].y + 6   // 数据点下方
    const lineEndY = startY + 50          // 再往下
    const arrowEndY = lineEndY + 5        // 再往下，三角底边
    return [
      {
        type: 'line',
        attrs: { coordinates: [{ x: startX, y: startY }, { x: startX, y: lineEndY }] },
        ignoreEvent: true,
      },
      {
        // 三角形：顶点在 lineEndY（更靠上），底边在 arrowEndY（更靠下）→ 箭头朝上
        type: 'polygon',
        attrs: {
          coordinates: [
            { x: startX, y: lineEndY },
            { x: startX - 4, y: arrowEndY },
            { x: startX + 4, y: arrowEndY },
          ],
        },
        ignoreEvent: true,
      },
      {
        // 文字放在三角形下方
        type: 'text',
        attrs: {
          x: startX,
          y: arrowEndY,
          text,
          align: 'center',
          baseline: 'top',
        },
        ignoreEvent: true,
      },
    ]
  },
})

interface Fractal {
  type: 'top' | 'bottom'
  index: number
  timestamp: number
  price: number
  peakIdx?: number
  leftIdx?: number
  rightIdx?: number
}
interface Bi {
  from: Fractal
  to: Fractal
}
interface Segment {
  from: Fractal
  to: Fractal
  direction: 'up' | 'down'
  anotherTransition?: Fractal | null
  terminationCase: number
  subcase: number
  anomalous?: boolean
}

const props = defineProps<{
  data: KLineData[]
  period: string
  fractals: Fractal[]
  bis: Bi[]
  segments: Segment[]
  showFractals: boolean
  showBis: boolean
  showSegments: boolean
}>()

const containerRef = ref<HTMLDivElement | null>(null)
let chart: Chart | null = null
let resizeObs: ResizeObserver | null = null

function timezone(period: string) {
  return ['day', 'week', 'month'].includes(period) ? 'Asia/Shanghai' : 'Asia/Shanghai'
}

const TOP_COLOR = '#EF4444'    // 顶分型 红
const BOTTOM_COLOR = '#10B981' // 底分型 绿
const BI_COLOR = '#FBBF24'     // 笔 黄
const SEG_COLOR = '#38BDF8'    // 线段主端点 青蓝
const SEG_ALT_COLOR = '#A855F7' // 线段 另一转折点 紫
const SEG_ANOMALY_COLOR = '#EF4444' // 异常线段（相邻同向笔无重合）红

function drawChan() {
  if (!chart) return
  // 清掉所有旧 overlay；缠论数据或显示模式改变时重新画
  chart.removeOverlay()

  if (props.showFractals) {
    for (const fx of props.fractals) {
      chart.createOverlay({
        // 顶分型用内置上方箭头，底分型用我们注册的下方镜像版
        name: fx.type === 'top' ? 'simpleAnnotation' : 'simpleAnnotationDown',
        points: [{ timestamp: fx.timestamp, value: fx.price }],
        extendData: fx.type === 'top' ? '顶' : '底',
        lock: true,
        styles: {
          text: {
            color: fx.type === 'top' ? TOP_COLOR : BOTTOM_COLOR,
            size: 11,
          },
        },
      })
    }
  }

  if (props.showBis) {
    for (const bi of props.bis) {
      chart.createOverlay({
        name: 'segment',
        points: [
          { timestamp: bi.from.timestamp, value: bi.from.price },
          { timestamp: bi.to.timestamp, value: bi.to.price },
        ],
        lock: true,
        styles: {
          line: { color: BI_COLOR, size: 1 },
          point: { activeColor: BI_COLOR, color: BI_COLOR, borderColor: BI_COLOR },
        },
      })
    }
  }

  if (props.showSegments) {
    for (let i = 0; i < props.segments.length; i++) {
      const seg = props.segments[i]
      // startCase = 上一段的 terminationCase；首段无上一段，默认 1（实线）
      // endCase   = 本段的 terminationCase；0（未终止）默认 1
      const startCase = i > 0 ? props.segments[i - 1].terminationCase || 1 : 1
      const endCase = seg.terminationCase || 1
      const startStyle = startCase === 2 ? LineType.Dashed : LineType.Solid
      const endStyle = endCase === 2 ? LineType.Dashed : LineType.Solid

      // 异常段（相邻同向笔无重合）→ 主线红色，另一转折点也用红色
      const lineColor = seg.anomalous ? SEG_ANOMALY_COLOR : SEG_COLOR
      const altLineColor = seg.anomalous ? SEG_ANOMALY_COLOR : SEG_ALT_COLOR

      // 两端线型相同 → 直接画一根，无需切分
      if (startStyle === endStyle) {
        chart.createOverlay({
          name: 'segment',
          points: [
            { timestamp: seg.from.timestamp, value: seg.from.price },
            { timestamp: seg.to.timestamp, value: seg.to.price },
          ],
          lock: true,
          styles: {
            line: { color: lineColor, size: 3, style: startStyle, dashedValue: [6, 4] },
            point: { activeColor: lineColor, color: lineColor, borderColor: lineColor },
          },
        })
      } else {
        // 线型不同 → 按数据下标算中点（不是按时间戳，避免周末/节假日导致弯折）
        const idxFrom = props.data.findIndex(d => d.timestamp === seg.from.timestamp)
        const idxTo = props.data.findIndex(d => d.timestamp === seg.to.timestamp)
        if (idxFrom < 0 || idxTo < 0 || idxTo === idxFrom) {
          // 退化：找不到下标就画整根，按 startStyle
          chart.createOverlay({
            name: 'segment',
            points: [
              { timestamp: seg.from.timestamp, value: seg.from.price },
              { timestamp: seg.to.timestamp, value: seg.to.price },
            ],
            lock: true,
            styles: {
              line: { color: lineColor, size: 3, style: startStyle, dashedValue: [6, 4] },
              point: { activeColor: lineColor, color: lineColor, borderColor: lineColor },
            },
          })
        } else {
          const idxMid = Math.round((idxFrom + idxTo) / 2)
          const midTs = props.data[idxMid].timestamp
          const ratio = (idxMid - idxFrom) / (idxTo - idxFrom)
          const midPrice = seg.from.price + (seg.to.price - seg.from.price) * ratio

          chart.createOverlay({
            name: 'segment',
            points: [
              { timestamp: seg.from.timestamp, value: seg.from.price },
              { timestamp: midTs, value: midPrice },
            ],
            lock: true,
            styles: {
              line: { color: lineColor, size: 3, style: startStyle, dashedValue: [6, 4] },
              point: { activeColor: lineColor, color: lineColor, borderColor: lineColor },
            },
          })
          chart.createOverlay({
            name: 'segment',
            points: [
              { timestamp: midTs, value: midPrice },
              { timestamp: seg.to.timestamp, value: seg.to.price },
            ],
            lock: true,
            styles: {
              line: { color: lineColor, size: 3, style: endStyle, dashedValue: [6, 4] },
              point: { activeColor: lineColor, color: lineColor, borderColor: lineColor },
            },
          })
        }
      }

      // 另一转折点：从 To 到 anotherTransition，紫色 3px（异常段为红色）
      if (seg.anotherTransition) {
        chart.createOverlay({
          name: 'segment',
          points: [
            { timestamp: seg.to.timestamp, value: seg.to.price },
            { timestamp: seg.anotherTransition.timestamp, value: seg.anotherTransition.price },
          ],
          lock: true,
          styles: {
            line: { color: altLineColor, size: 3 },
            point: { activeColor: altLineColor, color: altLineColor, borderColor: altLineColor },
          },
        })
      }
    }
  }
}

// 给外部用：滚动到指定时间戳
function scrollTo(timestamp: number) {
  if (!chart) return
  // KLineChart v9 提供按时间戳滚动
  ;(chart as any).scrollToTimestamp?.(timestamp, 200)
}
defineExpose({ scrollTo })

// === Hover 提示：以分型峰/谷 K 线为锚点，显示构成分型的左右 K 线 ===
const peakIdxToFractal = computed(() => {
  const m = new Map<number, Fractal>()
  for (const fx of props.fractals) {
    if (fx.peakIdx !== undefined) m.set(fx.peakIdx, fx)
  }
  return m
})

const hoverInfo = ref<{
  show: boolean
  type: 'top' | 'bottom'
  peakDate: string
  leftDate: string
  rightDate: string
  peakHigh: number
  peakLow: number
  leftHigh: number
  leftLow: number
  rightHigh: number
  rightLow: number
}>({
  show: false,
  type: 'top',
  peakDate: '',
  leftDate: '',
  rightDate: '',
  peakHigh: 0,
  peakLow: 0,
  leftHigh: 0,
  leftLow: 0,
  rightHigh: 0,
  rightLow: 0,
})

function fmtDate(ts?: number) {
  if (!ts) return '—'
  const d = new Date(ts)
  const yyyy = d.getFullYear()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  if (['day', 'week', 'month'].includes(props.period)) {
    return `${yyyy}-${mm}-${dd}`
  }
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  return `${mm}-${dd} ${hh}:${mi}`
}

onMounted(() => {
  if (!containerRef.value) return
  chart = init(containerRef.value)
  if (!chart) return
  // A 股惯例：红涨绿跌
  const upColor = '#EF4444'
  const downColor = '#10B981'
  const noChangeColor = '#888888'
  chart.setStyles({
    grid: {
      show: false,
    },
    candle: {
      bar: {
        upColor,
        downColor,
        upBorderColor: upColor,
        downBorderColor: downColor,
        upWickColor: upColor,
        downWickColor: downColor,
      },
    },
    // 默认副图柱状颜色（VOL/MACD 共用）
    indicator: {
      bars: [{ upColor, downColor, noChangeColor }],
    },
  })
  chart.createIndicator('MA', false, { id: 'candle_pane' })
  // 顺序：先 MACD，再 VOL —— MACD 显示在主图下方，VOL 在最底部
  chart.createIndicator('MACD')
  chart.createIndicator('VOL')
  chart.setTimezone(timezone(props.period))
  if (props.data.length) chart.applyNewData(props.data)
  drawChan()

  // 监听十字线变化：当鼠标停留在分型的峰/谷 K 线上时，显示提示
  // 仅当"显示·分型"勾选时启用
  ;(chart as any).subscribeAction?.('onCrosshairChange', (params: any) => {
    if (!props.showFractals) {
      hoverInfo.value.show = false
      return
    }
    const dataIndex: number | undefined = params?.dataIndex
    if (dataIndex === undefined || dataIndex < 0) {
      hoverInfo.value.show = false
      return
    }
    const fx = peakIdxToFractal.value.get(dataIndex)
    if (!fx) {
      hoverInfo.value.show = false
      return
    }
    const data = props.data
    const peak = fx.peakIdx !== undefined ? data[fx.peakIdx] : undefined
    const left = fx.leftIdx !== undefined ? data[fx.leftIdx] : undefined
    const right = fx.rightIdx !== undefined ? data[fx.rightIdx] : undefined
    if (!peak) {
      hoverInfo.value.show = false
      return
    }
    hoverInfo.value = {
      show: true,
      type: fx.type,
      peakDate: fmtDate(peak.timestamp),
      leftDate: fmtDate(left?.timestamp),
      rightDate: fmtDate(right?.timestamp),
      peakHigh: peak.high,
      peakLow: peak.low,
      leftHigh: left?.high ?? 0,
      leftLow: left?.low ?? 0,
      rightHigh: right?.high ?? 0,
      rightLow: right?.low ?? 0,
    }
  })

  // 跟随容器尺寸变化（窗口最大化、侧栏切换等）
  // 用 rAF 推迟到下一帧，避免在浏览器还没完成布局时读到旧尺寸（最大化场景下尤明显）
  resizeObs = new ResizeObserver(() => {
    requestAnimationFrame(() => chart?.resize())
  })
  resizeObs.observe(containerRef.value)
  // 兜底：直接挂到 window resize，最大化触发时同样会响应
  window.addEventListener('resize', onWinResize)
})

function onWinResize() {
  requestAnimationFrame(() => chart?.resize())
}

onBeforeUnmount(() => {
  resizeObs?.disconnect()
  resizeObs = null
  window.removeEventListener('resize', onWinResize)
  if (containerRef.value) dispose(containerRef.value)
  chart = null
})

watch(
  () => props.data,
  (d) => {
    if (chart) chart.applyNewData(d)
  },
)

watch(
  () => props.period,
  (p) => {
    if (chart) chart.setTimezone(timezone(p))
  },
)

// 缠论数据或显示开关变化 → 重新画 overlay
watch(
  () => [
    props.fractals,
    props.bis,
    props.segments,
    props.showFractals,
    props.showBis,
    props.showSegments,
  ],
  () => drawChan(),
  { deep: false },
)

// 关掉"分型"开关时，立刻把 hover 提示框收起
watch(
  () => props.showFractals,
  (v) => {
    if (!v) hoverInfo.value.show = false
  },
)
</script>

<template>
  <div ref="containerRef" class="kchart">
    <div v-if="hoverInfo.show" class="fx-tooltip" :class="hoverInfo.type">
      <div class="title">{{ hoverInfo.type === 'top' ? '顶分型' : '底分型' }}</div>
      <div class="row">
        <span class="lbl">左</span>
        <span class="date">{{ hoverInfo.leftDate }}</span>
        <span class="prc">{{ hoverInfo.leftHigh.toFixed(2) }}/{{ hoverInfo.leftLow.toFixed(2) }}</span>
      </div>
      <div class="row peak">
        <span class="lbl">中</span>
        <span class="date">{{ hoverInfo.peakDate }}</span>
        <span class="prc">{{ hoverInfo.peakHigh.toFixed(2) }}/{{ hoverInfo.peakLow.toFixed(2) }}</span>
      </div>
      <div class="row">
        <span class="lbl">右</span>
        <span class="date">{{ hoverInfo.rightDate }}</span>
        <span class="prc">{{ hoverInfo.rightHigh.toFixed(2) }}/{{ hoverInfo.rightLow.toFixed(2) }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.kchart {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 480px;
}
.fx-tooltip {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 10;
  padding: 6px 8px;
  background: rgba(15, 23, 42, 0.92);
  border: 1px solid #334155;
  border-radius: 4px;
  font-size: 11px;
  font-family: 'Consolas', monospace;
  color: #e2e8f0;
  pointer-events: none;
  min-width: 180px;
}
.fx-tooltip.top { border-color: rgba(239, 68, 68, 0.5); }
.fx-tooltip.bottom { border-color: rgba(16, 185, 129, 0.5); }
.fx-tooltip .title {
  font-weight: 600;
  margin-bottom: 4px;
  font-size: 12px;
}
.fx-tooltip.top .title { color: #ef4444; }
.fx-tooltip.bottom .title { color: #10b981; }
.fx-tooltip .row {
  display: flex;
  align-items: center;
  gap: 6px;
  line-height: 1.5;
}
.fx-tooltip .row.peak { color: #fbbf24; }
.fx-tooltip .lbl {
  width: 16px;
  color: #94a3b8;
  flex-shrink: 0;
}
.fx-tooltip .row.peak .lbl { color: #fbbf24; }
.fx-tooltip .date {
  flex: 1;
}
.fx-tooltip .prc {
  color: #cbd5e1;
}
</style>
