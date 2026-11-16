<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { GridLayout, GridItem } from 'grid-layout-plus'
import { api, type Device, type Dividers, type Entity, type Layout, type Widget, type WidgetKind } from '../api'
import Icon from './Icon.vue'
import { connectLive, type LiveFrame } from '../live'
import WidgetForm from './WidgetForm.vue'

const props = defineProps<{ device: Device; kinds: WidgetKind[]; entities: Entity[]; previewVersion: number }>()
const emit = defineEmits<{ saved: [d: Device] }>()

const clone = <T,>(v: T): T => JSON.parse(JSON.stringify(v))

// Local working copy; every change is saved (debounced) and the server
// re-renders the preview, which is the exact frame the display gets.
const layout = ref<Layout>(clone(props.device.layout))
const selectedId = ref<string>()
const saveState = ref<'saved' | 'saving' | 'error'>('saved')
const notice = ref('')
const saveError = ref('')

const kindByName = computed(() => Object.fromEntries(props.kinds.map((k) => [k.name, k])))
const widgetById = computed(() => Object.fromEntries(layout.value.widgets.map((w) => [w.id, w])))
const selectedWidget = computed(() => layout.value.widgets.find((w) => w.id === selectedId.value))
const selectedKind = computed(() => (selectedWidget.value ? kindByName.value[selectedWidget.value.kind] : undefined))

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

// ---- Panel size ----------------------------------------------------------
// The preview is never shown larger than the panel's own pixels, so at full
// width one panel pixel is one CSS pixel and the image stays crisp.
const frameEl = ref<HTMLElement>()
const width = ref(props.device.panel.width)
const aspect = computed(() => props.device.panel.height / props.device.panel.width)
const rowHeight = computed(() => (width.value * aspect.value) / layout.value.rows)
const colWidth = computed(() => width.value / layout.value.cols)
const exactPixels = computed(() => Math.abs(width.value - props.device.panel.width) < 0.5)
const scalePercent = computed(() => Math.round((width.value / props.device.panel.width) * 100))
// A scaled 1-bit image turns grey and soft in the browser, which the panel
// never does. "Actual size" shows one panel pixel per CSS pixel and lets
// the stage scroll instead.
const ACTUAL_KEY = 'homepaper.actualSize'
const actualSize = ref(readActual())
function readActual() {
  try {
    return localStorage.getItem(ACTUAL_KEY) === '1'
  } catch {
    return false
  }
}
watch(actualSize, (v) => {
  try {
    if (v) localStorage.setItem(ACTUAL_KEY, '1')
    else localStorage.removeItem(ACTUAL_KEY)
  } catch {
    /* private mode: the toggle still works for this page */
  }
})
let ro: ResizeObserver | undefined
onMounted(() => {
  ro = new ResizeObserver(([e]) => (width.value = e.contentRect.width))
  if (frameEl.value) ro.observe(frameEl.value)
})
onUnmounted(() => ro?.disconnect())

// ---- Live preview ----------------------------------------------------------
// The preview is a virtual display: it gets the same pushed frames as the
// panel (full, then partial diffs), without the panel's rate limit. The PNG
// below is only the fallback while the live connection is down.
const liveCanvas = ref<HTMLCanvasElement>()
const liveReady = ref(false)
const lastFrame = ref<LiveFrame>()
const flashes = ref<(LiveFrame & { id: number })[]>([])
let flashId = 0
let stopLive: (() => void) | undefined
onMounted(() => {
  stopLive = connectLive({
    deviceId: props.device.id,
    canvas: () => liveCanvas.value,
    onFrame: (f) => {
      liveReady.value = true
      lastFrame.value = f
      // Outline the region that changed, the way the panel refreshes it.
      const id = ++flashId
      flashes.value = [...flashes.value, { ...f, id }]
      setTimeout(() => (flashes.value = flashes.value.filter((x) => x.id !== id)), 1200)
    },
    onState: (connected) => {
      if (!connected) liveReady.value = false
    },
  })
})
onUnmounted(() => stopLive?.())

function pct(n: number, of: number) {
  return `${(n / of) * 100}%`
}

// ---- Preview -------------------------------------------------------------
// Load the new image off-screen and swap, so it never flashes blank.
const previewSrc = ref(api.previewUrl(props.device.id, props.previewVersion))
let previewTick = 0
function reloadPreview() {
  if (liveReady.value) return
  const url = api.previewUrl(props.device.id, `${props.previewVersion}.${++previewTick}`)
  const img = new Image()
  img.onload = () => (previewSrc.value = url)
  img.src = url
}
watch(() => props.previewVersion, reloadPreview)

// A clock widget changes with the minute, not with any entity.
let minuteTimer: number | undefined
function scheduleMinute() {
  minuteTimer = window.setTimeout(
    () => {
      if (layout.value.widgets.some((w) => w.kind === 'clock')) reloadPreview()
      scheduleMinute()
    },
    60_000 - (Date.now() % 60_000) + 500,
  )
}
onMounted(scheduleMinute)
onUnmounted(() => clearTimeout(minuteTimer))

// ---- Saving --------------------------------------------------------------
let saveTimer: number | undefined
function scheduleSave() {
  saveState.value = 'saving'
  clearTimeout(saveTimer)
  saveTimer = window.setTimeout(save, 350)
}
async function save() {
  clearTimeout(saveTimer)
  saveTimer = undefined
  const sent = clone(layout.value)
  try {
    const d = await api.saveLayout(props.device.id, sent)
    if (!saveTimer) saveState.value = 'saved'
    saveError.value = ''
    emit('saved', { ...d, layout: sent })
  } catch (e) {
    saveState.value = 'error'
    saveError.value = (e as Error).message
  }
}
// Don't lose the last edit when the editor closes within the debounce.
onUnmounted(() => {
  if (saveTimer) save()
})

// ---- Undo / redo ---------------------------------------------------------
// Snapshots of the whole layout: it is small, and restoring one can't leave
// the grid and the options out of step.
interface Snapshot {
  layout: Layout
  selected?: string
}
const undoStack = ref<Snapshot[]>([])
const redoStack = ref<Snapshot[]>([])
const formVersion = ref(0) // rebuilds the form after undo/redo
let lastKey = ''
let lastAt = 0

// Call before changing the layout. Changes with the same key in quick
// succession (typing in one field) become a single undo step.
function record(key = '') {
  const now = Date.now()
  if (key && key === lastKey && now - lastAt < 1500) {
    lastAt = now
    return
  }
  lastKey = key
  lastAt = now
  undoStack.value.push({ layout: clone(layout.value), selected: selectedId.value })
  if (undoStack.value.length > 100) undoStack.value.shift()
  redoStack.value = []
}

function restore(from: Snapshot[], to: Snapshot[]) {
  const snap = from.pop()
  if (!snap) return
  to.push({ layout: clone(layout.value), selected: selectedId.value })
  layout.value = snap.layout
  selectedId.value = snap.layout.widgets.some((w) => w.id === snap.selected) ? snap.selected : undefined
  gridItems.value = toItems()
  formVersion.value++
  lastKey = ''
  notice.value = ''
  scheduleSave()
}
const undo = () => restore(undoStack.value, redoStack.value)
const redo = () => restore(redoStack.value, undoStack.value)

// ---- Moving and resizing -------------------------------------------------
const dragging = ref(false)
// grid-layout-plus only emits moved/resized when the position changed, so a
// widget dropped where it started would leave the grid showing. Releasing
// the pointer always ends the drag.
const endDrag = () => (dragging.value = false)
onMounted(() => {
  window.addEventListener('pointerup', endDrag)
  window.addEventListener('pointercancel', endDrag)
})
onUnmounted(() => {
  window.removeEventListener('pointerup', endDrag)
  window.removeEventListener('pointercancel', endDrag)
})

function onLayoutUpdated() {
  const moved = gridItems.value.filter((it) => {
    const w = widgetById.value[it.i]
    return w && (w.x !== it.x || w.y !== it.y || w.w !== it.w || w.h !== it.h)
  })
  if (!moved.length) return
  record()
  for (const it of moved) Object.assign(widgetById.value[it.i], { x: it.x, y: it.y, w: it.w, h: it.h })
  scheduleSave()
}

function fits(x: number, y: number, w: number, h: number, ignore?: string) {
  if (x < 0 || y < 0 || x + w > layout.value.cols || y + h > layout.value.rows) return false
  return !layout.value.widgets.some(
    (o) => o.id !== ignore && x < o.x + o.w && x + w > o.x && y < o.y + o.h && y + h > o.y,
  )
}

function freeSpot(w: number, h: number) {
  for (let y = 0; y + h <= layout.value.rows; y++)
    for (let x = 0; x + w <= layout.value.cols; x++) if (fits(x, y, w, h)) return { x, y }
}

// Arrow keys move the selected widget by one cell, with Shift they resize it.
function nudge(dx: number, dy: number, resize: boolean) {
  const w = selectedWidget.value
  const kind = selectedKind.value
  if (!w || !kind) return
  const next = resize
    ? { x: w.x, y: w.y, w: Math.max(kind.min_w, w.w + dx), h: Math.max(kind.min_h, w.h + dy) }
    : { x: w.x + dx, y: w.y + dy, w: w.w, h: w.h }
  if ((next.x === w.x && next.y === w.y && next.w === w.w && next.h === w.h) || !fits(next.x, next.y, next.w, next.h, w.id))
    return
  record(`nudge:${w.id}:${resize}`)
  Object.assign(w, next)
  gridItems.value = toItems()
  scheduleSave()
}

// ---- Adding and removing -------------------------------------------------
let noticeTimer: number | undefined
function say(msg: string) {
  notice.value = msg
  clearTimeout(noticeTimer)
  noticeTimer = window.setTimeout(() => (notice.value = ''), 6000)
}

const newId = () => 'w' + Math.random().toString(36).slice(2, 8)

function place(kind: WidgetKind, options: Record<string, unknown>, w: number, h: number) {
  let spot = freeSpot(w, h)
  if (!spot) {
    ;[w, h] = [kind.min_w, kind.min_h]
    spot = freeSpot(w, h)
  }
  if (!spot) {
    say(`There's no room for a ${kind.label.toLowerCase()} widget. Shrink or remove another widget first.`)
    return
  }
  record()
  const id = newId()
  layout.value.widgets.push({ id, kind: kind.name, ...spot, w, h, options })
  gridItems.value = toItems()
  selectedId.value = id
  notice.value = ''
  scheduleSave()
}

function addWidget(kind: WidgetKind) {
  const options: Record<string, unknown> = {}
  for (const f of kind.fields) if (f.default !== undefined) options[f.key] = f.default
  if (kind.name === 'text') options.text = 'Text'
  place(kind, options, kind.default_w, kind.default_h)
}

function duplicate() {
  const w = selectedWidget.value
  if (w && selectedKind.value) place(selectedKind.value, clone(w.options), w.w, w.h)
}

function removeWidget(id: string) {
  record()
  layout.value.widgets = layout.value.widgets.filter((w) => w.id !== id)
  gridItems.value = toItems()
  selectedId.value = undefined
  say('Widget removed.')
  scheduleSave()
}

// Rules or frames between widgets: one setting for the whole layout (D-015).
const dividers = computed<Dividers>(() => layout.value.dividers || 'rules')
function setDividers(v: Dividers) {
  if (v === dividers.value) return
  record()
  layout.value.dividers = v
  scheduleSave()
}

function updateOptions(w: Widget, options: Record<string, unknown>, key: string) {
  record(`opt:${w.id}:${key}`)
  w.options = options
  scheduleSave()
}

// ---- What still needs attention -----------------------------------------
// A widget without its entity draws a dash on the panel. Say so here, where
// it can be fixed, instead of leaving the user to wonder about the dash.
const known = computed(() => new Set(props.entities.map((e) => e.id)))
const problems = computed(() => {
  const out: Record<string, string> = {}
  if (!props.entities.length) return out
  for (const w of layout.value.widgets) {
    for (const f of kindByName.value[w.kind]?.fields ?? []) {
      if (f.type !== 'entity' && f.type !== 'entities') continue
      const ids = String(w.options[f.key] ?? '')
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      if (f.required && !ids.length) out[w.id] ??= 'Choose an entity'
      else if (ids.some((id) => !known.value.has(id))) out[w.id] ??= 'Entity not found'
    }
  }
  return out
})

// ---- Keyboard ------------------------------------------------------------
function onKey(e: KeyboardEvent) {
  const t = e.target as HTMLElement
  if (t.closest('input, select, textarea, dialog, [role="listbox"]')) return
  const mod = e.metaKey || e.ctrlKey
  const key = e.key.toLowerCase()
  if (mod && key === 'z') {
    e.preventDefault()
    if (e.shiftKey) redo()
    else undo()
  } else if (mod && key === 'y') {
    e.preventDefault()
    redo()
  } else if (mod && key === 'd' && selectedId.value) {
    e.preventDefault()
    duplicate()
  } else if ((e.key === 'Delete' || e.key === 'Backspace') && selectedId.value) {
    e.preventDefault()
    removeWidget(selectedId.value)
  } else if (e.key === 'Escape') {
    selectedId.value = undefined
  } else if (e.key.startsWith('Arrow') && selectedId.value && !mod) {
    e.preventDefault()
    const d = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[e.key]!
    nudge(d[0], d[1], e.shiftKey)
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))

function onFramePointerDown(e: PointerEvent) {
  if (!(e.target as HTMLElement).closest('.cell')) selectedId.value = undefined
}

const isMac = navigator.platform.toLowerCase().includes('mac')
const modKey = isMac ? '⌘' : 'Ctrl'
</script>

<template>
  <section class="editor">
    <div class="stage">
      <div class="bar">
        <div class="group">
          <button class="small" :disabled="!undoStack.length" :title="`Undo (${modKey}+Z)`" @click="undo">
            <Icon name="undo" :size="14" :stroke="3" />
            Undo
          </button>
          <button class="small" :disabled="!redoStack.length" :title="`Redo (${modKey}+Shift+Z)`" @click="redo">
            <Icon name="redo" :size="14" :stroke="3" />
            Redo
          </button>
        </div>
        <div class="group" role="group" aria-label="Between widgets">
          <span class="small muted">Dividers</span>
          <div class="seg">
            <button
              class="small"
              :aria-pressed="dividers === 'rules'"
              :class="{ on: dividers === 'rules' }"
              title="2 px rules in the gaps between widgets"
              @click="setDividers('rules')"
            >
              Rules
            </button>
            <button
              class="small"
              :aria-pressed="dividers === 'frames'"
              :class="{ on: dividers === 'frames' }"
              title="A rounded frame around every widget"
              @click="setDividers('frames')"
            >
              Frames
            </button>
          </div>
        </div>
        <div class="group">
          <span v-if="!exactPixels && !actualSize" class="small muted">Shown at {{ scalePercent }} %</span>
          <button
            v-if="!exactPixels || actualSize"
            class="small"
            :aria-pressed="actualSize"
            title="Show one panel pixel per screen pixel"
            @click="actualSize = !actualSize"
          >
            <Icon name="actual" :size="14" />
            {{ actualSize ? 'Fit to window' : 'Actual size' }}
          </button>
        </div>
        <span class="save small" :class="saveState" role="status">
          <template v-if="saveState === 'error'">
            <Icon name="warn" :size="14" />
            Not saved: {{ saveError }}
            <button class="small" @click="save">Try again</button>
          </template>
          <template v-else-if="saveState === 'saving'"><span class="spin"></span> Saving…</template>
          <template v-else><Icon name="check" :size="14" :stroke="3" /> All changes saved</template>
        </span>
      </div>

      <div class="scroller" :class="{ actual: actualSize }">
        <div
          class="bezel"
          :style="actualSize ? { width: `${device.panel.width + 28}px` } : { maxWidth: `${device.panel.width + 28}px` }"
        >
          <div
            ref="frameEl"
            class="frame"
            :class="{ dragging }"
            :style="{
              aspectRatio: `${device.panel.width} / ${device.panel.height}`,
              '--col': `${colWidth}px`,
              '--row': `${rowHeight}px`,
            }"
            @pointerdown="onFramePointerDown"
          >
            <img
              v-show="!liveReady"
              class="preview"
              :class="{ exact: exactPixels }"
              :src="previewSrc"
              alt="The frame as the display shows it"
              draggable="false"
            />
            <canvas
              v-show="liveReady"
              ref="liveCanvas"
              class="preview"
              :class="{ exact: exactPixels }"
              role="img"
              aria-label="The frame as the display shows it, updated live"
            ></canvas>
            <div
              v-for="f in flashes"
              :key="f.id"
              class="flash"
              aria-hidden="true"
              :style="{
                left: pct(f.x, device.panel.width),
                top: pct(f.y, device.panel.height),
                width: pct(f.w, device.panel.width),
                height: pct(f.h, device.panel.height),
              }"
            ></div>
            <div class="guides" aria-hidden="true"></div>
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
                :class="{ selected: item.i === selectedId, problem: problems[item.i] }"
                @pointerdown="selectedId = String(item.i)"
                @move="dragging = true"
                @resize="dragging = true"
                @moved="dragging = false"
                @resized="dragging = false"
              >
                <span class="tag">{{ kindByName[widgetById[item.i]?.kind ?? '']?.label }}</span>
                <span v-if="problems[item.i]" class="flag">
                  <Icon name="warn" :size="12" :stroke="3" />
                  {{ problems[item.i] }}
                </span>
              </GridItem>
            </GridLayout>
            <div v-if="!layout.widgets.length" class="blank">
              <p class="blank-title">This display is empty.</p>
              <p class="small">Add a widget from the list to start the layout.</p>
            </div>
          </div>
        </div>
      </div>

      <p class="live small" role="status">
        <template v-if="liveReady && lastFrame">
          <span class="live-dot" aria-hidden="true"></span>
          Live · frame {{ lastFrame.seq }} {{ lastFrame.kind }} {{ lastFrame.w }} × {{ lastFrame.h }} px ·
          {{ (lastFrame.bytes / 1024).toFixed(1) }} KB · {{ lastFrame.at.toLocaleTimeString() }}
        </template>
        <template v-else><span class="spin"></span> Connecting the live preview…</template>
      </p>

      <p v-if="notice" class="toast" role="status">
        <span>{{ notice }}</span>
        <button v-if="notice === 'Widget removed.'" class="small" @click="undo">Undo</button>
      </p>
      <p class="hint small muted">
        Drag a widget to move it, drag its corner to resize. With a widget selected:
        <kbd>←</kbd> <kbd>→</kbd> <kbd>↑</kbd> <kbd>↓</kbd> move, <kbd>Shift</kbd> + arrows resize,
        <kbd>{{ modKey }}</kbd> <kbd>D</kbd> duplicates, <kbd>Del</kbd> removes, <kbd>Esc</kbd> deselects.
      </p>
    </div>

    <aside class="inspector" aria-label="Inspector">
      <section v-if="selectedWidget && selectedKind" class="card pane">
        <div class="pane-head">
          <div class="who">
            <span class="kind-icon solid"><Icon :name="selectedKind.name" :size="20" /></span>
            <div>
              <h3>{{ selectedKind.label }}</h3>
              <p class="mono muted">
                {{ selectedWidget.w }} × {{ selectedWidget.h }} cells · column {{ selectedWidget.x + 1 }}, row
                {{ selectedWidget.y + 1 }}
              </p>
            </div>
          </div>
          <button class="quiet small" title="Deselect (Esc)" @click="selectedId = undefined">Done</button>
        </div>
        <div class="pane-body">
          <p v-if="problems[selectedWidget.id]" class="notice bad" role="status">
            <Icon name="warn" :size="18" />
            {{
              problems[selectedWidget.id] === 'Choose an entity'
                ? 'Choose what this widget shows. Until then the display draws a dash.'
                : 'An entity this widget uses is not in Home Assistant right now, so the display draws a dash.'
            }}
          </p>
          <WidgetForm
            :key="selectedWidget.id + ':' + formVersion"
            :kind="selectedKind"
            :options="selectedWidget.options"
            :entities="entities"
            @update="(o, key) => updateOptions(selectedWidget!, o, key)"
          />
        </div>
        <div class="pane-actions">
          <button class="small" :title="`Duplicate (${modKey}+D)`" @click="duplicate">
            <Icon name="copy" :size="14" />
            Duplicate
          </button>
          <button class="small danger" title="Remove (Del)" @click="removeWidget(selectedWidget.id)">
            <Icon name="trash" :size="14" />
            Remove
          </button>
        </div>
      </section>

      <section class="card palette-pane">
        <h3>Add a widget</h3>
        <ul class="palette">
          <li v-for="k in kinds" :key="k.name">
            <button @click="addWidget(k)">
              <span class="kind-icon"><Icon :name="k.name" :size="18" /></span>
              <span class="k-text">
                <span class="k-label">{{ k.label }}</span>
                <span class="k-desc">{{ k.description }}</span>
              </span>
              <span class="k-size">{{ k.default_w }}×{{ k.default_h }}</span>
            </button>
          </li>
        </ul>
      </section>
    </aside>
  </section>
</template>

<style scoped>
.editor {
  display: flex;
  flex-wrap: wrap;
  gap: 28px;
  align-items: flex-start;
}

.stage {
  flex: 999 1 560px;
  min-width: 0;
  max-width: 860px;
}

.bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 16px;
  margin-bottom: 12px;
}

.group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.seg {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  border: 2px solid var(--ink);
  border-radius: var(--radius);
  background: var(--sheet);
}

.seg button {
  min-height: 26px;
  border: 0;
  border-radius: 5px;
}

.seg button.on,
.seg button.on:hover:not(:disabled) {
  background: var(--ink);
  color: var(--on-ink);
}

.save {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--muted);
}

.save.error {
  color: var(--bad);
  font-weight: 500;
}

.scroller.actual {
  overflow-x: auto;
}

/* The panel in its picture frame: a black bezel around the white sheet. */
.bezel {
  padding: 14px;
  border-radius: 12px;
  background: var(--bezel);
}

.frame {
  --vgl-placeholder-bg: var(--blue);
  --vgl-placeholder-opacity: 18%;
  --vgl-resizer-border-color: var(--blue);
  position: relative;
  width: 100%;
  background: #fff;
  user-select: none;
  touch-action: none;
}

.preview {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

.preview.exact {
  image-rendering: pixelated;
}

/* The snap grid, shown only while a widget is moved or resized. */
.guides {
  position: absolute;
  inset: 0;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.12s;
  background-image:
    linear-gradient(to right, rgb(29 78 216 / 0.3) 1px, transparent 1px),
    linear-gradient(to bottom, rgb(29 78 216 / 0.3) 1px, transparent 1px);
  background-size: var(--col) var(--row);
}

.frame.dragging .guides {
  opacity: 1;
}

.frame :deep(.vgl-layout) {
  position: absolute !important;
  inset: 0;
  height: 100% !important;
}

.cell {
  border-radius: 9px;
  outline: 3px solid transparent;
  outline-offset: -2px;
  cursor: move;
  touch-action: none;
}

.cell:hover {
  outline-color: rgb(29 78 216 / 0.45);
}

.cell.selected {
  outline-color: var(--blue);
  background: var(--blue-wash);
}

.cell.problem:not(.selected) {
  outline: 2px dashed var(--bad);
}

.cell :deep(.vgl-item__resizer) {
  opacity: 0;
}

.cell:hover :deep(.vgl-item__resizer),
.cell.selected :deep(.vgl-item__resizer) {
  opacity: 1;
}

.tag,
.flag {
  position: absolute;
  left: 8px;
  max-width: calc(100% - 16px);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  font-weight: 700;
  line-height: 16px;
  padding: 0 6px;
  border-radius: 4px;
  pointer-events: none;
}

/* Editor marks are blue: they never reach the panel. */
.tag {
  top: 8px;
  border: 1.5px solid var(--blue);
  background: #fff;
  color: var(--blue);
  opacity: 0;
}

.cell:hover .tag,
.cell.selected .tag,
.frame.dragging .tag {
  opacity: 1;
}

.cell.selected .tag {
  background: var(--blue);
  color: #fff;
}

.flag {
  bottom: 8px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: var(--bad);
  color: #fff;
}

.blank {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  text-align: center;
  color: #111;
  pointer-events: none;
}

.blank p {
  padding: 0 6px;
  background: #fff;
}

.blank .small {
  color: #55554f;
}

.blank-title {
  font-size: var(--t17);
  font-weight: 700;
}

/* The region a frame refreshed, outlined for a moment. Editor blue: it
   never reaches the panel. */
.flash {
  position: absolute;
  pointer-events: none;
  border: 2px solid var(--blue);
  background: rgb(29 78 216 / 0.1);
  animation: flash 1.2s ease-out forwards;
}

@keyframes flash {
  to {
    opacity: 0;
  }
}

.live {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  color: var(--muted);
  font-family: 'Go Mono', ui-monospace, monospace;
}

.live-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ink);
}

.toast {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 14px;
  padding: 8px 8px 8px 14px;
  border-radius: var(--radius);
  background: var(--ink);
  color: var(--on-ink);
  font-weight: 500;
}

.toast span {
  flex: 1;
}

.toast button {
  border-color: var(--sheet);
}

.hint {
  margin-top: 12px;
  line-height: 2;
}

.inspector {
  flex: 1 1 340px;
  min-width: 0;
  max-width: 460px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.pane-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 16px 14px;
  border-bottom: 2px dotted var(--ink);
}

.who {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.kind-icon {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  flex: none;
  border: 2px solid var(--ink);
  border-radius: 7px;
}

.kind-icon.solid {
  width: 36px;
  height: 36px;
  border-radius: var(--radius);
  background: var(--ink);
  color: var(--on-ink);
}

.pane-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
}

.pane-body .notice {
  display: flex;
  gap: 10px;
}

.pane-body .notice svg {
  flex: none;
  margin-top: 1px;
}

.pane-actions {
  display: flex;
  gap: 8px;
  padding: 14px 16px 16px;
  border-top: 2px dotted var(--ink);
}

.palette-pane {
  padding: 16px 8px 8px;
}

.palette-pane h3 {
  font-size: var(--t17);
  padding: 0 8px 8px;
}

.palette {
  list-style: none;
  padding: 0;
}

.palette button {
  width: 100%;
  min-height: 0;
  justify-content: flex-start;
  gap: 12px;
  padding: 10px 8px;
  border-color: transparent;
  background: transparent;
  text-align: left;
  white-space: normal;
  line-height: 1.35;
}

.palette button:hover:not(:disabled) {
  border-color: var(--ink);
  background: var(--sheet);
  color: var(--ink);
}

.k-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.k-label {
  font-weight: 700;
}

.k-desc {
  font-size: var(--t12);
  font-weight: 400;
  color: var(--muted);
}

.k-size {
  font-family: 'Go Mono', ui-monospace, monospace;
  font-size: var(--t12);
  font-weight: 400;
  color: var(--muted);
}

@media (max-width: 1080px) {
  .hint {
    display: none;
  }
}
</style>
