<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { GridLayout, GridItem } from 'grid-layout-plus'
import { api, type Device, type Entity, type Layout, type Widget, type WidgetKind } from '../api'
import WidgetForm from './WidgetForm.vue'

const props = defineProps<{ device: Device; kinds: WidgetKind[]; entities: Entity[]; previewVersion: number }>()
const emit = defineEmits<{ saved: [d: Device] }>()

// Local working copy; every change is saved (debounced) and the server
// re-renders the preview, which is the exact frame the display gets.
const layout = ref<Layout>(JSON.parse(JSON.stringify(props.device.layout)))
const selectedId = ref<string>()
const saveState = ref<'saved' | 'saving' | 'error'>('saved')
const saveError = ref('')

const kindByName = computed(() => Object.fromEntries(props.kinds.map((k) => [k.name, k])))
const selectedWidget = computed(() => layout.value.widgets.find((w) => w.id === selectedId.value))

const gridItems = ref(toItems())
function toItems() {
  return layout.value.widgets.map((w) => ({
    i: w.id,
    x: w.x,
    y: w.y,
    w: w.w,
    h: w.h,
    minW: kindByName.value[w.kind]?.min_w ?? 1,
    minH: kindByName.value[w.kind]?.min_h ?? 1,
  }))
}

// Container size → row height so the grid matches the panel's aspect ratio.
const frameEl = ref<HTMLElement>()
const width = ref(800)
const aspect = computed(() => props.device.panel.height / props.device.panel.width)
const rowHeight = computed(() => (width.value * aspect.value) / layout.value.rows)
let ro: ResizeObserver | undefined
onMounted(() => {
  ro = new ResizeObserver(([e]) => (width.value = e.contentRect.width))
  if (frameEl.value) ro.observe(frameEl.value)
})
onUnmounted(() => ro?.disconnect())

// Preview: load the new image off-screen and swap, so it never flashes blank.
const previewSrc = ref(api.previewUrl(props.device.id, props.previewVersion))
watch(
  () => props.previewVersion,
  (v) => {
    const url = api.previewUrl(props.device.id, v)
    const img = new Image()
    img.onload = () => (previewSrc.value = url)
    img.src = url
  },
)

let saveTimer: number | undefined
function scheduleSave() {
  saveState.value = 'saving'
  clearTimeout(saveTimer)
  saveTimer = window.setTimeout(save, 400)
}
async function save() {
  try {
    const d = await api.saveLayout(props.device.id, layout.value)
    saveState.value = 'saved'
    saveError.value = ''
    emit('saved', { ...d, layout: layout.value })
  } catch (e) {
    saveState.value = 'error'
    saveError.value = (e as Error).message
  }
}

function onLayoutUpdated() {
  let changed = false
  for (const it of gridItems.value) {
    const w = layout.value.widgets.find((x) => x.id === it.i)
    if (w && (w.x !== it.x || w.y !== it.y || w.w !== it.w || w.h !== it.h)) {
      Object.assign(w, { x: it.x, y: it.y, w: it.w, h: it.h })
      changed = true
    }
  }
  if (changed) scheduleSave()
}

function occupied(x: number, y: number, w: number, h: number) {
  return layout.value.widgets.some((o) => x < o.x + o.w && x + w > o.x && y < o.y + o.h && y + h > o.y)
}

function freeSpot(w: number, h: number) {
  for (let y = 0; y + h <= layout.value.rows; y++)
    for (let x = 0; x + w <= layout.value.cols; x++) if (!occupied(x, y, w, h)) return { x, y }
}

const addError = ref('')
function addWidget(kind: WidgetKind) {
  addError.value = ''
  let w = kind.default_w
  let h = kind.default_h
  let spot = freeSpot(w, h)
  if (!spot) {
    ;[w, h] = [kind.min_w, kind.min_h]
    spot = freeSpot(w, h)
  }
  if (!spot) {
    addError.value = 'No free space on the grid. Make room first.'
    return
  }
  const options: Record<string, unknown> = {}
  for (const f of kind.fields) if (f.default !== undefined) options[f.key] = f.default
  const id = 'w' + Math.random().toString(36).slice(2, 8)
  layout.value.widgets.push({ id, kind: kind.name, ...spot, w, h, options })
  gridItems.value = toItems()
  selectedId.value = id
  scheduleSave()
}

function removeWidget(id: string) {
  layout.value.widgets = layout.value.widgets.filter((w) => w.id !== id)
  gridItems.value = toItems()
  selectedId.value = undefined
  scheduleSave()
}

function updateOptions(w: Widget, options: Record<string, unknown>) {
  w.options = options
  scheduleSave()
}

function onKey(e: KeyboardEvent) {
  const t = e.target as HTMLElement
  if (t.tagName === 'INPUT' || t.tagName === 'SELECT' || t.tagName === 'TEXTAREA') return
  if ((e.key === 'Delete' || e.key === 'Backspace') && selectedId.value) removeWidget(selectedId.value)
  if (e.key === 'Escape') selectedId.value = undefined
}
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <section class="editor">
    <div class="canvas-col">
      <div class="toolbar">
        <h2>{{ device.name }}</h2>
        <span class="save muted" :class="saveState">
          {{ saveState === 'saving' ? 'Saving…' : saveState === 'error' ? 'Not saved: ' + saveError : 'Saved' }}
        </span>
      </div>

      <div ref="frameEl" class="frame" :style="{ aspectRatio: `${device.panel.width} / ${device.panel.height}` }">
        <img class="preview" :src="previewSrc" alt="Rendered display" draggable="false" />
        <GridLayout
          v-model:layout="gridItems"
          :col-num="layout.cols"
          :row-height="rowHeight"
          :max-rows="layout.rows"
          :margin="[0, 0]"
          :vertical-compact="false"
          :prevent-collision="true"
          :is-bounded="true"
          :auto-size="false"
          @layout-updated="onLayoutUpdated"
        >
          <GridItem
            v-for="item in gridItems"
            :key="item.i"
            :i="item.i"
            :x="item.x"
            :y="item.y"
            :w="item.w"
            :h="item.h"
            :min-w="item.minW"
            :min-h="item.minH"
            class="cell"
            :class="{ selected: item.i === selectedId }"
            @mousedown="selectedId = String(item.i)"
          >
            <span class="chip">{{ kindByName[layout.widgets.find((w) => w.id === item.i)?.kind ?? '']?.label }}</span>
          </GridItem>
        </GridLayout>
      </div>
      <p class="muted hint">
        {{ device.panel.width }}×{{ device.panel.height }} · {{ layout.cols }}×{{ layout.rows }} grid · drag to move, drag the
        corner to resize, Delete removes the selected widget. The image is exactly what the display shows.
      </p>
    </div>

    <div class="side">
      <div class="card">
        <h3>Add widget</h3>
        <div class="palette">
          <button v-for="k in kinds" :key="k.name" :title="k.description" @click="addWidget(k)">+ {{ k.label }}</button>
        </div>
        <p v-if="addError" class="error">{{ addError }}</p>
      </div>

      <div v-if="selectedWidget" class="card">
        <div class="props-head">
          <h3>{{ kindByName[selectedWidget.kind]?.label }}</h3>
          <button class="danger" @click="removeWidget(selectedWidget.id)">Remove</button>
        </div>
        <p class="muted desc">{{ kindByName[selectedWidget.kind]?.description }}</p>
        <WidgetForm
          :key="selectedWidget.id"
          :kind="kindByName[selectedWidget.kind]"
          :options="selectedWidget.options"
          :entities="entities"
          @update="updateOptions(selectedWidget, $event)"
        />
      </div>
      <div v-else class="card muted">Select a widget on the display to edit it.</div>
    </div>
  </section>
</template>

<style scoped>
.editor {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 300px;
  gap: 20px;
  align-items: start;
}

.toolbar {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.save {
  font-size: 13px;
}

.save.error {
  color: var(--bad);
}

.frame {
  --vgl-placeholder-bg: #2f5d50;
  position: relative;
  width: 100%;
  max-width: 1000px;
  background: #fff;
  border: 10px solid #2a2724;
  border-radius: 6px;
  box-shadow: 0 6px 24px rgb(0 0 0 / 0.18);
  box-sizing: content-box;
  user-select: none;
}

.preview {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  image-rendering: pixelated;
  pointer-events: none;
}

.frame :deep(.vgl-layout) {
  position: absolute !important;
  inset: 0;
  height: 100% !important;
}

.cell {
  border: 2px solid transparent;
  border-radius: 6px;
  cursor: move;
  transition: border-color 0.1s;
}

.cell:hover {
  border-color: rgb(47 93 80 / 0.5);
}

.cell.selected {
  border-color: var(--accent);
  background: rgb(47 93 80 / 0.06);
}

.chip {
  position: absolute;
  top: -1px;
  right: -1px;
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 0 4px 0 6px;
  background: var(--accent);
  color: #fff;
  opacity: 0;
}

.cell:hover .chip,
.cell.selected .chip {
  opacity: 1;
}

.hint {
  font-size: 13px;
  margin-top: 10px;
}

.side {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.side h3 {
  font-size: 17px;
  margin-bottom: 10px;
}

.palette {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.palette button {
  font-size: 13px;
  padding: 4px 9px;
}

.props-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.desc {
  font-size: 13px;
  margin: 0 0 12px;
}

@media (max-width: 1100px) {
  .editor {
    grid-template-columns: 1fr;
  }
}
</style>
