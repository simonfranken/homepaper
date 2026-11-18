<script setup lang="ts">
// The display area (D-021): the part of the panel homepaper draws on, so the
// content fills a picture frame's opening whose aspect ratio differs from
// the panel's. The rest of the panel stays white.
import { computed, ref, watch } from 'vue'
import { api, type Device, type Viewport } from '../api'
import Icon from './Icon.vue'

const props = defineProps<{ device: Device }>()
const emit = defineEmits<{ updated: [d: Device]; layout: [] }>()

const pw = computed(() => props.device.panel.width)
const ph = computed(() => props.device.panel.height)
const minW = computed(() => props.device.grid.min_w)
const minH = computed(() => props.device.grid.min_h)

// The form shows what is stored; 0 (the full panel) is shown as the panel's size.
interface Form {
  w: number
  h: number
  x: number
  y: number
  edges: boolean
}
function fromDevice(): Form {
  const v = props.device.viewport ?? {}
  return { w: v.w || pw.value, h: v.h || ph.value, x: v.offset_x ?? 0, y: v.offset_y ?? 0, edges: !!v.edges }
}
const saved = ref(fromDevice())
const form = ref(fromDevice())
watch(
  () => [props.device.viewport, props.device.panel],
  () => {
    const dirtyBefore = dirty.value
    saved.value = fromDevice()
    if (!dirtyBefore) form.value = fromDevice()
  },
  { deep: true },
)
const dirty = computed(() => JSON.stringify(form.value) !== JSON.stringify(saved.value))

// A viewport stored for a larger panel is kept and drawn clamped (D-021).
const storedLarger = computed(() => {
  const v = props.device.viewport ?? {}
  return (v.w ?? 0) > pw.value || (v.h ?? 0) > ph.value
})

// ---- Checks, the same as the server's -------------------------------------
const num = (v: unknown) => (Number.isFinite(Number(v)) ? Math.round(Number(v)) : 0)
const wErr = computed(() =>
  form.value.w < minW.value
    ? `At least ${minW.value} px, one cell.`
    : form.value.w > pw.value
      ? `At most ${pw.value} px, the panel's width.`
      : '',
)
const hErr = computed(() =>
  form.value.h < minH.value
    ? `At least ${minH.value} px, one cell.`
    : form.value.h > ph.value
      ? `At most ${ph.value} px, the panel's height.`
      : '',
)
const W = computed(() => Math.min(Math.max(form.value.w, minW.value), pw.value))
const H = computed(() => Math.min(Math.max(form.value.h, minH.value), ph.value))
const rawX = computed(() => Math.floor((pw.value - W.value) / 2) + form.value.x)
const rawY = computed(() => Math.floor((ph.value - H.value) / 2) + form.value.y)
const past = (n: number, edge: string, fix: string) =>
  `This moves the area ${n} px past the panel's ${edge} edge. Move it less or make it ${fix}.`
const xErr = computed(() =>
  rawX.value + W.value > pw.value
    ? past(rawX.value + W.value - pw.value, 'right', 'narrower')
    : rawX.value < 0
      ? past(-rawX.value, 'left', 'narrower')
      : '',
)
const yErr = computed(() =>
  rawY.value + H.value > ph.value
    ? past(rawY.value + H.value - ph.value, 'bottom', 'lower')
    : rawY.value < 0
      ? past(-rawY.value, 'top', 'lower')
      : '',
)
const oErr = computed(() => xErr.value || yErr.value)
const invalid = computed(() => !!(wErr.value || hErr.value || oErr.value))
const ax = computed(() => Math.min(Math.max(rawX.value, 0), pw.value - W.value))
const ay = computed(() => Math.min(Math.max(rawY.value, 0), ph.value - H.value))

// ---- The grid this size gives ---------------------------------------------
// A preview of the unsaved values. The minimum cell size comes from the
// server (render.MinCellW/H); after Apply the server's own grid and broken
// list take over.
const layout = computed(() => props.device.layout)
const cols = computed(() => Math.min(Math.max(Math.floor(W.value / minW.value), 1), layout.value.cols))
const rows = computed(() => Math.min(Math.max(Math.floor(H.value / minH.value), 1), layout.value.rows))
const lostC = computed(() => layout.value.cols - cols.value)
const lostR = computed(() => layout.value.rows - rows.value)
const brokenN = computed(
  () => layout.value.widgets.filter((w) => w.x + w.w > cols.value || w.y + w.h > rows.value).length,
)
const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`
const gridNote = computed(() => {
  if (!lostC.value && !lostR.value)
    return `${cols.value} × ${rows.value} cells, the same grid as on the full panel.`
  const parts = []
  if (lostC.value) parts.push(plural(lostC.value, 'column', 'columns'))
  if (lostR.value) parts.push(plural(lostR.value, 'row', 'rows'))
  return `${cols.value} × ${rows.value} cells, ${parts.join(' and ')} fewer than the layout's ${layout.value.cols} × ${layout.value.rows}`
})
const gridWhy = computed(() => {
  const why = []
  if (lostC.value) why.push(`below ${(cols.value + 1) * minW.value} px wide a column drops`)
  if (lostR.value) why.push(`below ${(rows.value + 1) * minH.value} px high a row drops`)
  const s = `${why.join(', ')}, so cells never get smaller than ${minW.value} × ${minH.value} px.`
  return s[0].toUpperCase() + s.slice(1)
})

const mm = (px: number) => (props.device.pitch_mm ? `${(px * props.device.pitch_mm).toFixed(1)} mm` : '')
const dir = (v: number, neg: string, pos: string) => (v === 0 ? 'centred' : `${Math.abs(v)} px ${v < 0 ? neg : pos}`)

// ---- The picture ----------------------------------------------------------
const pct = (n: number, of: number) => `${(n / of) * 100}%`
const areaStyle = computed(() => ({
  left: pct(ax.value, pw.value),
  top: pct(ay.value, ph.value),
  width: pct(W.value, pw.value),
  height: pct(H.value, ph.value),
  backgroundSize: `${100 / cols.value}% ${100 / rows.value}%`,
}))
const wantStyle = computed(() => ({
  left: pct(rawX.value, pw.value),
  top: pct(rawY.value, ph.value),
  width: pct(W.value, pw.value),
  height: pct(H.value, ph.value),
}))

// ---- Editing --------------------------------------------------------------
function step(key: 'w' | 'h' | 'x' | 'y', d: number) {
  form.value[key] = num(form.value[key]) + d
  applied.value = false
}
function onInput(key: 'w' | 'h' | 'x' | 'y', e: Event) {
  form.value[key] = num((e.target as HTMLInputElement).value)
  applied.value = false
}
const isFull = computed(() => form.value.w === pw.value && form.value.h === ph.value)
const isCentred = computed(() => form.value.x === 0 && form.value.y === 0)
function fullPanel() {
  Object.assign(form.value, { w: pw.value, h: ph.value, x: 0, y: 0 })
  applied.value = false
}
function centre() {
  Object.assign(form.value, { x: 0, y: 0 })
  applied.value = false
}

const busy = ref(false)
const applied = ref(false)
const error = ref('')
async function apply() {
  if (invalid.value || !dirty.value) return
  busy.value = true
  error.value = ''
  // The full panel is stored as 0, so it follows the panel if that changes.
  const f = form.value
  const v: Viewport = {
    w: f.w === pw.value ? 0 : f.w,
    h: f.h === ph.value ? 0 : f.h,
    offset_x: f.x,
    offset_y: f.y,
    edges: f.edges,
  }
  try {
    const d = await api.setViewport(props.device.id, v)
    saved.value = fromDeviceOf(d)
    form.value = fromDeviceOf(d)
    applied.value = true
    emit('updated', d)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
function fromDeviceOf(d: Device): Form {
  const v = d.viewport ?? {}
  return { w: v.w || d.panel.width, h: v.h || d.panel.height, x: v.offset_x ?? 0, y: v.offset_y ?? 0, edges: !!v.edges }
}
function revert() {
  form.value = { ...saved.value }
  applied.value = false
}
async function useClamped() {
  form.value = { ...form.value, w: Math.min(form.value.w, pw.value), h: Math.min(form.value.h, ph.value), x: 0, y: 0 }
  await apply()
}

const saveNote = computed(() =>
  invalid.value
    ? 'Fix the marked values first.'
    : applied.value
      ? 'Applied. The display redraws completely now.'
      : dirty.value
        ? 'Not applied yet. The display redraws completely (one full refresh) when you apply.'
        : 'This is what the display shows.',
)
</script>

<template>
  <section class="card area-card" aria-labelledby="area-h">
    <div class="head">
      <div>
        <h2 id="area-h">Display area</h2>
        <p class="muted">
          Make the area homepaper draws on smaller when the frame's opening doesn't match the panel. The rest of the
          panel stays white.
        </p>
      </div>
      <p class="mono chip">
        Panel {{ pw }} × {{ ph }} px<template v-if="device.pitch_mm"> · {{ mm(pw) }} × {{ mm(ph) }}</template>
      </p>
    </div>

    <div v-if="storedLarger" class="notice">
      <Icon name="warn" :size="18" />
      <div>
        <p>
          <b>This display now reports a {{ pw }} × {{ ph }} panel.</b> The area of {{ device.viewport?.w || pw }} ×
          {{ device.viewport?.h || ph }} you set doesn't fit, so it's drawn at {{ device.area.w }} × {{ device.area.h }}.
          It stays as you set it, in case the larger panel comes back.
        </p>
        <button class="small" :disabled="busy" @click="useClamped">Use {{ device.area.w }} × {{ device.area.h }}</button>
      </div>
    </div>

    <div class="body">
      <figure>
        <div class="bezel">
          <div
            class="panel hatch"
            :style="{ aspectRatio: `${pw} / ${ph}` }"
            role="img"
            :aria-label="`Panel ${pw} by ${ph} px with a drawn area of ${W} by ${H} px, ${cols} by ${rows} cells`"
          >
            <div class="area" :class="{ edges: form.edges }" :style="areaStyle">
              <span class="grid-label">{{ cols }} × {{ rows }}</span>
            </div>
            <div v-if="oErr" class="want" :style="wantStyle"></div>
          </div>
        </div>
        <figcaption class="small muted">
          <span><span class="key hatch"></span>Margin, stays white</span>
          <span><span class="key cells"></span>Grid cells</span>
          <span v-if="form.edges"><span class="key edge"></span>Edge line on the display</span>
        </figcaption>
      </figure>

      <div class="fields">
        <fieldset>
          <div class="legend-row">
            <legend class="cap">Size</legend>
            <button class="small quiet" :disabled="isFull" @click="fullPanel">Full panel</button>
          </div>
          <div class="pair">
            <div>
              <label for="va-w">Width</label>
              <div class="stepper">
                <button aria-label="8 px narrower" @click="step('w', -8)">−</button>
                <input
                  id="va-w"
                  type="number"
                  inputmode="numeric"
                  :class="{ bad: wErr }"
                  :aria-invalid="!!wErr"
                  :value="form.w"
                  @change="onInput('w', $event)"
                />
                <button aria-label="8 px wider" @click="step('w', 8)">+</button>
              </div>
              <p class="small muted">px<template v-if="device.pitch_mm"> · {{ mm(W) }}</template></p>
              <p v-if="wErr" class="err">{{ wErr }}</p>
            </div>
            <div>
              <label for="va-h">Height</label>
              <div class="stepper">
                <button aria-label="8 px lower" @click="step('h', -8)">−</button>
                <input
                  id="va-h"
                  type="number"
                  inputmode="numeric"
                  :class="{ bad: hErr }"
                  :aria-invalid="!!hErr"
                  :value="form.h"
                  @change="onInput('h', $event)"
                />
                <button aria-label="8 px taller" @click="step('h', 8)">+</button>
              </div>
              <p class="small muted">px<template v-if="device.pitch_mm"> · {{ mm(H) }}</template></p>
              <p v-if="hErr" class="err">{{ hErr }}</p>
            </div>
          </div>
        </fieldset>

        <fieldset>
          <div class="legend-row">
            <legend class="cap">Position from centre</legend>
            <button class="small quiet" :disabled="isCentred" @click="centre">Center</button>
          </div>
          <div class="pair">
            <div>
              <label for="va-x">Horizontal</label>
              <div class="stepper">
                <button aria-label="Move 2 px left" @click="step('x', -2)">←</button>
                <input
                  id="va-x"
                  type="number"
                  inputmode="numeric"
                  :class="{ bad: xErr }"
                  :aria-invalid="!!xErr"
                  :value="form.x"
                  @change="onInput('x', $event)"
                />
                <button aria-label="Move 2 px right" @click="step('x', 2)">→</button>
              </div>
              <p class="small muted">{{ dir(form.x, 'to the left', 'to the right') }}</p>
            </div>
            <div>
              <label for="va-y">Vertical</label>
              <div class="stepper">
                <button aria-label="Move 2 px up" @click="step('y', -2)">↑</button>
                <input
                  id="va-y"
                  type="number"
                  inputmode="numeric"
                  :class="{ bad: yErr }"
                  :aria-invalid="!!yErr"
                  :value="form.y"
                  @change="onInput('y', $event)"
                />
                <button aria-label="Move 2 px down" @click="step('y', 2)">↓</button>
              </div>
              <p class="small muted">{{ dir(form.y, 'up', 'down') }}</p>
            </div>
          </div>
          <p v-if="xErr" class="err">{{ xErr }}</p>
          <p v-if="yErr" class="err">{{ yErr }}</p>
        </fieldset>

        <div v-if="lostC || lostR" class="notice grid-note">
          <p><b>{{ gridNote }}</b></p>
          <p class="small muted">{{ gridWhy }}</p>
          <p v-if="brokenN" class="small">
            <b>{{ plural(brokenN, 'widget', 'widgets') }}</b> no longer fit and aren't drawn. They stay saved.
            <a href="#" @click.prevent="emit('layout')">Fix them in Layout</a>
          </p>
        </div>
        <p v-else class="small muted">{{ gridNote }}</p>
      </div>
    </div>

    <div class="edges-row">
      <button
        class="switch"
        :class="{ on: form.edges }"
        role="switch"
        :aria-checked="form.edges"
        aria-labelledby="va-edges"
        @click="(form.edges = !form.edges), (applied = false)"
      >
        <span></span>
      </button>
      <div>
        <p id="va-edges"><b>Show the area's edges on the display</b></p>
        <p class="small muted">
          Draws a 1 px line around the area, so you can line it up with the frame's opening. Turn it off when it fits.
        </p>
      </div>
    </div>

    <div class="actions">
      <button class="primary" :disabled="invalid || !dirty || busy" @click="apply">Apply to the display</button>
      <button :disabled="!dirty || busy" @click="revert">Undo changes</button>
      <p class="small muted" role="status">{{ saveNote }}</p>
    </div>
    <p v-if="error" class="error box" role="alert">
      <Icon name="warn" :size="18" />
      {{ error }}
    </p>
  </section>
</template>

<style scoped>
.area-card {
  padding: 20px 24px 24px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.head {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.head > div {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 560px;
}

.chip {
  padding: 4px 8px;
  border: 2px solid var(--ink);
  border-radius: 6px;
  word-break: normal;
}

.notice {
  display: flex;
  gap: 10px;
}

.notice svg {
  flex: none;
  margin-top: 1px;
}

.notice > div {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}

.body {
  display: flex;
  flex-wrap: wrap;
  gap: 28px;
  align-items: flex-start;
}

figure {
  margin: 0;
  flex: 1 1 320px;
  min-width: 0;
  max-width: 440px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.bezel {
  padding: 12px;
  border-radius: 12px;
  background: var(--bezel);
}

/* Editor blue hatch: the margin, which stays white on the panel. */
.hatch {
  background-color: #fff;
  background-image: repeating-linear-gradient(45deg, rgb(29 78 216 / 0.32) 0 2px, transparent 2px 7px);
}

.panel {
  position: relative;
  width: 100%;
  overflow: hidden;
}

.area {
  position: absolute;
  background-color: #fff;
  background-image:
    linear-gradient(to right, rgb(29 78 216 / 0.3) 2px, transparent 2px),
    linear-gradient(to bottom, rgb(29 78 216 / 0.3) 2px, transparent 2px);
  box-shadow: 0 0 0 2px var(--blue);
}

.area.edges {
  outline: 1px solid var(--ink);
  outline-offset: -1px;
}

.want {
  position: absolute;
  border: 2px dashed var(--bad);
}

.grid-label {
  position: absolute;
  left: 6px;
  top: 4px;
  padding: 0 4px;
  background: #fff;
  font-size: var(--t12);
  font-weight: 700;
  color: var(--blue);
}

figcaption {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
}

figcaption > span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.key {
  display: inline-block;
  width: 16px;
  height: 12px;
  border: 2px solid var(--blue);
}

.key.cells {
  border-style: dotted;
  background: #fff;
}

.key.edge {
  border: 1px solid var(--ink);
  background: #fff;
}

.fields {
  flex: 1 1 280px;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 18px;
}

fieldset {
  border: 0;
  margin: 0;
  padding: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.legend-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

legend {
  float: left;
  padding: 0;
}

.pair {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.pair .small {
  margin-top: 4px;
}

.stepper {
  display: flex;
}

.stepper button {
  min-width: 40px;
  padding: 0;
  font-size: var(--t17);
}

.stepper button:first-child {
  border-radius: var(--radius) 0 0 var(--radius);
  border-right: 0;
}

.stepper button:last-child {
  border-radius: 0 var(--radius) var(--radius) 0;
  border-left: 0;
}

.stepper input {
  min-width: 0;
  border-radius: 0;
  padding: 0 4px;
  text-align: center;
  font-variant-numeric: tabular-nums;
  appearance: textfield;
  -moz-appearance: textfield;
}

.stepper input::-webkit-inner-spin-button,
.stepper input::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

input.bad {
  border-color: var(--bad);
}

.err {
  margin-top: 6px;
  color: var(--bad);
  font-size: var(--t12);
  font-weight: 500;
}

fieldset > .err {
  margin-top: 0;
}

.grid-note {
  flex-direction: column;
  gap: 4px;
}

.edges-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding-top: 16px;
  border-top: 2px dotted var(--ink);
}

.switch {
  position: relative;
  flex: none;
  width: 44px;
  min-height: 0;
  height: 26px;
  padding: 0;
  border-radius: 13px;
}

.switch span {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--ink);
  transition: left 0.15s;
}

.switch:hover:not(:disabled) {
  background: var(--sheet);
}

.switch.on,
.switch.on:hover:not(:disabled) {
  background: var(--ink);
}

.switch.on span {
  left: 21px;
  background: var(--on-ink);
}

.actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  padding-top: 16px;
  border-top: 2px solid var(--ink);
}

.actions p {
  flex: 1 1 240px;
}
</style>
