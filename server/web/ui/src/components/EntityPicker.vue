<script setup lang="ts">
// A searchable entity chooser. People know their entities by name ("Living
// room"), not by ID, so the list leads with the name and the current state
// and matches the search against both name and ID.
import { computed, nextTick, ref, watch } from 'vue'
import type { Entity } from '../api'

const props = defineProps<{
  modelValue: string
  entities: Entity[] // candidates, already filtered by domain
  all: Entity[] // every entity, to describe a value outside the candidates
  placeholder?: string
  clearable?: boolean
  // "add" mode: the field stays empty and each pick is emitted once.
  add?: boolean
  exclude?: string[]
}>()
const emit = defineEmits<{ 'update:modelValue': [id: string] }>()

const MAX_RESULTS = 60

const open = ref(false)
const query = ref('')
const active = ref(0)
const root = ref<HTMLElement>()
const search = ref<HTMLInputElement>()
const list = ref<HTMLElement>()

const current = computed(() => props.all.find((e) => e.id === props.modelValue))

function stateText(e: Entity) {
  return e.unit ? `${e.state} ${e.unit}` : e.state
}

const matches = computed(() => {
  const words = query.value.toLowerCase().split(/\s+/).filter(Boolean)
  const skip = new Set(props.exclude ?? [])
  return props.entities.filter((e) => {
    if (skip.has(e.id)) return false
    const hay = `${e.name} ${e.id}`.toLowerCase()
    return words.every((w) => hay.includes(w))
  })
})
const shown = computed(() => matches.value.slice(0, MAX_RESULTS))

// An ID Home Assistant doesn't list right now can still be typed in, e.g.
// for a device that is switched off.
const custom = computed(() => {
  const q = query.value.trim()
  return /^[a-z_]+\.[a-z0-9_]+$/.test(q) && !props.entities.some((e) => e.id === q) ? q : ''
})
const optionCount = computed(() => shown.value.length + (custom.value ? 1 : 0))

watch(query, () => (active.value = 0))

async function openList() {
  open.value = true
  query.value = ''
  active.value = Math.max(0, shown.value.findIndex((e) => e.id === props.modelValue))
  await nextTick()
  search.value?.focus()
  scrollActive()
}

function close(refocus = false) {
  open.value = false
  if (refocus) root.value?.querySelector<HTMLElement>('.field')?.focus()
}

function pick(id: string) {
  emit('update:modelValue', id)
  close(true)
}

function pickActive() {
  const e = shown.value[active.value]
  if (e) pick(e.id)
  else if (custom.value) pick(custom.value)
}

function move(by: number) {
  if (!optionCount.value) return
  active.value = (active.value + by + optionCount.value) % optionCount.value
  scrollActive()
}

function scrollActive() {
  nextTick(() => list.value?.querySelector('.active')?.scrollIntoView({ block: 'nearest' }))
}

function onFocusOut(e: FocusEvent) {
  if (!root.value?.contains(e.relatedTarget as Node)) close()
}
</script>

<template>
  <div ref="root" class="picker" @focusout="onFocusOut" @keydown.esc.stop="close(true)">
    <div class="closed">
      <button
        type="button"
        class="field"
        :class="{ empty: !modelValue || add, missing: modelValue && !current && !add }"
        aria-haspopup="listbox"
        :aria-expanded="open"
        @click="open ? close() : openList()"
      >
        <template v-if="add || !modelValue">
          <span class="name muted">{{ placeholder ?? 'Choose an entity' }}</span>
        </template>
        <template v-else-if="current">
          <span class="name">{{ current.name }}</span>
          <span class="state">{{ stateText(current) }}</span>
        </template>
        <template v-else>
          <span class="name mono">{{ modelValue }}</span>
          <span class="state">not found</span>
        </template>
      </button>
      <button
        v-if="clearable && modelValue && !add"
        type="button"
        class="quiet clear"
        aria-label="Clear"
        title="Clear"
        @click="emit('update:modelValue', '')"
      >
        ×
      </button>
    </div>
    <p v-if="modelValue && !add && current" class="id mono muted">{{ modelValue }}</p>

    <div v-if="open" class="pop">
      <input
        ref="search"
        v-model="query"
        type="search"
        placeholder="Search by name or ID"
        aria-label="Search entities"
        autocomplete="off"
        spellcheck="false"
        @keydown.down.prevent="move(1)"
        @keydown.up.prevent="move(-1)"
        @keydown.enter.prevent="pickActive"
      />
      <ul ref="list" role="listbox">
        <li
          v-for="(e, i) in shown"
          :key="e.id"
          role="option"
          :aria-selected="e.id === modelValue"
          :class="{ active: i === active, chosen: e.id === modelValue && !add }"
          @mousedown.prevent
          @click="pick(e.id)"
          @mousemove="active = i"
        >
          <span class="row">
            <span class="name">{{ e.name }}</span>
            <span class="state">{{ stateText(e) }}</span>
          </span>
          <span class="mono muted">{{ e.id }}</span>
        </li>
        <li
          v-if="custom"
          role="option"
          aria-selected="false"
          :class="{ active: active === shown.length }"
          @mousedown.prevent
          @click="pick(custom)"
        >
          <span class="row"><span class="name">Use this ID anyway</span></span>
          <span class="mono muted">{{ custom }}</span>
        </li>
      </ul>
      <p v-if="!optionCount" class="none muted small">
        {{ entities.length ? 'No entity matches that search.' : 'Home Assistant has no entities of this type.' }}
      </p>
      <p v-else-if="matches.length > MAX_RESULTS" class="none muted small">
        Showing {{ MAX_RESULTS }} of {{ matches.length }}. Type to narrow it down.
      </p>
    </div>
  </div>
</template>

<style scoped>
.picker {
  position: relative;
}

.closed {
  display: flex;
  gap: 2px;
}

.field {
  flex: 1;
  min-width: 0;
  justify-content: space-between;
  gap: 10px;
  font-weight: 400;
  text-align: left;
}

.field.empty {
  border-style: dashed;
}

.field.missing {
  border-color: var(--warn);
}

.field.missing .state {
  color: var(--warn);
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.state {
  flex: none;
  max-width: 45%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--muted);
  font-size: var(--text-s);
}

.clear {
  width: 30px;
  padding: 0;
  font-size: 18px;
}

.id {
  margin-top: 3px;
  font-size: 12px;
}

.pop {
  position: absolute;
  z-index: 20;
  top: 38px;
  left: 0;
  right: 0;
  padding: 6px;
  border: 1px solid var(--blue);
  border-radius: var(--radius);
  background: var(--sheet);
  box-shadow: 0 8px 24px rgb(10 30 50 / 0.18);
}

ul {
  list-style: none;
  margin: 6px 0 0;
  padding: 0;
  max-height: 264px;
  overflow-y: auto;
}

li {
  display: flex;
  flex-direction: column;
  padding: 5px 8px;
  border-radius: 3px;
  cursor: pointer;
}

li.active {
  background: var(--blue-wash);
}

li.chosen .name {
  font-weight: 700;
}

li .mono {
  font-size: 12px;
}

.row {
  display: flex;
  justify-content: space-between;
  gap: 10px;
}

.none {
  padding: 6px 8px 2px;
}
</style>
