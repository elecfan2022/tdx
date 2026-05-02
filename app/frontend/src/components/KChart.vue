<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { init, dispose, registerOverlay, type Chart, type KLineData } from 'klinecharts'

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
}
interface Bi {
  from: Fractal
  to: Fractal
}

const props = defineProps<{
  data: KLineData[]
  period: string
  fractals: Fractal[]
  bis: Bi[]
  showFractals: boolean
  showBis: boolean
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
}

// 给外部用：滚动到指定时间戳
function scrollTo(timestamp: number) {
  if (!chart) return
  // KLineChart v9 提供按时间戳滚动
  ;(chart as any).scrollToTimestamp?.(timestamp, 200)
}
defineExpose({ scrollTo })

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

  // 跟随容器尺寸变化（窗口最大化、侧栏切换等）
  resizeObs = new ResizeObserver(() => {
    chart?.resize()
  })
  resizeObs.observe(containerRef.value)
})

onBeforeUnmount(() => {
  resizeObs?.disconnect()
  resizeObs = null
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
  () => [props.fractals, props.bis, props.showFractals, props.showBis],
  () => drawChan(),
  { deep: false },
)
</script>

<template>
  <div ref="containerRef" class="kchart" />
</template>

<style scoped>
.kchart {
  width: 100%;
  height: 100%;
  min-height: 480px;
}
</style>
