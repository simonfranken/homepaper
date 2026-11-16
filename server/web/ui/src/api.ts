// Typed client for the Go server's REST API and event WebSocket.

export type FieldType = 'entity' | 'entities' | 'text' | 'number' | 'select' | 'bool'

export interface Field {
  key: string
  label: string
  type: FieldType
  help?: string
  placeholder?: string
  domains?: string[]
  choices?: { value: string; label: string }[]
  default?: unknown
  required?: boolean
}

export interface WidgetKind {
  name: string
  label: string
  description: string
  fields: Field[]
  min_w: number
  min_h: number
  default_w: number
  default_h: number
}

export interface Widget {
  id: string
  kind: string
  x: number
  y: number
  w: number
  h: number
  options: Record<string, unknown>
}

export type Dividers = 'rules' | 'frames'

export interface Layout {
  cols: number
  rows: number
  /** How widgets are separated; empty means rules (D-015). */
  dividers?: Dividers
  widgets: Widget[]
}

export interface DeviceStatus {
  connected: boolean
  connected_at?: string
  remote?: string
  firmware?: string
  panel?: string
  frames_sent: number
  bytes_sent: number
  last_kind?: string
  last_seq: number
  last_ack_seq: number
  last_ack_at?: string
  in_sync: boolean
  rssi?: number
  free_heap?: number
  uptime_s?: number
  last_error?: string
}

export interface Device {
  id: string
  name: string
  token: string
  panel: { model: string; width: number; height: number; format: string }
  layout: Layout
  created_at: string
  last_seen?: string
  status: DeviceStatus
}

export interface Entity {
  id: string
  name: string
  state: string
  unit?: string
  domain: string
}

export interface ServerStatus {
  source: 'demo' | 'homeassistant'
  connected: boolean
  error?: string
  entities: number
}

export interface HaSettings {
  url: string
  has_token: boolean
  origin: 'ui' | 'env' | 'none'
  status: ServerStatus
}

async function req<T>(method: string, url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    let msg = res.statusText
    try {
      msg = (await res.json()).error ?? msg
    } catch {
      /* not JSON */
    }
    throw new Error(msg)
  }
  return res.status === 204 ? (undefined as T) : res.json()
}

export const api = {
  status: () => req<ServerStatus>('GET', '/api/status'),
  widgets: () => req<WidgetKind[]>('GET', '/api/widgets'),
  entities: () => req<Entity[]>('GET', '/api/entities'),
  devices: () => req<Device[]>('GET', '/api/devices'),
  device: (id: string) => req<Device>('GET', `/api/devices/${id}`),
  createDevice: (id: string, name: string) => req<Device>('POST', '/api/devices', { id, name }),
  renameDevice: (id: string, name: string) => req<Device>('PATCH', `/api/devices/${id}`, { name }),
  resetToken: (id: string) => req<Device>('PATCH', `/api/devices/${id}`, { reset_token: true }),
  deleteDevice: (id: string) => req<void>('DELETE', `/api/devices/${id}`),
  saveLayout: (id: string, layout: Layout) => req<Device>('PUT', `/api/devices/${id}/layout`, layout),
  haSettings: () => req<HaSettings>('GET', '/api/settings/ha'),
  saveHaSettings: (url: string, token: string) => req<HaSettings>('PUT', '/api/settings/ha', { url, token }),
  previewUrl: (id: string, version: number | string) => `/api/devices/${id}/preview.png?v=${version}`,
}

export type ServerEvent =
  | { type: 'device'; device: string }
  | { type: 'devices' }
  | { type: 'render'; device: string }
  | { type: 'settings' }

// Subscribes to server events, reconnecting with backoff. Returns a stop function.
export function subscribe(
  onEvent: (e: ServerEvent) => void,
  onReconnect: () => void,
  onLost: () => void,
): () => void {
  let ws: WebSocket | undefined
  let stopped = false
  let delay = 1000
  const connect = () => {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    ws = new WebSocket(`${proto}://${location.host}/api/events`)
    ws.onopen = () => {
      delay = 1000
      onReconnect()
    }
    ws.onmessage = (m) => onEvent(JSON.parse(m.data))
    ws.onclose = () => {
      if (stopped) return
      onLost()
      setTimeout(connect, delay)
      delay = Math.min(delay * 2, 15000)
    }
  }
  connect()
  return () => {
    stopped = true
    ws?.close()
  }
}

// The URL a device should be configured with, for a given host[:port].
export function deviceUrl(host: string): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${host}/ws/device`
}

// Addresses that only work on the machine the browser runs on.
export function isLoopback(host: string): boolean {
  const name = host.replace(/:\d+$/, '').toLowerCase()
  return name === 'localhost' || name === '[::1]' || name.startsWith('127.')
}

// The display's setup portal (firmware/src/setup.rs). It accepts the
// connection values as query parameters and fills its form with them.
export const SETUP_PORTAL = 'http://192.168.71.1'
export const SETUP_SSID = 'homepaper-setup'

export function setupLink(serverUrl: string, d: Device): string {
  const q = new URLSearchParams({ server_url: serverUrl, device_id: d.id, token: d.token })
  return `${SETUP_PORTAL}/?${q}`
}

export function hasConnected(d: Device): boolean {
  return d.status.connected || (!!d.last_seen && !d.last_seen.startsWith('0001'))
}
