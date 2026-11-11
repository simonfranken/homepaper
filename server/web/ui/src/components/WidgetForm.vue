<script setup lang="ts">
// The form for one widget, built from the field schema the server sends
// (GET /api/widgets). Every keystroke is emitted, so the preview follows
// while the user types.
import { ref } from 'vue'
import type { Entity, Field, WidgetKind } from '../api'
import EntityPicker from './EntityPicker.vue'

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
function removeFrom(key: string, id: string) {
  set(
    key,
    list(key)
      .filter((x) => x !== id)
      .join(', '),
  )
}
function moveUp(key: string, i: number) {
  const ids = list(key)
  ;[ids[i - 1], ids[i]] = [ids[i], ids[i - 1]]
  set(key, ids.join(', '))
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

      <template v-else>
        <label :for="`f-${f.key}`">{{ f.label }}</label>

        <EntityPicker
          v-if="f.type === 'entity'"
          :model-value="str(f.key)"
          :entities="candidates(f)"
          :all="entities"
          :clearable="!f.required"
          :placeholder="f.required ? 'Choose an entity' : 'None'"
          @update:model-value="set(f.key, $event)"
        />

        <template v-else-if="f.type === 'entities'">
          <ul v-if="list(f.key).length" class="chosen">
            <li v-for="(id, i) in list(f.key)" :key="id" :class="{ missing: !entityName(id) }">
              <span class="name">{{ entityName(id) ?? id }}</span>
              <span v-if="!entityName(id)" class="small">not found</span>
              <button
                v-if="i > 0"
                type="button"
                class="quiet small"
                :aria-label="`Move ${entityName(id) ?? id} up`"
                title="Move up"
                @click="moveUp(f.key, i)"
              >
                ↑
              </button>
              <button
                type="button"
                class="quiet small"
                :aria-label="`Remove ${entityName(id) ?? id}`"
                title="Remove"
                @click="removeFrom(f.key, id)"
              >
                ×
              </button>
            </li>
          </ul>
          <EntityPicker
            model-value=""
            add
            :entities="candidates(f)"
            :all="entities"
            :exclude="list(f.key)"
            :placeholder="list(f.key).length ? 'Add another' : 'Choose an entity'"
            @update:model-value="addTo(f.key, $event)"
          />
        </template>

        <select
          v-else-if="f.type === 'select'"
          :id="`f-${f.key}`"
          :value="str(f.key) || String(f.default ?? '')"
          @change="set(f.key, ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="c in f.choices" :key="c.value" :value="c.value">{{ c.label }}</option>
        </select>

        <input
          v-else-if="f.type === 'number'"
          :id="`f-${f.key}`"
          type="number"
          step="1"
          min="0"
          inputmode="numeric"
          :placeholder="f.placeholder"
          :value="Number(values[f.key] ?? -1) < 0 ? '' : str(f.key)"
          @input="setNumber(f.key, ($event.target as HTMLInputElement).value)"
        />

        <input
          v-else
          :id="`f-${f.key}`"
          :value="str(f.key)"
          :placeholder="f.placeholder"
          autocomplete="off"
          @input="set(f.key, ($event.target as HTMLInputElement).value)"
        />

        <p v-if="f.help" class="help muted small">{{ f.help }}</p>
      </template>
    </div>
  </form>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.help {
  margin-top: 4px;
}

.check {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  font-size: var(--text-m);
  font-weight: 400;
  cursor: pointer;
}

.chosen {
  list-style: none;
  margin: 0 0 6px;
  padding: 0;
  border: 1px solid var(--rule);
  border-radius: var(--radius);
}

.chosen li {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 1px 2px 1px 9px;
}

.chosen li + li {
  border-top: 1px solid var(--rule-soft);
}

.chosen .name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chosen .missing {
  color: var(--warn);
}

.chosen button {
  width: 28px;
  padding: 0;
  font-size: 16px;
}
</style>
