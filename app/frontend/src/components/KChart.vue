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
  chart.setStyles({
    grid: {
      show: false,
    },
    candle: {
      bar: {
        // A 股惯例：红涨绿跌
        upColor: '#EF4444',
        downColor: '#10B981',
        upBorderColor: '#EF4444',
        downBorderColor: '#10B981',
        upWickColor: '#EF4444',
        downWickColor: '#10B981',
      },
    },
  })
  chart.createIndicator('MA', false, { id: 'candle_pane' })
  chart.createIndicator('VOL')
  chart.createIndicator('MACD')
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
