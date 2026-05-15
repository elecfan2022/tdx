<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import KChart from './components/KChart.vue'
import Watchlist from './components/Watchlist.vue'
import ChanList from './components/ChanList.vue'
import type { KLineData } from 'klinecharts'

declare global {
  interface Window {
    go: any
  }
}

type Period = '1m' | '5m' | '30m' | 'day' | 'week' | 'month'
type RightPanel = 'watchlist' | 'chan' | null

const periods: { value: Period; label: string }[] = [
  { value: '1m', label: '1分钟' },
  { value: '5m', label: '5分钟' },
  { value: '30m', label: '30分钟' },
  { value: 'day', label: '日线' },
  { value: 'week', label: '周线' },
  { value: 'month', label: '月线' },
]

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
interface Segment {
  from: Fractal
  to: Fractal
  direction: 'up' | 'down'
  anotherTransition?: Fractal | null
  terminationCase: number
  subcase: number
}

const code = ref('000001')
const codeInput = ref('000001')
const name = ref('')
const period = ref<Period>('day')
const data = ref<KLineData[]>([])
const fractals = ref<Fractal[]>([])
const bis = ref<Bi[]>([])
const segments = ref<Segment[]>([])

// 右侧面板：自选股 / 分型笔列表 互斥；null 表示都不显示
const rightPanel = ref<RightPanel>('watchlist')
const watchlistRef = ref<InstanceType<typeof Watchlist> | null>(null)
const chartRef = ref<InstanceType<typeof KChart> | null>(null)

// 主图缠论显示开关（独立勾选，可同时关掉）
const showFractals = ref(true)
const showBis = ref(true)
const showSegments = ref(true)

// 数据来源选择（至少勾一个；都勾即"本地+实时补齐"模式）
const useRealtime = ref(true)
const useLocal = ref(false)
const cutoffDate = ref('') // YYYY-MM-DD，仅在 useLocal 时显示

const showDisplayMenu = ref(false)
const displayMenuRef = ref<HTMLDivElement | null>(null)

// 防止两个数据源都被取消勾选
function ensureAtLeastOneSource(changed: 'realtime' | 'local') {
  if (!useRealtime.value && !useLocal.value) {
    // 用户刚刚取消的那个，强制保留另一个
    if (changed === 'realtime') {
      useLocal.value = true
    } else {
      useRealtime.value = true
    }
  }
  loadKline()
}

function toggleDisplayMenu() {
  showDisplayMenu.value = !showDisplayMenu.value
}

// 设置下拉
const showSettingsMenu = ref(false)
const settingsMenuRef = ref<HTMLDivElement | null>(null)
const tdxDirInput = ref('')
const tdxDirSaved = ref('')
const settingsMsg = ref('')

function toggleSettingsMenu() {
  showSettingsMenu.value = !showSettingsMenu.value
  if (showSettingsMenu.value) {
    // 打开时同步当前已保存值，避免刚改一半被覆盖
    tdxDirInput.value = tdxDirSaved.value
    settingsMsg.value = ''
  }
}

async function loadSettings() {
  try {
    const s = await window.go.main.App.GetSettings()
    tdxDirSaved.value = s?.tdxDir ?? ''
    tdxDirInput.value = tdxDirSaved.value
  } catch {
    /* ignore */
  }
}

async function saveTdxDir() {
  try {
    const s = await window.go.main.App.SetTdxDir(tdxDirInput.value.trim())
    tdxDirSaved.value = s?.tdxDir ?? ''
    settingsMsg.value = '已保存'
  } catch (e: any) {
    settingsMsg.value = '错误：' + String(e?.message ?? e)
  }
}

function onDocClick(e: MouseEvent) {
  const target = e.target as Node
  if (showDisplayMenu.value && displayMenuRef.value && !displayMenuRef.value.contains(target)) {
    showDisplayMenu.value = false
  }
  if (showSettingsMenu.value && settingsMenuRef.value && !settingsMenuRef.value.contains(target)) {
    showSettingsMenu.value = false
  }
}

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

// 防竞态：每次 loadKline 拿一个递增 token；await 返回后校验是不是最新的，
// 否则丢弃（避免用户快速切代码/周期时旧请求覆盖新结果）
let klineToken = 0

async function loadKline() {
  const token = ++klineToken
  const reqCode = code.value
  const reqPeriod = period.value
  const reqUseRealtime = useRealtime.value
  const reqUseLocal = useLocal.value
  const reqCutoff = useLocal.value ? cutoffDate.value.trim() : ''
  // 数量上限：纯实时 8000，含本地 20000
  const reqCount = reqUseLocal ? 20000 : 8000
  statusMsg.value = '加载中…'
  try {
    const resp = await window.go.main.App.GetKline(
      reqCode, reqPeriod, reqCount, reqUseRealtime, reqUseLocal, reqCutoff,
    )
    if (token !== klineToken) return // 已有更新的请求在路上，丢弃此次结果
    const list = resp?.klines ?? []
    data.value = list.map((b: any): KLineData => ({
      timestamp: b.timestamp,
      open: b.open,
      high: b.high,
      low: b.low,
      close: b.close,
      volume: b.volume,
      turnover: b.turnover,
    }))
    fractals.value = resp?.fractals ?? []
    bis.value = resp?.bis ?? []
    segments.value = resp?.segments ?? []
    lastUpdate.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
    const segC1 = segments.value.filter(s => s.terminationCase === 1).length
    const segC2 = segments.value.filter(s => s.terminationCase === 2).length
    statusMsg.value = `K 线 ${data.value.length} 根 · 分型 ${fractals.value.length} · 笔 ${bis.value.length} · 线段 ${segments.value.length}(一类 ${segC1}/二类 ${segC2})`
  } catch (e: any) {
    if (token === klineToken) {
      statusMsg.value = '错误：' + String(e?.message ?? e)
    }
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

function pickFromWatchlist(c: string, n: string) {
  code.value = c
  codeInput.value = c
  name.value = n
  if (!n) loadName()
  loadKline()
}

async function addCurrentToWatchlist() {
  try {
    await window.go.main.App.AddToWatchlist(code.value)
    rightPanel.value = 'watchlist'
    watchlistRef.value?.reload()
    statusMsg.value = `已添加 ${code.value} 到自选股`
  } catch (e: any) {
    statusMsg.value = '添加失败：' + String(e?.message ?? e)
  }
}

function togglePanel(p: 'watchlist' | 'chan') {
  rightPanel.value = rightPanel.value === p ? null : p
}

function jumpToTimestamp(ts: number) {
  chartRef.value?.scrollTo(ts)
}

onMounted(() => {
  refreshStatus()
  loadName()
  loadKline()
  loadSettings()
  statusTimer = window.setInterval(refreshStatus, 3000)
  document.addEventListener('click', onDocClick)
})

onBeforeUnmount(() => {
  if (statusTimer) window.clearInterval(statusTimer)
  document.removeEventListener('click', onDocClick)
})
</script>

<template>
  <div class="app">
    <!-- 菜单栏 -->
    <nav class="menubar">
      <button
        class="menu-item"
        :class="{ active: rightPanel === 'watchlist' }"
        @click="togglePanel('watchlist')"
      >
        自选股
      </button>
      <button
        class="menu-item"
        :class="{ active: rightPanel === 'chan' }"
        @click="togglePanel('chan')"
      >
        分型/笔
      </button>

      <span class="menu-sep" />

      <div ref="displayMenuRef" class="display-menu">
        <button
          class="menu-item"
          :class="{ active: showDisplayMenu }"
          @click.stop="toggleDisplayMenu"
        >
          显示 ▾
        </button>
        <div v-if="showDisplayMenu" class="display-dropdown">
          <label class="display-opt">
            <input type="checkbox" v-model="showFractals" />
            <span>分型</span>
          </label>
          <label class="display-opt">
            <input type="checkbox" v-model="showBis" />
            <span>笔</span>
          </label>
          <label class="display-opt">
            <input type="checkbox" v-model="showSegments" />
            <span>线段</span>
          </label>
          <div class="display-divider" />
          <label class="display-opt">
            <input
              type="checkbox"
              :checked="useRealtime"
              @change="(e) => { useRealtime = (e.target as HTMLInputElement).checked; ensureAtLeastOneSource('realtime') }"
            />
            <span>实时数据</span>
          </label>
          <label class="display-opt">
            <input
              type="checkbox"
              :checked="useLocal"
              @change="(e) => { useLocal = (e.target as HTMLInputElement).checked; ensureAtLeastOneSource('local') }"
            />
            <span>本地数据</span>
          </label>
          <div v-if="useLocal" class="display-cutoff">
            <label class="display-cutoff-label">截至日期</label>
            <input
              v-model="cutoffDate"
              class="display-cutoff-input"
              placeholder="YYYY-MM-DD（空=至今）"
              @change="loadKline"
              @keyup.enter="loadKline"
            />
            <button
              class="display-cutoff-clear"
              title="清空截至日期"
              @click="() => { cutoffDate = ''; loadKline() }"
            >
              ×
            </button>
          </div>
        </div>
      </div>

      <div ref="settingsMenuRef" class="display-menu">
        <button
          class="menu-item"
          :class="{ active: showSettingsMenu }"
          @click.stop="toggleSettingsMenu"
        >
          设置 ▾
        </button>
        <div v-if="showSettingsMenu" class="settings-dropdown" @click.stop>
          <div class="settings-row">
            <label class="settings-label">通达信目录</label>
            <input
              v-model="tdxDirInput"
              class="settings-input"
              placeholder="例如 D:\new_tdx"
              @keyup.enter="saveTdxDir"
            />
            <button class="settings-save" @click="saveTdxDir">保存</button>
          </div>
          <div v-if="settingsMsg" class="settings-msg">{{ settingsMsg }}</div>
        </div>
      </div>
    </nav>

    <!-- 主体 -->
    <div class="body">
      <section class="main-pane">
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

        <main class="chart-wrap">
          <KChart
            ref="chartRef"
            :data="data"
            :period="period"
            :fractals="fractals"
            :bis="bis"
            :segments="segments"
            :show-fractals="showFractals"
            :show-bis="showBis"
            :show-segments="showSegments"
          />
        </main>
      </section>

      <Watchlist
        v-show="rightPanel === 'watchlist'"
        ref="watchlistRef"
        :active-code="code"
        :codes-ready="codesReady"
        @select="pickFromWatchlist"
      />
      <ChanList
        v-if="rightPanel === 'chan'"
        :fractals="fractals"
        :bis="bis"
        :period="period"
        :code="code"
        :use-realtime="useRealtime"
        :use-local="useLocal"
        :cutoff-date="cutoffDate"
        @pick="jumpToTimestamp"
      />
    </div>

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
  gap: 6px;
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
.menu-sep {
  width: 1px;
  height: 18px;
  margin: 0 6px;
  background: #334155;
}
/* 显示菜单 + 下拉 */
.display-menu {
  position: relative;
}
.display-dropdown {
  position: absolute;
  top: 100%;
  left: 0;
  margin-top: 2px;
  min-width: 120px;
  padding: 4px 0;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 4px;
  box-shadow: 0 4px 12px rgba(0,0,0,0.4);
  z-index: 100;
}
.display-opt {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  font-size: 13px;
  color: #cbd5e1;
  cursor: pointer;
}
.display-opt:hover {
  background: #334155;
}
.display-opt input[type='checkbox'] {
  margin: 0;
  cursor: pointer;
  accent-color: #2563eb;
}
.display-divider {
  height: 1px;
  margin: 4px 0;
  background: #334155;
}
.display-cutoff {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px 6px 32px;
}
.display-cutoff-label {
  font-size: 11px;
  color: #94a3b8;
  white-space: nowrap;
}
.display-cutoff-input {
  flex: 1;
  min-width: 0;
  padding: 3px 6px;
  font-family: 'Consolas', monospace;
  font-size: 11px;
  background: #0f172a;
  color: #e2e8f0;
  border: 1px solid #475569;
  border-radius: 3px;
}
.display-cutoff-input:focus {
  outline: none;
  border-color: #2563eb;
}
.display-cutoff-clear {
  flex: 0 0 auto;
  width: 22px;
  height: 22px;
  padding: 0;
  font-size: 14px;
  line-height: 1;
  background: #334155;
  color: #cbd5e1;
  border: 1px solid #475569;
  border-radius: 3px;
  cursor: pointer;
}
.display-cutoff-clear:hover {
  background: #ef4444;
  color: #fff;
  border-color: #ef4444;
}

/* 设置下拉 */
.settings-dropdown {
  position: absolute;
  top: 100%;
  left: 0;
  margin-top: 2px;
  min-width: 320px;
  padding: 10px 12px;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 4px;
  box-shadow: 0 4px 12px rgba(0,0,0,0.4);
  z-index: 100;
}
.settings-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.settings-label {
  font-size: 12px;
  color: #94a3b8;
  white-space: nowrap;
}
.settings-input {
  flex: 1;
  min-width: 0;
  padding: 4px 8px;
  font-family: 'Consolas', monospace;
  font-size: 12px;
  background: #0f172a;
  color: #e2e8f0;
  border: 1px solid #475569;
  border-radius: 3px;
}
.settings-input:focus {
  outline: none;
  border-color: #2563eb;
}
.settings-save {
  padding: 4px 12px;
  font-size: 12px;
  background: #2563eb;
  color: #fff;
  border: none;
  border-radius: 3px;
  cursor: pointer;
}
.settings-save:hover { background: #1d4ed8; }
.settings-msg {
  margin-top: 6px;
  font-size: 11px;
  color: #10b981;
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
