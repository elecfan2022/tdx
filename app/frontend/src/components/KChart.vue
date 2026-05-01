<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { init, dispose, type Chart, type KLineData } from 'klinecharts'

const props = defineProps<{
  data: KLineData[]
  period: string
}>()

const containerRef = ref<HTMLDivElement | null>(null)
let chart: Chart | null = null

function timezone(period: string) {
  return ['day', 'week', 'month'].includes(period) ? 'Asia/Shanghai' : 'Asia/Shanghai'
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
})

onBeforeUnmount(() => {
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
