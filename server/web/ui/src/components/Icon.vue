<script setup lang="ts">
// Icons on a 24 px grid with thick strokes, the outline twins of the
// panel's icons (docs/design.md). Decorative unless a label is given.
import { computed } from 'vue'

const props = withDefaults(defineProps<{ name: string; size?: number; stroke?: number; label?: string }>(), {
  size: 16,
  stroke: 2.5,
  label: undefined,
})

// Each entry: stroked paths, and optionally filled shapes.
const icons: Record<string, { d: string; fill?: string }> = {
  undo: { d: 'M9 14 4 9l5-5M4 9h11a5 5 0 0 1 0 10h-3' },
  redo: { d: 'm15 14 5-5-5-5M20 9H9a5 5 0 0 0 0 10h3' },
  plus: { d: 'M12 5v14M5 12h14' },
  check: { d: 'm5 12 5 5 9-10' },
  copy: { d: 'M8 8h12v12H8zM16 8V4H4v12h4' },
  trash: { d: 'M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13' },
  chevron: { d: 'm6 9 6 6 6-6' },
  external: { d: 'M8 16 16 8M9 8h7v7' },
  actual: { d: 'M3 3h18v18H3zM8 9v7M12 9v.5M12 15v.5M16 9v7' },
  search: { d: 'M10.5 4a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13zM15.5 15.5l5 5' },
  close: { d: 'M6 6l12 12M18 6 6 18' },
  warn: { d: 'M12 3 2 21h20zM12 10v4M12 17.5v.5' },
  grip: { d: 'M9 6h.01M9 12h.01M9 18h.01M15 6h.01M15 12h.01M15 18h.01' },
  up: { d: 'm6 15 6-6 6 6' },
  // widget kinds
  value: { d: 'M4 19h16M7 15v-5M12 15V5M17 15v-3' },
  temperature: { d: 'M10 4.5a2 2 0 0 1 4 0V14a4 4 0 1 1-4 0z', fill: 'M12 15.4a1.6 1.6 0 1 0 0 3.2 1.6 1.6 0 0 0 0-3.2z' },
  climate: { d: 'M12 4a8 8 0 1 0 0 16 8 8 0 0 0 0-16zM12 12l4-4', fill: 'M12 10.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3z' },
  weather: {
    d: 'M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.1 2.1M16.6 16.6l2.1 2.1M5.3 18.7l2.1-2.1M16.6 7.4l2.1-2.1',
    fill: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
  },
  lights: { d: 'M9.5 19h5M10.5 22h3', fill: 'M12 3a6 6 0 0 0-4 10.5c.8.8 1 1.5 1 2.5h6c0-1 .2-1.7 1-2.5A6 6 0 0 0 12 3z' },
  binary: { d: 'M7.5 7h9a5 5 0 0 1 0 10h-9a5 5 0 0 1 0-10z', fill: 'M16.5 9.2a2.8 2.8 0 1 0 0 5.6 2.8 2.8 0 0 0 0-5.6z' },
  clock: { d: 'M12 3.5a8.5 8.5 0 1 0 0 17 8.5 8.5 0 0 0 0-17zM12 7v5l3.5 2' },
  text: { d: 'M5 7V4h14v3M12 4v16M8.5 20h7' },
}
const icon = computed(() => icons[props.name] ?? icons.value)
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    :stroke-width="stroke"
    stroke-linecap="round"
    stroke-linejoin="round"
    :role="label ? 'img' : undefined"
    :aria-label="label"
    :aria-hidden="label ? undefined : 'true'"
  >
    <path :d="icon.d" />
    <path v-if="icon.fill" :d="icon.fill" fill="currentColor" stroke="none" />
  </svg>
</template>
