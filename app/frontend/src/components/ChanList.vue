<script setup lang="ts">
import { ref, computed } from 'vue'

interface Fractal {
  type: 'top' | 'bottom'
  index: number
  timestamp: number
  price: number
  isEndpoint?: boolean
}
interface Bi {
  from: Fractal
  to: Fractal
}

const props = defineProps<{
  fractals: Fractal[]
  bis: Bi[]
  period: string
  code: string
  useRealtime: boolean
  useLocal: boolean
  cutoffDate: string
}>()

const emit = defineEmits<{
  (e: 'pick', timestamp: number): void
}>()

const tab = ref<'fractals' | 'bis' | 'diag'>('bis')
const diagMode = ref<'bi' | 'segment'>('bi')

// 诊断输入
const diagFrom = ref('')
const diagTo = ref('')
const diagResult = ref<any>(null)
const diagLoading = ref(false)
const diagError = ref('')

// 段诊断输入
const segDiagStart = ref('')
const segDiagResult = ref<any>(null)

async function runDiag() {
  diagError.value = ''
  diagResult.value = null
  segDiagResult.value = null
  if (diagMode.value === 'bi') {
    if (!diagFrom.value || !diagTo.value) {
      diagError.value = '请输入两个日期 (YYYY-MM-DD)'
      return
    }
    diagLoading.value = true
    try {
      diagResult.value = await window.go.main.App.DiagnoseBi(
        props.code,
        props.period,
        diagFrom.value,
        diagTo.value,
      )
    } catch (e: any) {
      diagError.value = String(e?.message ?? e)
    } finally {
      diagLoading.value = false
    }
  } else {
    if (!segDiagStart.value) {
      diagError.value = '请输入段起点日期'
      return
    }
    diagLoading.value = true
    try {
      segDiagResult.value = await window.go.main.App.DiagnoseSegment(
        props.code,
        props.period,
        props.useRealtime,
        props.useLocal,
        props.useLocal ? props.cutoffDate : '',
        segDiagStart.value,
      )
    } catch (e: any) {
      diagError.value = String(e?.message ?? e)
    } finally {
      diagLoading.value = false
    }
  }
}

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
      <button
        class="tab"
        :class="{ active: tab === 'diag' }"
        @click="tab = 'diag'"
      >
        诊断
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

    <div v-else-if="tab === 'fractals'" class="rows">
      <div v-if="fractals.length === 0" class="empty">无</div>
      <div
        v-for="(fx, i) in fractalsDesc"
        :key="fx.timestamp + '-' + i"
        class="fx-row"
        :class="{ endpoint: fx.isEndpoint }"
        @click="emit('pick', fx.timestamp)"
      >
        <span class="idx">#{{ fractalsDesc.length - i }}</span>
        <span :class="['mark', fx.type]">{{ fx.type === 'top' ? '顶' : '底' }}</span>
        <span class="time">{{ fmtTime(fx.timestamp) }}</span>
        <span class="price">{{ fx.price.toFixed(2) }}</span>
        <span class="endpoint-tag" :title="fx.isEndpoint ? '是笔端点' : '不是笔端点'">
          {{ fx.isEndpoint ? '★' : '·' }}
        </span>
      </div>
    </div>

    <div v-else class="diag">
      <div class="diag-mode-switch">
        <label>
          <input type="radio" v-model="diagMode" value="bi" />
          笔诊断
        </label>
        <label>
          <input type="radio" v-model="diagMode" value="segment" />
          段诊断
        </label>
      </div>
      <div v-if="diagMode === 'bi'" class="diag-form">
        <label>
          起点
          <input v-model="diagFrom" placeholder="1997-05-30" />
        </label>
        <label>
          终点
          <input v-model="diagTo" placeholder="1997-09-30" />
        </label>
        <button :disabled="diagLoading" @click="runDiag">
          {{ diagLoading ? '诊断中…' : '诊断笔' }}
        </button>
        <div v-if="diagError" class="err">{{ diagError }}</div>
      </div>
      <div v-else class="diag-form">
        <label>
          段起点
          <input v-model="segDiagStart" placeholder="2021-02-05" />
        </label>
        <button :disabled="diagLoading" @click="runDiag">
          {{ diagLoading ? '诊断中…' : '诊断段' }}
        </button>
        <div v-if="diagError" class="err">{{ diagError }}</div>
      </div>

      <div v-if="segDiagResult" class="diag-result">
        <div v-if="!segDiagResult.found" class="diag-note">
          {{ segDiagResult.note }}
        </div>
        <template v-else>
          <div class="diag-section">
            <div class="diag-label">线段</div>
            <div>方向 {{ segDiagResult.direction === 'up' ? '向上 ↑' : '向下 ↓' }}</div>
            <div class="diag-sub">起点 {{ fmtTime(segDiagResult.segFrom.timestamp) }} 价 {{ segDiagResult.segFrom.price.toFixed(2) }}</div>
            <div class="diag-sub">终点 {{ fmtTime(segDiagResult.segTo.timestamp) }} 价 {{ segDiagResult.segTo.price.toFixed(2) }}</div>
            <div class="diag-sub">
              <template v-if="segDiagResult.terminationCase === 1">终止于 第一种情况（无缺口）</template>
              <template v-else-if="segDiagResult.terminationCase === 2">终止于 第二种情况（有缺口）</template>
              <template v-else>未终止</template>
              <template v-if="segDiagResult.subcase === 1">· subcase 1a</template>
              <template v-else-if="segDiagResult.subcase === 2">· subcase 1b</template>
            </div>
            <div v-if="segDiagResult.anotherTransition" class="diag-sub">
              另一转折点 {{ fmtTime(segDiagResult.anotherTransition.timestamp) }} 价 {{ segDiagResult.anotherTransition.price.toFixed(2) }}
            </div>
          </div>
          <div class="diag-section">
            <div class="diag-label">缺口判定</div>
            <div class="diag-rule">{{ segDiagResult.gapDescription }}</div>
          </div>
          <div class="diag-section">
            <div class="diag-label">第一CS 元素链 (前包后合并后)</div>
            <div
              v-for="(c, ci) in segDiagResult.csA"
              :key="ci"
              class="diag-rule"
              :class="{
                'cs-a': segDiagResult.fractalIdx && ci === segDiagResult.fractalIdx[0],
                'cs-b': segDiagResult.fractalIdx && ci === segDiagResult.fractalIdx[1],
                'cs-c': segDiagResult.fractalIdx && ci === segDiagResult.fractalIdx[2],
              }"
            >
              <template v-if="segDiagResult.fractalIdx && ci === segDiagResult.fractalIdx[0]">[a]</template>
              <template v-else-if="segDiagResult.fractalIdx && ci === segDiagResult.fractalIdx[1]">[b]</template>
              <template v-else-if="segDiagResult.fractalIdx && ci === segDiagResult.fractalIdx[2]">[c]</template>
              <template v-else>#{{ ci }}</template>
              high={{ c.high.toFixed(2) }} low={{ c.low.toFixed(2) }}
              [{{ fmtTime(c.fromTs) }} → {{ fmtTime(c.toTs) }}] bi[{{ c.biStartIdx }}..{{ c.biEndIdx }}]
            </div>
          </div>
          <div v-if="segDiagResult.note" class="diag-note">{{ segDiagResult.note }}</div>

          <!-- subcase 1a 双 CS trace -->
          <template v-if="segDiagResult.dualCS">
            <div class="diag-section">
              <div class="diag-label">双 CS 验证触发</div>
              <div class="diag-rule">
                <template v-if="segDiagResult.dualCS.trigger === 'csA_fractal'">
                  ★ CS-A 出现段方向相反分型 → 段终止
                </template>
                <template v-else-if="segDiagResult.dualCS.trigger === 'csB_fractal'">
                  ★ CS-B 出现 opposite 分型 → 段终止 + 另一转折点
                </template>
                <template v-else-if="segDiagResult.dualCS.trigger === 'break_end'">
                  ★ 破破坏笔结束点 → 段终止 (兜底)
                </template>
                <template v-else-if="segDiagResult.dualCS.trigger === 'break_start'">
                  ★ 破破坏笔开始点 → 段延续 (不应出现在已确认的段中)
                </template>
                <template v-else-if="segDiagResult.dualCS.trigger === 'exhausted'">
                  ★ 数据扫完无任何信号
                </template>
                <template v-else>{{ segDiagResult.dualCS.trigger }}</template>
              </div>
              <div class="diag-sub">破坏笔区间：[{{ segDiagResult.dualCS.breakingLow.toFixed(2) }}, {{ segDiagResult.dualCS.breakingHigh.toFixed(2) }}]</div>
              <div v-if="segDiagResult.dualCS.triggerBiIdx >= 0" class="diag-sub">触发笔下标 bi[{{ segDiagResult.dualCS.triggerBiIdx }}]</div>
            </div>
            <div class="diag-section" v-if="segDiagResult.dualCS.csA && segDiagResult.dualCS.csA.length > 0">
              <div class="diag-label">CS-A 元素链 (反向笔，前包后)</div>
              <div
                v-for="(c, ci) in segDiagResult.dualCS.csA"
                :key="'csa-'+ci"
                class="diag-rule"
                :class="{
                  'cs-a': segDiagResult.dualCS.trigger === 'csA_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[0],
                  'cs-b': segDiagResult.dualCS.trigger === 'csA_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[1],
                  'cs-c': segDiagResult.dualCS.trigger === 'csA_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[2],
                }"
              >
                <template v-if="segDiagResult.dualCS.trigger === 'csA_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[0]">[a]</template>
                <template v-else-if="segDiagResult.dualCS.trigger === 'csA_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[1]">[b]</template>
                <template v-else-if="segDiagResult.dualCS.trigger === 'csA_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[2]">[c]</template>
                <template v-else>#{{ ci }}</template>
                high={{ c.high.toFixed(2) }} low={{ c.low.toFixed(2) }}
                [{{ fmtTime(c.fromTs) }} → {{ fmtTime(c.toTs) }}] bi[{{ c.biStartIdx }}..{{ c.biEndIdx }}]
              </div>
            </div>
            <div class="diag-section" v-if="segDiagResult.dualCS.csB && segDiagResult.dualCS.csB.length > 0">
              <div class="diag-label">CS-B 元素链 (同向笔，不做包含)</div>
              <div
                v-for="(c, ci) in segDiagResult.dualCS.csB"
                :key="'csb-'+ci"
                class="diag-rule"
                :class="{
                  'cs-a': segDiagResult.dualCS.trigger === 'csB_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[0],
                  'cs-b': segDiagResult.dualCS.trigger === 'csB_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[1],
                  'cs-c': segDiagResult.dualCS.trigger === 'csB_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[2],
                }"
              >
                <template v-if="segDiagResult.dualCS.trigger === 'csB_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[0]">[a]</template>
                <template v-else-if="segDiagResult.dualCS.trigger === 'csB_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[1]">[b]</template>
                <template v-else-if="segDiagResult.dualCS.trigger === 'csB_fractal' && ci === segDiagResult.dualCS.triggerFractalIdx[2]">[c]</template>
                <template v-else>#{{ ci }}</template>
                high={{ c.high.toFixed(2) }} low={{ c.low.toFixed(2) }}
                [{{ fmtTime(c.fromTs) }} → {{ fmtTime(c.toTs) }}] bi[{{ c.biStartIdx }}..{{ c.biEndIdx }}]
              </div>
            </div>
            <div v-if="segDiagResult.dualCS.anotherTransition" class="diag-sub">
              CS-B 另一转折点：{{ fmtTime(segDiagResult.dualCS.anotherTransition.timestamp) }} 价 {{ segDiagResult.dualCS.anotherTransition.price.toFixed(2) }}
            </div>
          </template>
        </template>
      </div>

      <div v-if="diagResult" class="diag-result">
        <div v-if="!diagResult.fromFound || !diagResult.toFound" class="diag-note">
          {{ diagResult.note }}
          <ul>
            <li>起点找到：{{ diagResult.fromFound }}</li>
            <li>终点找到：{{ diagResult.toFound }}</li>
          </ul>
        </div>
        <template v-else>
          <div class="diag-section">
            <div class="diag-label">起点</div>
            <div>{{ diagResult.from.type === 'top' ? '顶' : '底' }}
              {{ fmtTime(diagResult.from.timestamp) }}
              价 {{ diagResult.from.price.toFixed(2) }}</div>
            <div class="diag-sub">K 线区间 {{ diagResult.from.kLow.toFixed(2) }} – {{ diagResult.from.kHigh.toFixed(2) }}</div>
            <div class="diag-sub">处理后下标 {{ diagResult.from.index }}, PeakIdx {{ diagResult.from.peakIdx }}</div>
          </div>
          <div class="diag-section">
            <div class="diag-label">终点</div>
            <div>{{ diagResult.to.type === 'top' ? '顶' : '底' }}
              {{ fmtTime(diagResult.to.timestamp) }}
              价 {{ diagResult.to.price.toFixed(2) }}</div>
            <div class="diag-sub">K 线区间 {{ diagResult.to.kLow.toFixed(2) }} – {{ diagResult.to.kHigh.toFixed(2) }}</div>
            <div class="diag-sub">处理后下标 {{ diagResult.to.index }}, PeakIdx {{ diagResult.to.peakIdx }}</div>
          </div>
          <div class="diag-section">
            <div class="diag-label">规则判定</div>
            <div class="diag-rule">{{ diagResult.rule1 }}</div>
            <div class="diag-rule">{{ diagResult.rule2 }}</div>
            <div class="diag-rule">{{ diagResult.rule3 }}</div>
          </div>
          <div class="diag-section diag-final" :class="{ pass: diagResult.allPass, fail: !diagResult.allPass }">
            {{ diagResult.allPass ? '✓ 三条规则都满足' : '✗ ' + diagResult.note }}
          </div>
        </template>
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
.fx-row.endpoint {
  background: rgba(251, 191, 36, 0.08);
}
.fx-row.endpoint:hover {
  background: rgba(251, 191, 36, 0.18);
}
.endpoint-tag {
  width: 14px;
  text-align: center;
  font-size: 11px;
  flex-shrink: 0;
}
.fx-row.endpoint .endpoint-tag {
  color: #fbbf24;
}
.fx-row:not(.endpoint) .endpoint-tag {
  color: #475569;
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

/* 诊断面板 */
.diag {
  flex: 1;
  overflow-y: auto;
  padding: 10px;
  font-size: 12px;
  font-family: 'Consolas', monospace;
}
.diag-form {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-bottom: 10px;
  border-bottom: 1px solid #334155;
  margin-bottom: 10px;
}
.diag-form label {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #94a3b8;
}
.diag-form input {
  flex: 1;
  padding: 3px 6px;
  background: #0f172a;
  color: #e2e8f0;
  border: 1px solid #475569;
  border-radius: 3px;
  font-family: inherit;
}
.diag-form button {
  padding: 5px 0;
  background: #2563eb;
  color: #fff;
  border: none;
  border-radius: 3px;
  cursor: pointer;
}
.diag-form button:disabled {
  background: #475569;
  cursor: not-allowed;
}
.diag-form .err {
  color: #f87171;
  font-size: 11px;
}
.diag-section {
  margin-bottom: 10px;
}
.diag-label {
  color: #fbbf24;
  font-weight: 600;
  margin-bottom: 2px;
}
.diag-sub {
  color: #94a3b8;
  font-size: 11px;
  padding-left: 8px;
}
.diag-rule {
  color: #cbd5e1;
  padding-left: 8px;
  line-height: 1.6;
}
.diag-final {
  padding: 6px 8px;
  border-radius: 3px;
  font-weight: 600;
  text-align: center;
}
.diag-final.pass {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}
.diag-final.fail {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}
.diag-note {
  color: #f87171;
}
.diag-mode-switch {
  display: flex;
  gap: 12px;
  padding: 8px 10px 0;
  font-size: 12px;
  color: #cbd5e1;
}
.diag-mode-switch label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
}
.diag-rule.cs-a {
  color: #f87171;
  font-weight: 600;
}
.diag-rule.cs-b {
  color: #fbbf24;
  font-weight: 600;
}
.diag-rule.cs-c {
  color: #38bdf8;
  font-weight: 600;
}
</style>
