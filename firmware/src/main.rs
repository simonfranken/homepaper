//! homepaper firmware: connects to Wi-Fi and to the homepaper server, receives
//! prerendered frames over WebSocket and shows them on the panel.
//! Protocol: docs/device-protocol.md.
//!
//! Robustness model for a device on the wall: any unexpected error, or being
//! offline for too long, restarts the chip; a restart redoes Wi-Fi and the
//! server connection from scratch.

mod config;
mod display;
mod protocol;
mod setup;
mod ws;

use std::sync::mpsc::RecvTimeoutError;
use std::time::{Duration, Instant};

use anyhow::Result;
use esp_idf_svc::eventloop::EspSystemEventLoop;
use esp_idf_svc::hal::gpio::{PinDriver, Pull};
use esp_idf_svc::hal::peripherals::Peripherals;
use esp_idf_svc::hal::reset::restart;
use esp_idf_svc::nvs::EspDefaultNvsPartition;
use esp_idf_svc::wifi::{AuthMethod, BlockingWifi, ClientConfiguration, Configuration, EspWifi};
use serde::Deserialize;

use config::{Config, Settings};
use display::{Framebuffer, NullPanel, Panel};

const FW_VERSION: &str = env!("CARGO_PKG_VERSION");
const WIFI_ATTEMPTS: u32 = 5;
const STATUS_INTERVAL: Duration = Duration::from_secs(60);
/// Restart when the server has been unreachable this long (also covers a lost
/// Wi-Fi association, which nothing else re-establishes).
const OFFLINE_RESTART: Duration = Duration::from_secs(5 * 60);
/// A setup portal opened because Wi-Fi failed (not because the device is
/// unconfigured) restarts after this long, e.g. for a router still booting
/// after a power cut.
const FAILED_WIFI_PORTAL_TIMEOUT: Duration = Duration::from_secs(5 * 60);
/// Holding BOOT this long while running enters setup.
const SETUP_HOLD: Duration = Duration::from_secs(3);

fn main() {
    esp_idf_svc::sys::link_patches();
    esp_idf_svc::log::EspLogger::initialize_default();
    log::info!("homepaper firmware {FW_VERSION}");

    if let Err(e) = app() {
        log::error!("Fatal: {e:?}. Restarting in 5 s.");
    }
    std::thread::sleep(Duration::from_secs(5));
    restart();
}

fn app() -> Result<()> {
    // The checksum must match the server's (Go crc32.ChecksumIEEE).
    let check = display::crc32(b"123456789");
    log::info!("CRC-32 self-check: {check:08x} ({})", if check == 0xcbf4_3926 { "ok" } else { "WRONG" });

    let peripherals = Peripherals::take()?;
    let sysloop = EspSystemEventLoop::take()?;
    let nvs = EspDefaultNvsPartition::take()?;

    let mut settings = Settings::new(nvs.clone())?;
    let cfg = settings.load();
    let force_setup = settings.take_force_setup();

    let mut wifi = BlockingWifi::wrap(EspWifi::new(peripherals.modem, sysloop.clone(), Some(nvs.clone()))?, sysloop)?;

    if force_setup || !cfg.is_complete() {
        log::info!("Entering setup ({})", if force_setup { "BOOT button" } else { "not configured" });
        match setup::run(&mut wifi, settings, cfg, None)? {}
    }

    // GPIO0 is a strapping pin: holding it at reset enters the ROM download
    // mode, so setup is requested by holding BOOT *while running* instead.
    let boot = PinDriver::input(peripherals.pins.gpio0, Pull::Up)?;
    std::thread::Builder::new().stack_size(4096).spawn(move || {
        let mut held_since: Option<Instant> = None;
        loop {
            if boot.is_low() {
                let since = *held_since.get_or_insert_with(Instant::now);
                if since.elapsed() >= SETUP_HOLD {
                    log::info!("BOOT held for {}s: restarting into setup", SETUP_HOLD.as_secs());
                    if let Err(e) = config::request_setup(nvs.clone()) {
                        log::error!("Could not save setup request: {e:?}");
                    }
                    restart();
                }
            } else {
                held_since = None;
            }
            std::thread::sleep(Duration::from_millis(100));
        }
    })?;
    log::info!("Hold BOOT for {}s at any time to open the setup portal", SETUP_HOLD.as_secs());

    if let Err(e) = connect_wifi(&mut wifi, &cfg) {
        log::error!("Wi-Fi failed after {WIFI_ATTEMPTS} attempts: {e:?}. Opening setup portal for {} min.", FAILED_WIFI_PORTAL_TIMEOUT.as_secs() / 60);
        match setup::run(&mut wifi, settings, cfg, Some(FAILED_WIFI_PORTAL_TIMEOUT))? {}
    }

    let mut panel = NullPanel;
    run(&cfg, &mut panel)
}

fn connect_wifi(wifi: &mut BlockingWifi<EspWifi<'static>>, cfg: &Config) -> Result<()> {
    let client = ClientConfiguration {
        ssid: cfg.wifi_ssid.as_str().try_into().map_err(|_| anyhow::anyhow!("SSID too long"))?,
        password: cfg.wifi_pass.as_str().try_into().map_err(|_| anyhow::anyhow!("password too long"))?,
        auth_method: if cfg.wifi_pass.is_empty() { AuthMethod::None } else { AuthMethod::WPA2Personal },
        ..Default::default()
    };
    wifi.set_configuration(&Configuration::Client(client))?;
    wifi.start()?;
    let mut last_err = None;
    for attempt in 1..=WIFI_ATTEMPTS {
        log::info!("Connecting to Wi-Fi '{}' (attempt {attempt})", cfg.wifi_ssid);
        match wifi.connect().and_then(|_| wifi.wait_netif_up()) {
            Ok(()) => {
                let ip = wifi.wifi().sta_netif().get_ip_info()?.ip;
                log::info!("Wi-Fi connected, IP {ip}");
                return Ok(());
            }
            Err(e) => {
                log::warn!("Wi-Fi attempt {attempt} failed: {e}");
                last_err = Some(e);
                let _ = wifi.disconnect();
                std::thread::sleep(Duration::from_secs(3));
            }
        }
    }
    Err(last_err.unwrap().into())
}

#[derive(Deserialize)]
struct ServerMsg {
    #[serde(rename = "type")]
    kind: String,
    #[serde(default)]
    message: String,
}

/// Sends a text message, logging instead of failing: a lost message is
/// recovered by the protocol (missing acks time out, hello is resent on
/// reconnect), so it must not take the device down.
fn send(client: &ws::Client, text: &str) {
    if let Err(e) = client.send_text(text) {
        log::warn!("send failed: {e}");
    }
}

fn run(cfg: &Config, panel: &mut dyn Panel) -> Result<()> {
    let mut fb = Framebuffer::new();
    let (client, events) = ws::Client::connect(&cfg.server_url, protocol::HEADER_SIZE + display::FB_SIZE)?;
    log::info!("Connecting to {}", cfg.server_url);

    let hello = serde_json::json!({
        "type": "hello",
        "device_id": cfg.device_id,
        "token": cfg.token,
        "fw_version": FW_VERSION,
        "panel": {"model": panel.model(), "width": display::WIDTH, "height": display::HEIGHT, "format": display::PANEL_FORMAT},
        "capabilities": ["partial"],
    })
    .to_string();

    let started = Instant::now();
    let mut last_status = Instant::now();
    let mut last_online = Instant::now();
    loop {
        match events.recv_timeout(Duration::from_secs(5)) {
            Ok(ws::Event::Connected) => {
                log::info!("Connected to server, sending hello");
                send(&client, &hello);
                send_status(&client, started);
                last_status = Instant::now();
            }
            Ok(ws::Event::Disconnected) => log::warn!("Server connection lost, reconnecting…"),
            Ok(ws::Event::Text(t)) => match serde_json::from_str::<ServerMsg>(&t) {
                Ok(m) if m.kind == "error" => log::error!("Server rejected us: {} (check device ID/token via setup)", m.message),
                Ok(m) => log::info!("Server: {}", m.kind),
                Err(_) => log::warn!("Unparseable server message: {t}"),
            },
            Ok(ws::Event::Binary) => {
                let result = client.with_binary(|buf| -> Result<protocol::Header> {
                    let (h, data) = protocol::parse(buf, display::WIDTH, display::HEIGHT)?;
                    fb.apply(&h.region, data);
                    Ok(h)
                });
                match result {
                    Ok(h) => {
                        if let Err(e) = panel.show(&fb, &h.region, h.kind) {
                            log::error!("Panel refresh failed: {e:?}");
                        }
                        let crc = fb.crc32();
                        log::info!("Frame {} applied, crc {crc:08x}", h.seq);
                        send(&client, &format!(r#"{{"type":"ack","seq":{},"crc":"{crc:08x}"}}"#, h.seq));
                    }
                    Err(e) => log::warn!("Invalid frame: {e}"),
                }
            }
            Err(RecvTimeoutError::Timeout) => {}
            Err(RecvTimeoutError::Disconnected) => anyhow::bail!("websocket event channel closed"),
        }

        if client.is_connected() {
            last_online = Instant::now();
            if last_status.elapsed() >= STATUS_INTERVAL {
                send_status(&client, started);
                last_status = Instant::now();
            }
        } else if last_online.elapsed() >= OFFLINE_RESTART {
            anyhow::bail!("server unreachable for {} min", OFFLINE_RESTART.as_secs() / 60);
        }
    }
}

fn send_status(client: &ws::Client, started: Instant) {
    let mut rssi = 0;
    let mut info = esp_idf_svc::sys::wifi_ap_record_t::default();
    if unsafe { esp_idf_svc::sys::esp_wifi_sta_get_ap_info(&mut info) } == 0 {
        rssi = info.rssi as i32;
    }
    let heap = unsafe { esp_idf_svc::sys::esp_get_free_heap_size() };
    send(client, &format!(r#"{{"type":"status","rssi":{rssi},"free_heap":{heap},"uptime_s":{}}}"#, started.elapsed().as_secs()));
}
