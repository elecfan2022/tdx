<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import KChart from './components/KChart.vue'
import Watchlist from './components/Watchlist.vue'
import type { KLineData } from 'klinecharts'

declare global {
  interface Window {
    go: any
  }
}

type Period = '1m' | '5m' | '30m' | 'day' | 'week' | 'month'
const periods: { value: Period; label: string }[] = [
  { value: '1m', label: '1分钟' },
  { value: '5m', label: '5分钟' },
  { value: '30m', label: '30分钟' },
  { value: 'day', label: '日线' },
  { value: 'week', label: '周线' },
  { value: 'month', label: '月线' },
]

const code = ref('000001')
const codeInput = ref('000001')
const name = ref('')
const period = ref<Period>('day')
const data = ref<KLineData[]>([])

// 自选股侧边栏
const showWatchlist = ref(true)
const watchlistRef = ref<InstanceType<typeof Watchlist> | null>(null)

// 状态栏
const connected = ref(false)
const codesReady = ref(false)
const stockCount = ref(0)
const lastUpdate = ref('')
const statusMsg = ref('就绪')

let statusTimer: number | null = null

async function refreshStatus() {
  try {
    const s = await window.go.main.App.GetStatus()
    connected.value = !!s.connected
    codesReady.value = !!s.codesReady
    stockCount.value = s.stockCount ?? 0
    if (codesReady.value && !name.value) loadName()
  } catch {
    /* ignore */
  }
}

async function loadName() {
  try {
    const info = await window.go.main.App.GetStockInfo(code.value)
    name.value = info?.name ?? ''
  } catch {
    name.value = ''
  }
}

async function loadKline() {
  statusMsg.value = '加载中…'
  try {
    const list = await window.go.main.App.GetKline(code.value, period.value, 320)
    data.value = (list ?? []).map((b: any): KLineData => ({
      timestamp: b.timestamp,
      open: b.open,
      high: b.high,
      low: b.low,
      close: b.close,
      volume: b.volume,
      turnover: b.turnover,
    }))
    lastUpdate.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
    statusMsg.value = `共 ${data.value.length} 根 K 线`
  } catch (e: any) {
    statusMsg.value = '错误：' + String(e?.message ?? e)
  }
}

function applyCode() {
  const c = codeInput.value.trim()
  if (c.length !== 6 || !/^\d{6}$/.test(c)) {
    statusMsg.value = '股票代码必须是 6 位数字'
    return
  }
  code.value = c
  name.value = ''
  loadName()
  loadKline()
}

function selectPeriod(p: Period) {
  period.value = p
  loadKline()
}

// 自选股选中：刷新左侧主图
function pickFromWatchlist(c: string, n: string) {
  code.value = c
  codeInput.value = c
  name.value = n
  if (!n) loadName()
  loadKline()
}

// 把当前股票加入自选股
async function addCurrentToWatchlist() {
  try {
    await window.go.main.App.AddToWatchlist(code.value)
    showWatchlist.value = true
    watchlistRef.value?.reload()
    statusMsg.value = `已添加 ${code.value} 到自选股`
  } catch (e: any) {
    statusMsg.value = '添加失败：' + String(e?.message ?? e)
  }
}

onMounted(() => {
  refreshStatus()
  loadName()
  loadKline()
  statusTimer = window.setInterval(refreshStatus, 3000)
})

onBeforeUnmount(() => {
  if (statusTimer) window.clearInterval(statusTimer)
})
</script>

<template>
  <div class="app">
    <!-- 菜单栏 -->
    <nav class="menubar">
      <button
        class="menu-item"
        :class="{ active: showWatchlist }"
        @click="showWatchlist = !showWatchlist"
      >
        自选股
      </button>
    </nav>

    <!-- 主体：左侧（header + 图） + 右侧自选股 -->
    <div class="body">
      <section class="main-pane">
        <!-- 第一行：股票名称 / 代码 + 周期按钮 -->
        <header class="header">
          <div class="title">
            <span class="name">{{ name || (codesReady ? '未知' : '加载中…') }}</span>
            <input
              v-model="codeInput"
              class="code"
              maxlength="6"
              @keyup.enter="applyCode"
              @blur="applyCode"
            />
            <button class="add" title="加入自选股" @click="addCurrentToWatchlist">+</button>
          </div>
          <div class="periods">
            <button
              v-for="p in periods"
              :key="p.value"
              :class="{ active: period === p.value }"
              @click="selectPeriod(p.value)"
            >
              {{ p.label }}
            </button>
          </div>
        </header>

        <!-- 主图：K 线 + VOL + MACD -->
        <main class="chart-wrap">
          <KChart :data="data" :period="period" />
        </main>
      </section>

      <Watchlist
        v-show="showWatchlist"
        ref="watchlistRef"
        :active-code="code"
        :codes-ready="codesReady"
        @select="pickFromWatchlist"
      />
    </div>

    <!-- 状态栏 -->
    <footer class="statusbar">
      <span class="dot" :class="{ on: connected }" />
      <span>{{ connected ? '已连接' : '未连接' }}</span>
      <span class="sep">|</span>
      <span>代码库: {{ codesReady ? `${stockCount} 只` : '加载中' }}</span>
      <span class="sep">|</span>
      <span class="msg">{{ statusMsg }}</span>
      <span class="spacer" />
      <span v-if="lastUpdate">更新: {{ lastUpdate }}</span>
    </footer>
  </div>
</template>

<style>
html, body, #app {
  height: 100%;
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  background: #0f172a;
  color: #e2e8f0;
}
* { box-sizing: border-box; }

.app {
  display: flex;
  flex-direction: column;
  height: 100vh;
}

/* 菜单栏 */
.menubar {
  display: flex;
  align-items: center;
  padding: 0 6px;
  background: #0b1220;
  border-bottom: 1px solid #334155;
  flex-shrink: 0;
}
.menubar .menu-item {
  padding: 6px 14px;
  font-size: 13px;
  color: #cbd5e1;
  background: transparent;
  border: none;
  cursor: pointer;
  border-bottom: 2px solid transparent;
}
.menubar .menu-item:hover {
  color: #fff;
}
.menubar .menu-item.active {
  color: #fbbf24;
  border-bottom-color: #fbbf24;
}

/* 主体 */
.body {
  flex: 1;
  display: flex;
  min-height: 0;
}
.main-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

/* 第一行 */
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  background: #1e293b;
  border-bottom: 1px solid #334155;
  flex-shrink: 0;
}
.title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.title .name {
  font-size: 18px;
  font-weight: 600;
  color: #fbbf24;
}
.title .code {
  width: 80px;
  padding: 3px 6px;
  font-family: 'Consolas', monospace;
  font-size: 14px;
  background: #0f172a;
  color: #e2e8f0;
  border: 1px solid #475569;
  border-radius: 3px;
}
.title .add {
  width: 24px;
  height: 24px;
  padding: 0;
  font-size: 16px;
  line-height: 1;
  background: #334155;
  color: #cbd5e1;
  border: 1px solid #475569;
  border-radius: 3px;
  cursor: pointer;
}
.title .add:hover {
  background: #2563eb;
  color: #fff;
  border-color: #2563eb;
}
.periods {
  display: flex;
  gap: 4px;
}
.periods button {
  padding: 4px 14px;
  font-size: 13px;
  background: #334155;
  color: #cbd5e1;
  border: 1px solid #475569;
  border-radius: 3px;
  cursor: pointer;
}
.periods button:hover {
  background: #475569;
}
.periods button.active {
  background: #2563eb;
  color: #fff;
  border-color: #2563eb;
}

/* 主图 */
.chart-wrap {
  flex: 1;
  min-height: 0;
}

/* 状态栏 */
.statusbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 12px;
  background: #1e293b;
  border-top: 1px solid #334155;
  font-size: 12px;
  color: #94a3b8;
  flex-shrink: 0;
}
.statusbar .dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #ef4444;
}
.statusbar .dot.on {
  background: #10b981;
}
.statusbar .sep {
  color: #475569;
}
.statusbar .msg {
  color: #cbd5e1;
}
.statusbar .spacer {
  flex: 1;
}
</style>
