<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'

interface WatchItem {
  code: string
  fullCode: string
  name: string
}

const props = defineProps<{ activeCode: string; codesReady: boolean }>()
const emit = defineEmits<{
  (e: 'select', code: string, name: string): void
}>()

const items = ref<WatchItem[]>([])
const errMsg = ref('')

// 拖拽状态
const dragIdx = ref<number | null>(null)
// 目标位置：每行的"上沿"或"下沿"（视鼠标在行内的位置而定）
const dropIdx = ref<number | null>(null)
const dropPos = ref<'before' | 'after'>('before')

async function reload() {
  try {
    items.value = (await window.go.main.App.GetWatchlist()) ?? []
    errMsg.value = ''
  } catch (e: any) {
    errMsg.value = String(e?.message ?? e)
  }
}

async function remove(code: string, ev: Event) {
  ev.stopPropagation()
  try {
    items.value = (await window.go.main.App.RemoveFromWatchlist(code)) ?? []
  } catch (e: any) {
    errMsg.value = String(e?.message ?? e)
  }
}

function pick(it: WatchItem) {
  emit('select', it.code, it.name)
}

defineExpose({ reload })

watch(() => props.codesReady, (ready) => {
  if (ready) reload()
})

onMounted(reload)

// === 拖拽 ===
function onDragStart(ev: DragEvent, i: number) {
  dragIdx.value = i
  if (ev.dataTransfer) {
    ev.dataTransfer.effectAllowed = 'move'
    // Firefox 必须 setData 才会触发 dragstart
    ev.dataTransfer.setData('text/plain', String(i))
  }
}

function onDragOver(ev: DragEvent, i: number) {
  ev.preventDefault()
  if (dragIdx.value === null) return
  const target = ev.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  const before = ev.clientY - rect.top < rect.height / 2
  dropIdx.value = i
  dropPos.value = before ? 'before' : 'after'
  if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'move'
}

function onDragLeaveList() {
  // 离开整个列表时清掉指示线，但不清 dragIdx（drop 可能还没触发）
  dropIdx.value = null
}

async function onDrop(ev: DragEvent) {
  ev.preventDefault()
  const from = dragIdx.value
  const to = dropIdx.value
  const pos = dropPos.value
  dragIdx.value = null
  dropIdx.value = null
  if (from === null || to === null || from === to) return

  // 计算插入位置：dropPos=before 插到目标之前，after 插到之后
  const arr = items.value.slice()
  const [moved] = arr.splice(from, 1)
  let insert = to + (pos === 'after' ? 1 : 0)
  if (from < to) insert -= 1 // 因为前面已经删除了一个
  arr.splice(insert, 0, moved)

  // 乐观更新 + 持久化
  items.value = arr
  try {
    const codes = arr.map((it) => it.code)
    items.value = (await window.go.main.App.ReorderWatchlist(codes)) ?? arr
  } catch (e: any) {
    errMsg.value = String(e?.message ?? e)
  }
}

function onDragEnd() {
  dragIdx.value = null
  dropIdx.value = null
}
</script>

<template>
  <aside class="watchlist">
    <header class="title">自选股</header>
    <div v-if="errMsg" class="err">{{ errMsg }}</div>
    <div v-if="items.length === 0" class="empty">
      暂无自选股<br />
      <small>在左侧主图按 + 添加</small>
    </div>
    <ul v-else @dragleave="onDragLeaveList" @drop="onDrop">
      <li
        v-for="(it, idx) in items"
        :key="it.code"
        :class="{
          active: it.code === activeCode,
          dragging: dragIdx === idx,
          'drop-before': dropIdx === idx && dropPos === 'before' && dragIdx !== idx,
          'drop-after': dropIdx === idx && dropPos === 'after' && dragIdx !== idx,
        }"
        draggable="true"
        @click="pick(it)"
        @dragstart="onDragStart($event, idx)"
        @dragover="onDragOver($event, idx)"
        @drop="onDrop($event)"
        @dragend="onDragEnd"
      >
        <span class="grip" title="拖动排序">⋮⋮</span>
        <span class="name">{{ it.name || '—' }}</span>
        <span class="code">{{ it.code }}</span>
        <button class="del" title="删除" @click="remove(it.code, $event)">×</button>
      </li>
    </ul>
  </aside>
</template>

<style scoped>
.watchlist {
  width: 200px;
  display: flex;
  flex-direction: column;
  background: #1e293b;
  border-left: 1px solid #334155;
  flex-shrink: 0;
  overflow: hidden;
}
.title {
  padding: 8px 12px;
  font-size: 13px;
  font-weight: 600;
  color: #cbd5e1;
  border-bottom: 1px solid #334155;
}
.empty {
  padding: 20px 12px;
  font-size: 12px;
  color: #64748b;
  text-align: center;
  line-height: 1.6;
}
.err {
  padding: 6px 12px;
  font-size: 12px;
  color: #f87171;
}
ul {
  list-style: none;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  flex: 1;
}
li {
  position: relative;
  display: flex;
  align-items: center;
  padding: 6px 8px 6px 4px;
  font-size: 13px;
  cursor: pointer;
  border-bottom: 1px solid #334155;
  user-select: none;
}
li:hover {
  background: #334155;
}
li.active {
  background: #1e40af;
}
li.dragging {
  opacity: 0.4;
}
/* 蓝色指示线，落在目标行的上沿或下沿 */
li.drop-before::before,
li.drop-after::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  height: 2px;
  background: #38bdf8;
  pointer-events: none;
}
li.drop-before::before { top: -1px; }
li.drop-after::after  { bottom: -1px; }

li .grip {
  width: 14px;
  font-size: 11px;
  color: #64748b;
  cursor: grab;
  text-align: center;
  letter-spacing: -2px;
}
li:active .grip { cursor: grabbing; }
li .name {
  flex: 1;
  color: #fbbf24;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
li.active .name { color: #fff; }
li .code {
  font-family: 'Consolas', monospace;
  color: #94a3b8;
  margin-right: 8px;
}
li .del {
  background: transparent;
  color: #64748b;
  border: none;
  font-size: 16px;
  cursor: pointer;
  padding: 0 4px;
  line-height: 1;
}
li .del:hover {
  color: #ef4444;
}
</style>
