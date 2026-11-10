//! WebSocket client over the C `esp_websocket_client` component.
//!
//! The safe `esp_idf_svc::ws::client` wrapper is not used because it drops
//! `payload_offset`/`payload_len`, so a 48 KB binary frame (which arrives in
//! many WEBSOCKET_EVENT_DATA chunks) could not be reassembled. Same reasoning
//! as fader-control's `dealer.rs`.
//!
//! Memory: the binary receive buffer is allocated once at full-frame size and
//! never grows; oversized messages are dropped instead of reallocating.

use std::ffi::CString;
use std::sync::mpsc::{sync_channel, Receiver, SyncSender};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::{bail, Result};
use esp_idf_svc::hal::delay::TickType;
use esp_idf_svc::sys::{
    esp_event_base_t, esp_websocket_client_config_t, esp_websocket_client_destroy,
    esp_websocket_client_handle_t, esp_websocket_client_init, esp_websocket_client_is_connected,
    esp_websocket_client_send_text, esp_websocket_client_start, esp_websocket_event_data_t,
    esp_websocket_event_id_t_WEBSOCKET_EVENT_ANY, esp_websocket_event_id_t_WEBSOCKET_EVENT_CONNECTED,
    esp_websocket_event_id_t_WEBSOCKET_EVENT_DATA, esp_websocket_event_id_t_WEBSOCKET_EVENT_DISCONNECTED,
    esp_websocket_event_id_t_WEBSOCKET_EVENT_ERROR, esp_websocket_register_events,
};

const OP_TEXT: u8 = 1;
const OP_BINARY: u8 = 2;
const MAX_TEXT: usize = 2048;
const SEND_TIMEOUT: Duration = Duration::from_secs(5);

#[derive(Debug)]
pub enum Event {
    Connected,
    Disconnected,
    Text(String),
    /// A complete binary message is in the shared buffer (see `Client::binary`).
    Binary,
}

struct Ctx {
    tx: SyncSender<Event>,
    text: Vec<u8>,
    binary: Arc<Mutex<Vec<u8>>>,
    op: u8,
    received: usize,
    skipping: bool,
}

impl Ctx {
    fn on_data(&mut self, d: &esp_websocket_event_data_t) {
        let offset = d.payload_offset.max(0) as usize;
        let total = d.payload_len.max(0) as usize;
        let len = d.data_len.max(0) as usize;

        if offset == 0 {
            // First chunk of a message decides its type. The C client keeps
            // op_code constant across chunks; control frames are filtered here.
            self.op = d.op_code;
            self.received = 0;
            self.skipping = match self.op {
                OP_TEXT => {
                    self.text.clear();
                    total > MAX_TEXT
                }
                OP_BINARY => {
                    let mut b = self.binary.lock().unwrap();
                    b.clear();
                    total > b.capacity()
                }
                _ => true,
            };
            if self.skipping && (self.op == OP_TEXT || self.op == OP_BINARY) {
                log::warn!("dropping oversized message ({total} bytes, op {})", self.op);
            }
        }

        if !self.skipping && len > 0 && !d.data_ptr.is_null() {
            let take = len.min(total.saturating_sub(self.received));
            // SAFETY: the C client guarantees data_ptr is valid for data_len bytes during the callback.
            let chunk = unsafe { std::slice::from_raw_parts(d.data_ptr as *const u8, take) };
            self.received += take;
            match self.op {
                OP_TEXT => self.text.extend_from_slice(chunk),
                OP_BINARY => {
                    let mut b = self.binary.lock().unwrap();
                    // Capacity was checked against `total`, so this never reallocates.
                    b.extend_from_slice(chunk);
                }
                _ => {}
            }
        }

        if offset + len >= total {
            if !self.skipping {
                let ev = match self.op {
                    OP_TEXT => Some(Event::Text(String::from_utf8_lossy(&self.text).into_owned())),
                    OP_BINARY => Some(Event::Binary),
                    _ => None,
                };
                if let Some(ev) = ev {
                    if self.tx.try_send(ev).is_err() {
                        log::warn!("event queue full, message dropped");
                    }
                }
            }
            self.skipping = false;
        }
    }
}

unsafe extern "C" fn on_event(arg: *mut core::ffi::c_void, _base: esp_event_base_t, id: i32, data: *mut core::ffi::c_void) {
    // SAFETY: `arg` is the `Ctx` registered in `Client::connect`, alive until
    // `esp_websocket_client_destroy` (which joins the client task) has run.
    // Events for one client are dispatched from a single task.
    let ctx = unsafe { &mut *(arg as *mut Ctx) };
    if id == esp_websocket_event_id_t_WEBSOCKET_EVENT_CONNECTED {
        let _ = ctx.tx.try_send(Event::Connected);
    } else if id == esp_websocket_event_id_t_WEBSOCKET_EVENT_DISCONNECTED
        || id == esp_websocket_event_id_t_WEBSOCKET_EVENT_ERROR
    {
        let _ = ctx.tx.try_send(Event::Disconnected);
    } else if id == esp_websocket_event_id_t_WEBSOCKET_EVENT_DATA && !data.is_null() {
        // SAFETY: for DATA events the C client passes a valid event struct.
        ctx.on_data(unsafe { &*(data as *const esp_websocket_event_data_t) });
    }
}

pub struct Client {
    handle: esp_websocket_client_handle_t,
    binary: Arc<Mutex<Vec<u8>>>,
    _ctx: Box<Ctx>,
}

// The handle is only used through thread-safe C API calls.
unsafe impl Send for Client {}

impl Client {
    /// Starts the client. The C library reconnects on its own; every
    /// (re)connect is reported as `Event::Connected`.
    pub fn connect(uri: &str, max_binary: usize) -> Result<(Self, Receiver<Event>)> {
        let uri_c = CString::new(uri)?;
        let config = esp_websocket_client_config_t {
            uri: uri_c.as_ptr(),
            buffer_size: 4096,
            task_stack: 6144,
            crt_bundle_attach: Some(esp_idf_svc::sys::esp_crt_bundle_attach),
            reconnect_timeout_ms: 5000,
            network_timeout_ms: 10_000,
            ..Default::default()
        };
        let handle = unsafe { esp_websocket_client_init(&config) };
        drop(uri_c); // copied by esp_websocket_client_init
        if handle.is_null() {
            bail!("esp_websocket_client_init failed (bad URL?)");
        }

        let (tx, rx) = sync_channel(8);
        let binary = Arc::new(Mutex::new(Vec::with_capacity(max_binary)));
        let mut ctx = Box::new(Ctx { tx, text: Vec::with_capacity(MAX_TEXT), binary: binary.clone(), op: 0, received: 0, skipping: false });
        let ctx_ptr: *mut Ctx = ctx.as_mut();

        let rc = unsafe {
            esp_websocket_register_events(handle, esp_websocket_event_id_t_WEBSOCKET_EVENT_ANY, Some(on_event), ctx_ptr as *mut _)
        };
        if rc != 0 {
            unsafe { esp_websocket_client_destroy(handle) };
            bail!("esp_websocket_register_events failed ({rc})");
        }
        let rc = unsafe { esp_websocket_client_start(handle) };
        if rc != 0 {
            unsafe { esp_websocket_client_destroy(handle) };
            bail!("esp_websocket_client_start failed ({rc})");
        }
        Ok((Self { handle, binary, _ctx: ctx }, rx))
    }

    pub fn send_text(&self, text: &str) -> Result<()> {
        let timeout: TickType = SEND_TIMEOUT.into();
        let rc = unsafe {
            esp_websocket_client_send_text(self.handle, text.as_ptr() as *const core::ffi::c_char, text.len() as i32, timeout.0)
        };
        if rc < 0 {
            bail!("send_text failed ({rc})");
        }
        Ok(())
    }

    /// Runs `f` on the last complete binary message.
    pub fn with_binary<R>(&self, f: impl FnOnce(&[u8]) -> R) -> R {
        f(&self.binary.lock().unwrap())
    }

    pub fn is_connected(&self) -> bool {
        unsafe { esp_websocket_client_is_connected(self.handle) }
    }
}

impl Drop for Client {
    fn drop(&mut self) {
        unsafe { esp_websocket_client_destroy(self.handle) };
    }
}
