<script setup lang="ts">
import { ref, computed } from 'vue'

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
  fractals: Fractal[]
  bis: Bi[]
  period: string
}>()

const emit = defineEmits<{
  (e: 'pick', timestamp: number): void
}>()

const tab = ref<'fractals' | 'bis'>('bis')

function fmtTime(ts: number): string {
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

// 默认按时间倒序展示，最近的在最上面
const fractalsDesc = computed(() => [...props.fractals].reverse())
const bisDesc = computed(() => [...props.bis].reverse())
</script>

<template>
  <aside class="chan-list">
    <header class="tabs">
      <button
        class="tab"
        :class="{ active: tab === 'bis' }"
        @click="tab = 'bis'"
      >
        笔 ({{ bis.length }})
      </button>
      <button
        class="tab"
        :class="{ active: tab === 'fractals' }"
        @click="tab = 'fractals'"
      >
        分型 ({{ fractals.length }})
      </button>
    </header>

    <div v-if="tab === 'bis'" class="rows">
      <div v-if="bis.length === 0" class="empty">无</div>
      <div
        v-for="(bi, i) in bisDesc"
        :key="bi.from.timestamp + '-' + bi.to.timestamp"
        class="bi-row"
        @click="emit('pick', bi.to.timestamp)"
      >
        <div class="bi-line">
          <span class="idx">#{{ bisDesc.length - i }}</span>
          <span :class="['mark', bi.from.type]">{{ bi.from.type === 'top' ? '顶' : '底' }}</span>
          <span class="time">{{ fmtTime(bi.from.timestamp) }}</span>
          <span class="price">{{ bi.from.price.toFixed(2) }}</span>
        </div>
        <div class="bi-line indent">
          <span class="arrow">↓</span>
          <span :class="['mark', bi.to.type]">{{ bi.to.type === 'top' ? '顶' : '底' }}</span>
          <span class="time">{{ fmtTime(bi.to.timestamp) }}</span>
          <span class="price">{{ bi.to.price.toFixed(2) }}</span>
        </div>
      </div>
    </div>

    <div v-else class="rows">
      <div v-if="fractals.length === 0" class="empty">无</div>
      <div
        v-for="(fx, i) in fractalsDesc"
        :key="fx.timestamp + '-' + i"
        class="fx-row"
        @click="emit('pick', fx.timestamp)"
      >
        <span class="idx">#{{ fractalsDesc.length - i }}</span>
        <span :class="['mark', fx.type]">{{ fx.type === 'top' ? '顶' : '底' }}</span>
        <span class="time">{{ fmtTime(fx.timestamp) }}</span>
        <span class="price">{{ fx.price.toFixed(2) }}</span>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.chan-list {
  width: 240px;
  display: flex;
  flex-direction: column;
  background: #1e293b;
  border-left: 1px solid #334155;
  flex-shrink: 0;
  overflow: hidden;
}
.tabs {
  display: flex;
  border-bottom: 1px solid #334155;
  flex-shrink: 0;
}
.tab {
  flex: 1;
  padding: 6px 0;
  font-size: 12px;
  background: transparent;
  color: #94a3b8;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
}
.tab:hover {
  color: #cbd5e1;
}
.tab.active {
  color: #fbbf24;
  border-bottom-color: #fbbf24;
}
.rows {
  flex: 1;
  overflow-y: auto;
  font-size: 12px;
  font-family: 'Consolas', monospace;
}
.empty {
  padding: 20px;
  text-align: center;
  color: #64748b;
}
.fx-row,
.bi-row {
  padding: 4px 8px;
  border-bottom: 1px solid #273548;
  cursor: pointer;
}
.fx-row:hover,
.bi-row:hover {
  background: #334155;
}
.fx-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.bi-line {
  display: flex;
  align-items: center;
  gap: 6px;
  line-height: 1.5;
}
.bi-line.indent {
  padding-left: 22px;
}
.idx {
  color: #64748b;
  width: 36px;
  text-align: right;
  flex-shrink: 0;
}
.mark {
  width: 18px;
  text-align: center;
  font-weight: 600;
  flex-shrink: 0;
}
.mark.top {
  color: #ef4444;
}
.mark.bottom {
  color: #10b981;
}
.arrow {
  color: #fbbf24;
  width: 18px;
  text-align: center;
  font-weight: bold;
  flex-shrink: 0;
}
.time {
  flex: 1;
  color: #cbd5e1;
}
.price {
  color: #e2e8f0;
}
</style>
