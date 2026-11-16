<script setup lang="ts">
// A searchable entity chooser. People know their entities by name ("Living
// room"), not by ID, so the list leads with the name and the current state
// and matches the search against both name and ID.
import { computed, nextTick, ref, watch } from 'vue'
import type { Entity } from '../api'
import Icon from './Icon.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  modelValue: string
  entities: Entity[] // candidates, already filtered by domain
  all: Entity[] // every entity, to describe a value outside the candidates
  placeholder?: string
  clearable?: boolean
  // "add" mode: the field stays empty and each pick is emitted once.
  add?: boolean
  exclude?: string[]
  domains?: string[] // the field's domains, offered as a filter chip
  id?: string
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

// The field's domains are the default; "all domains" widens the search,
// e.g. for a template sensor of an unusual domain.
const allDomains = ref(false)
const pool = computed(() => (allDomains.value ? props.all : props.entities))

const matches = computed(() => {
  const words = query.value.toLowerCase().split(/\s+/).filter(Boolean)
  const skip = new Set(props.exclude ?? [])
  return pool.value.filter((e) => {
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
  return /^[a-z_]+\.[a-z0-9_]+$/.test(q) && !pool.value.some((e) => e.id === q) ? q : ''
})
const optionCount = computed(() => shown.value.length + (custom.value ? 1 : 0))

watch([query, allDomains], () => (active.value = 0))

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
        :id="id"
        type="button"
        class="field"
        :class="{ empty: !modelValue || add, missing: modelValue && !current && !add }"
        aria-haspopup="listbox"
        :aria-expanded="open"
        @click="open ? close() : openList()"
      >
        <span class="who">
          <template v-if="add || !modelValue">
            <span class="name muted">{{ placeholder ?? 'Choose an entity…' }}</span>
          </template>
          <template v-else-if="current">
            <span class="name">{{ current.name }}</span>
            <span class="id mono">{{ modelValue }}</span>
          </template>
          <template v-else>
            <span class="name">{{ modelValue }}</span>
            <span class="id mono">not in Home Assistant right now</span>
          </template>
        </span>
        <span v-if="current && !add" class="state">{{ stateText(current) }}</span>
        <Icon :name="add ? 'plus' : 'chevron'" :size="16" :stroke="3" />
      </button>
      <button
        v-if="clearable && modelValue && !add"
        type="button"
        class="quiet clear"
        aria-label="Clear"
        title="Clear"
        @click="emit('update:modelValue', '')"
      >
        <Icon name="close" :size="14" :stroke="3" />
      </button>
    </div>

    <div v-if="open" class="pop">
      <div class="search">
        <Icon name="search" :size="16" :stroke="3" />
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
      </div>
      <div v-if="domains?.length" class="chips" role="group" aria-label="Domain">
        <button type="button" class="chip" :aria-pressed="!allDomains" :class="{ on: !allDomains }" @mousedown.prevent @click="allDomains = false">
          {{ domains.join(', ') }}
        </button>
        <button type="button" class="chip" :aria-pressed="allDomains" :class="{ on: allDomains }" @mousedown.prevent @click="allDomains = true">
          all domains
        </button>
      </div>
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
          <span class="text">
            <span class="name">{{ e.name }}</span>
            <span class="mono">{{ e.id }}</span>
          </span>
          <span class="state">{{ stateText(e) }}</span>
        </li>
        <li
          v-if="custom"
          role="option"
          aria-selected="false"
          :class="{ active: active === shown.length }"
          @mousedown.prevent
          @click="pick(custom)"
        >
          <span class="text">
            <span class="name">Use this ID anyway</span>
            <span class="mono">{{ custom }}</span>
          </span>
        </li>
      </ul>
      <p v-if="!optionCount" class="foot">
        {{ pool.length ? 'No entity matches that search.' : 'Home Assistant has no entities of this type.' }}
      </p>
      <p v-else class="foot">
        <template v-if="matches.length > MAX_RESULTS">Showing {{ MAX_RESULTS }} of {{ matches.length }}. Type to narrow it down.</template>
        <template v-else>{{ matches.length }} of {{ all.length }} entities · states update live</template>
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
  gap: 4px;
}

.field {
  flex: 1;
  min-width: 0;
  min-height: 48px;
  justify-content: flex-start;
  gap: 12px;
  padding: 6px 12px;
  font-weight: 400;
  text-align: left;
  white-space: normal;
  line-height: 1.3;
}

.field:hover:not(:disabled) {
  background: var(--sheet);
  color: var(--ink);
  box-shadow: inset 0 0 0 1px var(--ink);
}

.field.empty {
  border-style: dashed;
}

.field.missing {
  border-color: var(--bad);
}

.field.missing .id {
  color: var(--bad);
}

.who {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.name {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.field .name.muted {
  font-weight: 400;
}

.id {
  color: var(--muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  word-break: normal;
}

.state {
  flex: none;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--t17);
  font-weight: 700;
}

.clear {
  width: 40px;
  padding: 0;
}

.pop {
  position: absolute;
  z-index: 20;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  border: 2px solid var(--ink);
  border-radius: var(--radius);
  background: var(--sheet);
  outline: 3px solid var(--blue);
  outline-offset: 2px;
  overflow: hidden;
}

.search {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border-bottom: 2px solid var(--ink);
}

.search input {
  flex: 1;
  min-width: 0;
  border: 0;
  border-radius: 0;
  padding: 0;
}

.search input:focus-visible {
  outline: none;
}

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 8px 10px;
  border-bottom: 2px dotted var(--ink);
}

.chip {
  min-height: 28px;
  padding: 0 10px;
  border-radius: 14px;
  font-size: var(--t12);
}

.chip.on {
  background: var(--ink);
  color: var(--on-ink);
}

ul {
  list-style: none;
  margin: 0;
  padding: 4px;
  max-height: 280px;
  overflow-y: auto;
}

li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
}

li .text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

li .mono {
  color: var(--muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  word-break: normal;
}

li .state {
  font-size: var(--t14);
}

li.active {
  background: var(--paper);
}

li.chosen {
  background: var(--ink);
  color: var(--on-ink);
}

li.chosen .mono {
  color: #d6d5ce;
}

.foot {
  padding: 8px 12px;
  border-top: 2px dotted var(--ink);
  font-size: var(--t12);
  color: var(--muted);
}
</style>
