<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Entity, Field, WidgetKind } from '../api'

const props = defineProps<{ kind: WidgetKind; options: Record<string, unknown>; entities: Entity[] }>()
const emit = defineEmits<{ update: [options: Record<string, unknown>] }>()

const values = ref<Record<string, unknown>>({ ...props.options })

function set(key: string, v: unknown) {
  values.value = { ...values.value, [key]: v }
  emit('update', values.value)
}

function str(key: string) {
  const v = values.value[key]
  return v === undefined || v === null ? '' : String(v)
}

function candidates(f: Field) {
  return f.domains?.length ? props.entities.filter((e) => f.domains!.includes(e.domain)) : props.entities
}

function entityHint(id: string) {
  const e = props.entities.find((x) => x.id === id)
  if (!id) return ''
  return e ? `${e.name} · ${e.state}${e.unit ? ' ' + e.unit : ''}` : 'not found'
}

// Multi-entity fields are stored as a comma-separated string.
function list(key: string) {
  return str(key)
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}
const pending = ref<Record<string, string>>({})
function addTo(key: string) {
  const id = (pending.value[key] ?? '').trim()
  if (!id || list(key).includes(id)) return
  set(key, [...list(key), id].join(', '))
  pending.value[key] = ''
}
function removeFrom(key: string, id: string) {
  set(
    key,
    list(key)
      .filter((x) => x !== id)
      .join(', '),
  )
}

const listId = computed(() => (f: Field) => `ents-${props.kind.name}-${f.key}`)
</script>

<template>
  <form class="form" @submit.prevent>
    <div v-for="f in kind.fields" :key="f.key" class="field">
      <template v-if="f.type === 'bool'">
        <label class="check">
          <input
            type="checkbox"
            :checked="values[f.key] === undefined ? Boolean(f.default) : Boolean(values[f.key])"
            @change="set(f.key, ($event.target as HTMLInputElement).checked)"
          />
          {{ f.label }}
        </label>
      </template>

      <template v-else>
        <label>{{ f.label }}<span v-if="f.required"> *</span></label>

        <template v-if="f.type === 'entity'">
          <input
            :value="str(f.key)"
            :list="listId(f)"
            placeholder="Search entity…"
            @change="set(f.key, ($event.target as HTMLInputElement).value.trim())"
          />
          <small class="muted" :class="{ error: entityHint(str(f.key)) === 'not found' }">{{ entityHint(str(f.key)) }}</small>
        </template>

        <template v-else-if="f.type === 'entities'">
          <ul class="chips">
            <li v-for="id in list(f.key)" :key="id">
              <span>{{ entities.find((e) => e.id === id)?.name ?? id }}</span>
              <button type="button" title="Remove" @click="removeFrom(f.key, id)">×</button>
            </li>
          </ul>
          <div class="add-row">
            <input v-model="pending[f.key]" :list="listId(f)" placeholder="Add entity…" @keydown.enter.prevent="addTo(f.key)" />
            <button type="button" @click="addTo(f.key)">Add</button>
          </div>
        </template>

        <select v-else-if="f.type === 'select'" :value="str(f.key) || String(f.default ?? '')" @change="set(f.key, ($event.target as HTMLSelectElement).value)">
          <option v-for="c in f.choices" :key="c" :value="c">{{ c }}</option>
        </select>

        <input
          v-else-if="f.type === 'number'"
          type="number"
          step="1"
          :value="str(f.key) || String(f.default ?? '')"
          @change="set(f.key, Number(($event.target as HTMLInputElement).value))"
        />

        <input v-else :value="str(f.key)" @change="set(f.key, ($event.target as HTMLInputElement).value)" />
      </template>

      <datalist v-if="f.type === 'entity' || f.type === 'entities'" :id="listId(f)">
        <option v-for="e in candidates(f)" :key="e.id" :value="e.id">{{ e.name }}</option>
      </datalist>
    </div>
  </form>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

small {
  display: block;
  margin-top: 3px;
  font-size: 12px;
}

.check {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--ink);
}

.check input {
  width: auto;
}

.chips {
  list-style: none;
  margin: 0 0 6px;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.chips li {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  border: 1px solid var(--rule);
  border-radius: 6px;
  padding: 2px 4px 2px 8px;
}

.chips button {
  padding: 0 8px;
  border: none;
}

.add-row {
  display: flex;
  gap: 6px;
}
</style>
