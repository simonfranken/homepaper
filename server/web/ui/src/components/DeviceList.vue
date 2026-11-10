<script setup lang="ts">
import { ref } from 'vue'
import { api, type Device } from '../api'

defineProps<{ devices: Device[]; selected?: string }>()
const emit = defineEmits<{ select: [id: string]; created: [d: Device] }>()

const adding = ref(false)
const id = ref('')
const name = ref('')
const error = ref('')

function slug(s: string) {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 32)
}

async function create() {
  error.value = ''
  try {
    const d = await api.createDevice(id.value || slug(name.value), name.value)
    emit('created', d)
    adding.value = false
    id.value = name.value = ''
  } catch (e) {
    error.value = (e as Error).message
  }
}

function dotClass(d: Device) {
  if (!d.status.connected) return ''
  return d.status.in_sync ? 'ok' : 'warn'
}
</script>

<template>
  <div class="list">
    <h3>Displays</h3>
    <button
      v-for="d in devices"
      :key="d.id"
      class="item"
      :class="{ active: d.id === selected }"
      @click="emit('select', d.id)"
    >
      <span class="dot" :class="dotClass(d)"></span>
      <span class="name">{{ d.name }}</span>
      <span class="muted state">{{ d.status.connected ? 'online' : 'offline' }}</span>
    </button>

    <form v-if="adding" class="add card" @submit.prevent="create">
      <div>
        <label>Name</label>
        <input v-model="name" placeholder="Living room" autofocus />
      </div>
      <div>
        <label>Device ID</label>
        <input v-model="id" :placeholder="slug(name) || 'living-room'" />
      </div>
      <p v-if="error" class="error">{{ error }}</p>
      <div class="row">
        <button type="submit" class="primary">Add</button>
        <button type="button" @click="adding = false">Cancel</button>
      </div>
    </form>
    <button v-else @click="adding = true">+ Add display</button>
  </div>
</template>

<style scoped>
.list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

h3 {
  font-size: 18px;
  margin-bottom: 6px;
}

.item {
  display: flex;
  align-items: center;
  gap: 8px;
  text-align: left;
}

.item.active {
  border-color: var(--ink);
  box-shadow: inset 3px 0 0 var(--ink);
}

.name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.state {
  font-size: 12px;
}

.add {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
}

.row {
  display: flex;
  gap: 8px;
}
</style>
