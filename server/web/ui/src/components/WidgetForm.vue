<script setup lang="ts">
// The form for one widget, built from the field schema the server sends
// (GET /api/widgets). Every keystroke is emitted, so the preview follows
// while the user types.
import { ref } from 'vue'
import type { Entity, Field, WidgetKind } from '../api'
import EntityPicker from './EntityPicker.vue'
import Icon from './Icon.vue'

const props = defineProps<{ kind: WidgetKind; options: Record<string, unknown>; entities: Entity[] }>()
const emit = defineEmits<{ update: [options: Record<string, unknown>, key: string] }>()

const values = ref<Record<string, unknown>>({ ...props.options })

function set(key: string, v: unknown) {
  const next = { ...values.value }
  if (v === undefined || v === '') delete next[key]
  else next[key] = v
  values.value = next
  emit('update', next, key)
}

function str(key: string) {
  const v = values.value[key]
  return v === undefined || v === null ? '' : String(v)
}

function setNumber(key: string, raw: string) {
  const n = Number(raw)
  set(key, raw.trim() === '' || Number.isNaN(n) ? undefined : n)
}

function candidates(f: Field) {
  return f.domains?.length ? props.entities.filter((e) => f.domains!.includes(e.domain)) : props.entities
}

// Multi-entity fields are stored as a comma-separated string.
function list(key: string) {
  return str(key)
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}
function addTo(key: string, id: string) {
  if (id && !list(key).includes(id)) set(key, [...list(key), id].join(', '))
}
function removeFrom(key: string, id: string, labelsKey?: string) {
  if (labelsKey) setLabel(labelsKey, id, '')
  set(
    key,
    list(key)
      .filter((x) => x !== id)
      .join(', '),
  )
}

// Per-entity labels live in one option as "entity_id=Label" lines. Labels of
// entities that were removed are dropped when the label is next written.
function labels(key: string) {
  const m: Record<string, string> = {}
  str(key)
    .split('\n')
    .forEach((line) => {
      const i = line.indexOf('=')
      if (i > 0 && line.slice(i + 1).trim()) m[line.slice(0, i).trim()] = line.slice(i + 1).trim()
    })
  return m
}
function setLabel(key: string, id: string, label: string) {
  const m = labels(key)
  if (label.trim()) m[id] = label
  else delete m[id]
  set(
    key,
    Object.entries(m)
      .map(([k, v]) => `${k}=${v}`)
      .join('\n'),
  )
}
// The label field of one row is open at a time.
const editing = ref<string>()
function toggleEdit(id: string) {
  editing.value = editing.value === id ? undefined : id
}
function moveUp(key: string, i: number) {
  move(key, i, i - 1)
}
function move(key: string, from: number, to: number) {
  const ids = list(key)
  if (from === to || to < 0 || to >= ids.length) return
  const [id] = ids.splice(from, 1)
  ids.splice(to, 0, id)
  set(key, ids.join(', '))
}

// Drag to reorder. The order is the order on the panel.
const dragFrom = ref<{ key: string; i: number }>()
function onDrop(key: string, i: number) {
  if (dragFrom.value?.key === key) move(key, dragFrom.value.i, i)
  dragFrom.value = undefined
}

// Numbers get a stepper; empty means "as reported".
function step(f: Field, by: number) {
  const cur = values.value[f.key]
  const n = typeof cur === 'number' ? cur : by > 0 ? -1 : 0
  const next = n + by
  set(f.key, next < 0 ? undefined : next)
}

// Short choice lists read better as radio cards than as a dropdown.
const asCards = (f: Field) => (f.choices?.length ?? 0) <= 4

// "Date (Monday, 16 November)" → name and example, shown side by side.
function splitChoice(label: string) {
  const m = label.match(/^(.*?)\s*\((.*)\)$/)
  return m ? { name: m[1], example: m[2] } : { name: label, example: '' }
}

function selected(f: Field) {
  return str(f.key) || String(f.default ?? '')
}

function entityName(id: string) {
  return props.entities.find((e) => e.id === id)?.name
}
</script>

<template>
  <form class="form" @submit.prevent>
    <div v-for="f in kind.fields" :key="f.key" class="field">
      <label v-if="f.type === 'bool'" class="check">
        <input
          type="checkbox"
          :checked="values[f.key] === undefined ? Boolean(f.default) : Boolean(values[f.key])"
          @change="set(f.key, ($event.target as HTMLInputElement).checked)"
        />
        {{ f.label }}
      </label>

      <fieldset v-else-if="f.type === 'select' && asCards(f)" class="choices">
        <legend class="lbl">
          <span>{{ f.label }}</span>
        </legend>
        <label
          v-for="c in f.choices"
          :key="c.value"
          class="choice"
          :class="{ on: selected(f) === c.value }"
        >
          <input
            type="radio"
            :name="`f-${f.key}`"
            :value="c.value"
            :checked="selected(f) === c.value"
            @change="set(f.key, c.value)"
          />
          <span class="c-name">{{ splitChoice(c.label).name }}</span>
          <span v-if="splitChoice(c.label).example" class="c-example">{{ splitChoice(c.label).example }}</span>
        </label>
        <p v-if="f.help" class="help">{{ f.help }}</p>
      </fieldset>

      <template v-else>
        <label :for="`f-${f.key}`" class="lbl">
          <span>{{ f.label }}</span>
          <span v-if="f.required" class="req">Required</span>
        </label>

        <EntityPicker
          v-if="f.type === 'entity'"
          :id="`f-${f.key}`"
          :model-value="str(f.key)"
          :entities="candidates(f)"
          :all="entities"
          :domains="f.domains"
          :clearable="!f.required"
          :placeholder="'Choose an entity…'"
          @update:model-value="set(f.key, $event)"
        />

        <template v-else-if="f.type === 'entities'">
          <ul v-if="list(f.key).length" class="chosen">
            <li
              v-for="(id, i) in list(f.key)"
              :key="id"
              :class="{ missing: !entityName(id), over: dragFrom && dragFrom.key === f.key && dragFrom.i !== i }"
              draggable="true"
              @dragstart="dragFrom = { key: f.key, i }"
              @dragend="dragFrom = undefined"
              @dragover.prevent
              @drop.prevent="onDrop(f.key, i)"
            >
              <span class="grip" title="Drag to reorder"><Icon name="grip" :size="16" :stroke="3.5" /></span>
              <span class="name">
                <span class="n">{{ (f.labels_key && labels(f.labels_key)[id]) || entityName(id) || id }}</span>
                <span class="id mono">{{ entityName(id) ? id : 'not found' }}</span>
              </span>
              <span v-if="f.labels_key && labels(f.labels_key)[id]" class="tag">Renamed</span>
              <button
                v-if="f.labels_key && list(f.key).length > 1"
                type="button"
                class="quiet small icon"
                :class="{ on: editing === id }"
                :aria-label="`Edit label for ${entityName(id) ?? id}`"
                :aria-expanded="editing === id"
                title="Edit label"
                @click="toggleEdit(id)"
              >
                <Icon name="edit" :size="14" :stroke="3" />
              </button>
              <button
                v-if="i > 0"
                type="button"
                class="quiet small icon"
                :aria-label="`Move ${entityName(id) ?? id} up`"
                title="Move up"
                @click="moveUp(f.key, i)"
              >
                <Icon name="up" :size="14" :stroke="3" />
              </button>
              <button
                type="button"
                class="quiet small icon"
                :aria-label="`Remove ${entityName(id) ?? id}`"
                title="Remove"
                @click="removeFrom(f.key, id, f.labels_key)"
              >
                <Icon name="close" :size="14" :stroke="3" />
              </button>
              <div v-if="f.labels_key && list(f.key).length > 1 && editing === id" class="rename">
                <label :for="`f-${f.key}-lbl-${i}`">Label on the display</label>
                <div class="row">
                  <input
                    :id="`f-${f.key}-lbl-${i}`"
                    type="text"
                    autocomplete="off"
                    :placeholder="entityName(id) ?? id"
                    :value="labels(f.labels_key)[id] ?? ''"
                    @input="setLabel(f.labels_key, id, ($event.target as HTMLInputElement).value)"
                  />
                  <button
                    v-if="labels(f.labels_key)[id]"
                    type="button"
                    @click="setLabel(f.labels_key, id, '')"
                  >
                    Reset
                  </button>
                </div>
                <p class="help">Empty uses the Home Assistant name. Only the label changes, never the state.</p>
              </div>
            </li>
          </ul>
          <EntityPicker
            :id="`f-${f.key}`"
            model-value=""
            add
            :entities="candidates(f)"
            :all="entities"
            :domains="f.domains"
            :exclude="list(f.key)"
            :placeholder="list(f.key).length ? 'Add another' : 'Choose an entity…'"
            @update:model-value="addTo(f.key, $event)"
          />
          <p class="help">The display lists them in this order. Drag to reorder.</p>
        </template>

        <select
          v-else-if="f.type === 'select'"
          :id="`f-${f.key}`"
          :value="selected(f)"
          @change="set(f.key, ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="c in f.choices" :key="c.value" :value="c.value">{{ c.label }}</option>
        </select>

        <div v-else-if="f.type === 'number'" class="stepper">
          <button type="button" class="quiet" :aria-label="`Less: ${f.label}`" @click="step(f, -1)">−</button>
          <input
            :id="`f-${f.key}`"
            type="text"
            inputmode="numeric"
            :placeholder="f.placeholder"
            :value="Number(values[f.key] ?? -1) < 0 ? '' : str(f.key)"
            @input="setNumber(f.key, ($event.target as HTMLInputElement).value)"
          />
          <button type="button" class="quiet" :aria-label="`More: ${f.label}`" @click="step(f, 1)">+</button>
        </div>

        <input
          v-else
          :id="`f-${f.key}`"
          :value="str(f.key)"
          :placeholder="f.placeholder"
          autocomplete="off"
          @input="set(f.key, ($event.target as HTMLInputElement).value)"
        />

        <p v-if="f.help" class="help">{{ f.help }}</p>
      </template>
    </div>
  </form>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.lbl {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
  width: 100%;
  margin-bottom: 6px;
  padding: 0;
  font-size: var(--t14);
  font-weight: 700;
}

.req {
  font-size: var(--t12);
  font-weight: 400;
  color: var(--muted);
}

.help {
  margin-top: 6px;
  font-size: var(--t12);
  color: var(--muted);
}

.check {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 32px;
  margin: 0;
  font-weight: 500;
  cursor: pointer;
}

.choices {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  border: 0;
}

.choice {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0;
  padding: 10px 12px;
  border: 2px solid var(--ink);
  border-radius: var(--radius);
  font-weight: 400;
  cursor: pointer;
}

.choice.on {
  background: var(--ink);
  color: var(--on-ink);
}

.choice.on input {
  accent-color: var(--on-ink);
}

.choice:focus-within {
  outline: 3px solid var(--blue);
  outline-offset: 2px;
}

.c-name {
  flex: 1;
}

.c-example {
  font-family: 'Go Mono', ui-monospace, monospace;
  font-size: var(--t12);
  color: var(--muted);
}

.choice.on .c-example {
  color: #d6d5ce;
}

.chosen {
  list-style: none;
  margin: 0 0 8px;
  padding: 0;
  border: 2px solid var(--ink);
  border-radius: var(--radius);
  background: var(--sheet);
}

.chosen li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 6px 4px 6px 6px;
}

.chosen li + li {
  border-top: 2px dotted var(--ink);
}

.chosen li.over {
  box-shadow: inset 0 3px 0 var(--blue);
}

.grip {
  display: grid;
  place-items: center;
  width: 20px;
  color: var(--muted);
  cursor: grab;
}

.chosen .name {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.chosen .n {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chosen .id {
  color: var(--muted);
}

.chosen .missing .n,
.chosen .missing .id {
  color: var(--bad);
}

.icon {
  width: 32px;
  padding: 0;
}

.icon.on {
  background: var(--ink);
  color: var(--on-ink);
}

.tag {
  padding: 0 6px;
  border: 2px solid var(--ink);
  border-radius: 6px;
  font-size: var(--t12);
  font-weight: 700;
}

.rename {
  flex-basis: 100%;
  margin: 2px 4px 4px 28px;
  padding: 10px;
  border: 2px dotted var(--ink);
  border-radius: var(--radius);
  background: var(--paper);
}

.rename label {
  display: block;
  margin-bottom: 4px;
  font-size: var(--t12);
  font-weight: 700;
}

.rename .row {
  display: flex;
  gap: 8px;
}

.rename input {
  flex: 1;
  min-width: 0;
}

.stepper {
  display: flex;
  align-items: stretch;
  max-width: 200px;
  border: 2px solid var(--ink);
  border-radius: var(--radius);
  background: var(--sheet);
  overflow: hidden;
}

.stepper button {
  min-width: 40px;
  border: 0;
  border-radius: 0;
  font-size: var(--t17);
}

.stepper button:first-child {
  border-right: 2px solid var(--ink);
}

.stepper button:last-child {
  border-left: 2px solid var(--ink);
}

.stepper input {
  flex: 1;
  min-width: 0;
  border: 0;
  border-radius: 0;
  text-align: center;
  font-family: 'Go Mono', ui-monospace, monospace;
}
</style>
