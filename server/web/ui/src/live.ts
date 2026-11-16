// The editor's live preview: a virtual display that speaks the device
// protocol (docs/device-protocol.md) over /api/devices/{id}/live. It gets
// binary frames pushed exactly as the panel does (full first, then partial
// diffs), applies them to its own framebuffer and acks each one with the
// framebuffer's CRC-32, so the preview shows what the panel would show.
// Unlike the panel it is not rate limited.

export interface LiveFrame {
  seq: number
  kind: 'full' | 'partial'
  x: number
  y: number
  w: number
  h: number
  bytes: number
  at: Date
}

const MAGIC = 0x5048
const HEADER = 16

let crcTable: Uint32Array | undefined
function crc32(buf: Uint8Array): number {
  if (!crcTable) {
    crcTable = new Uint32Array(256)
    for (let n = 0; n < 256; n++) {
      let c = n
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
      crcTable[n] = c >>> 0
    }
  }
  let c = 0xffffffff
  for (let i = 0; i < buf.length; i++) c = crcTable[(c ^ buf[i]) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}

export interface LiveOptions {
  deviceId: string
  canvas: () => HTMLCanvasElement | undefined
  onFrame: (f: LiveFrame) => void
  onState: (connected: boolean) => void
}

// connect opens the live preview and reconnects with backoff until stopped.
export function connectLive(o: LiveOptions): () => void {
  let ws: WebSocket | undefined
  let stopped = false
  let delay = 1000
  let fb: Uint8Array | undefined
  let width = 0
  let height = 0

  const draw = (x: number, y: number, w: number, h: number) => {
    const canvas = o.canvas()
    if (!canvas || !fb) return
    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width
      canvas.height = height
      x = 0
      y = 0
      w = width
      h = height
    }
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    const img = ctx.createImageData(w, h)
    const stride = width / 8
    for (let row = 0; row < h; row++) {
      for (let col = 0; col < w; col++) {
        const px = x + col
        const bit = (fb[(y + row) * stride + (px >> 3)] >> (7 - (px & 7))) & 1
        const v = bit ? 255 : 0 // bit set = white
        const i = (row * w + col) * 4
        img.data[i] = img.data[i + 1] = img.data[i + 2] = v
        img.data[i + 3] = 255
      }
    }
    ctx.putImageData(img, x, y)
  }

  const onFrameData = (buf: ArrayBuffer) => {
    if (buf.byteLength < HEADER) return
    const v = new DataView(buf)
    if (v.getUint16(0, true) !== MAGIC || v.getUint8(2) !== 1) return
    const kind = v.getUint8(3)
    const seq = v.getUint32(4, true)
    const x = v.getUint16(8, true)
    const y = v.getUint16(10, true)
    const w = v.getUint16(12, true)
    const h = v.getUint16(14, true)
    const data = new Uint8Array(buf, HEADER)
    if (kind === 0) {
      // A full frame is the whole panel and sets its size.
      width = w
      height = h
      fb = new Uint8Array((w / 8) * h)
    }
    if (!fb || x % 8 || w % 8 || x + w > width || y + h > height || data.length !== (w / 8) * h) return
    const stride = width / 8
    for (let row = 0; row < h; row++) fb.set(data.subarray(row * (w / 8), (row + 1) * (w / 8)), (y + row) * stride + x / 8)
    draw(x, y, w, h)
    const crc = crc32(fb).toString(16).padStart(8, '0')
    ws?.send(JSON.stringify({ type: 'ack', seq, crc }))
    o.onFrame({ seq, kind: kind === 0 ? 'full' : 'partial', x, y, w, h, bytes: buf.byteLength, at: new Date() })
  }

  const connect = () => {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    ws = new WebSocket(`${proto}://${location.host}/api/devices/${encodeURIComponent(o.deviceId)}/live`)
    ws.binaryType = 'arraybuffer'
    ws.onmessage = (m) => {
      if (m.data instanceof ArrayBuffer) onFrameData(m.data)
      else {
        delay = 1000
        o.onState(true)
      }
    }
    ws.onclose = () => {
      o.onState(false)
      fb = undefined
      if (stopped) return
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
