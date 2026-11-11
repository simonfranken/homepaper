<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { GridLayout, GridItem } from 'grid-layout-plus'
import { api, type Device, type Entity, type Layout, type Widget, type WidgetKind } from '../api'
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
let ro: ResizeObserver | undefined
onMounted(() => {
  ro = new ResizeObserver(([e]) => (width.value = e.contentRect.width))
  if (frameEl.value) ro.observe(frameEl.value)
})
onUnmounted(() => ro?.disconnect())

// ---- Preview -------------------------------------------------------------
// Load the new image off-screen and swap, so it never flashes blank.
const previewSrc = ref(api.previewUrl(props.device.id, props.previewVersion))
let previewTick = 0
function reloadPreview() {
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
        <div class="history">
          <button class="small" :disabled="!undoStack.length" :title="`Undo (${modKey}+Z)`" @click="undo">Undo</button>
          <button class="small" :disabled="!redoStack.length" :title="`Redo (${modKey}+Shift+Z)`" @click="redo">Redo</button>
        </div>
        <span class="save small" :class="saveState" role="status">
          <template v-if="saveState === 'error'">
            Not saved: {{ saveError }}
            <button class="small" @click="save">Try again</button>
          </template>
          <template v-else>{{ saveState === 'saving' ? 'Saving…' : 'All changes saved' }}</template>
        </span>
      </div>

      <div
        class="bezel"
        :style="{ maxWidth: `${device.panel.width + 28}px` }"
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
            class="preview"
            :class="{ exact: exactPixels }"
            :src="previewSrc"
            alt="The frame as the display shows it"
            draggable="false"
          />
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
              <span v-if="problems[item.i]" class="flag">{{ problems[item.i] }}</span>
            </GridItem>
          </GridLayout>
          <div v-if="!layout.widgets.length" class="blank">
            <p>This display is empty.</p>
            <p class="muted small">Add a widget from the list to start the layout.</p>
          </div>
        </div>
      </div>

      <p v-if="notice" class="notice note" role="status">
        {{ notice }}
        <button v-if="notice === 'Widget removed.'" class="small" @click="undo">Undo</button>
      </p>
      <p class="hint muted small">
        Drag a widget to move it, drag its corner to resize. With a widget selected:
        <kbd>←</kbd> <kbd>→</kbd> <kbd>↑</kbd> <kbd>↓</kbd> move, <kbd>Shift</kbd> + arrows resize,
        <kbd>{{ modKey }}</kbd> <kbd>D</kbd> duplicates, <kbd>Del</kbd> removes.
      </p>
    </div>

    <aside class="inspector">
      <div v-if="selectedWidget && selectedKind" class="pane">
        <div class="pane-head">
          <h3>{{ selectedKind.label }}</h3>
          <button class="quiet small" title="Deselect (Esc)" @click="selectedId = undefined">Done</button>
        </div>
        <p v-if="problems[selectedWidget.id]" class="notice">
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
        <div class="pane-actions">
          <button class="small" :title="`Duplicate (${modKey}+D)`" @click="duplicate">Duplicate</button>
          <button class="small danger" title="Remove (Del)" @click="removeWidget(selectedWidget.id)">Remove</button>
        </div>
      </div>

      <div class="pane">
        <h3>Add a widget</h3>
        <ul class="palette">
          <li v-for="k in kinds" :key="k.name">
            <button @click="addWidget(k)">
              <span class="k-label">{{ k.label }}</span>
              <span class="k-desc muted small">{{ k.description }}</span>
            </button>
          </li>
        </ul>
      </div>
    </aside>
  </section>
</template>

<style scoped>
.editor {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 330px;
  gap: 28px;
  align-items: start;
}

.stage {
  min-width: 0;
  max-width: 828px;
}

.bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 28px;
  margin-bottom: 10px;
}

.history {
  display: flex;
  gap: 6px;
}

.save {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--muted);
}

.save.error {
  color: var(--bad);
}

/* The panel is the one object on the page drawn as a thing: a dark bezel
   around a white sheet, lifted off the desk. */
.bezel {
  padding: 14px;
  border-radius: 5px;
  background: var(--bezel);
  box-shadow:
    0 1px 0 rgb(255 255 255 / 0.08) inset,
    0 14px 30px -12px rgb(10 25 35 / 0.45);
}

.frame {
  --vgl-placeholder-bg: var(--edit);
  --vgl-placeholder-opacity: 22%;
  --vgl-resizer-border-color: var(--edit);
  /* The preview is always white, so editor marks use the light-theme blue
     in both themes. */
  --edit: #1269b8;
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

/* Cell guides, shown while a widget moves so the snap grid is visible. */
.guides {
  position: absolute;
  inset: 0;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.12s;
  background-image:
    linear-gradient(to right, rgb(18 105 184 / 0.35) 1px, transparent 1px),
    linear-gradient(to bottom, rgb(18 105 184 / 0.35) 1px, transparent 1px);
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
  border-radius: 3px;
  outline: 2px solid transparent;
  outline-offset: -3px;
  cursor: move;
  touch-action: none;
}

.cell:hover {
  outline-color: rgb(18 105 184 / 0.45);
}

.cell.selected {
  outline-color: var(--edit);
  background: rgb(18 105 184 / 0.07);
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
  font-size: 11px;
  font-weight: 500;
  line-height: 16px;
  padding: 0 6px;
  white-space: nowrap;
  pointer-events: none;
}

.tag {
  top: 3px;
  right: 3px;
  border-radius: 0 2px 0 3px;
  background: var(--edit);
  color: #fff;
  opacity: 0;
}

.cell:hover .tag,
.cell.selected .tag {
  opacity: 1;
}

.flag {
  left: 50%;
  bottom: 8px;
  transform: translateX(-50%);
  max-width: calc(100% - 12px);
  overflow: hidden;
  text-overflow: ellipsis;
  border-radius: 8px;
  background: #fbeccd;
  color: #6b3f00;
  border: 1px solid #c98a1b;
}

.blank {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  color: #14191c;
  pointer-events: none;
}

.blank .muted {
  color: #55616a;
}

.note {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 14px;
}

.hint {
  margin-top: 14px;
  line-height: 1.9;
}

.inspector {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.pane {
  padding: 16px;
  border: 1px solid var(--rule);
  border-radius: 6px;
  background: var(--sheet);
}

.pane > h3 {
  margin-bottom: 8px;
}

.pane-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.pane .notice {
  margin-bottom: 14px;
}

.pane-actions {
  display: flex;
  justify-content: space-between;
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--rule-soft);
}

.palette {
  list-style: none;
  margin: 0 -8px -8px;
  padding: 0;
}

.palette button {
  width: 100%;
  flex-direction: column;
  align-items: flex-start;
  gap: 0;
  padding: 7px 8px;
  border-color: transparent;
  background: none;
  text-align: left;
}

.palette button:hover {
  background: var(--blue-wash);
  border-color: transparent;
}

.k-label::before {
  content: '+ ';
  color: var(--blue);
  font-weight: 700;
}

.k-desc {
  font-weight: 400;
  padding-left: 15px;
}

@media (max-width: 1080px) {
  .editor {
    grid-template-columns: 1fr;
  }

  .hint {
    display: none;
  }
}
</style>
