//! Setup portal: an open Wi-Fi access point "homepaper-setup" with a form at
//! http://192.168.71.1 for Wi-Fi credentials, server URL, device ID and
//! token. Saving stores the values in NVS and restarts the device.

use std::convert::Infallible;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::Result;
use esp_idf_svc::http::server::{Configuration as HttpConfig, EspHttpServer};
use esp_idf_svc::http::Method;
use esp_idf_svc::io::Write;
use esp_idf_svc::wifi::{
    AccessPointConfiguration, AuthMethod, BlockingWifi, ClientConfiguration, Configuration, EspWifi,
};

use crate::config::{Config, Settings};

pub const AP_SSID: &str = "homepaper-setup";

/// Runs the portal until the user saves (or `timeout` passes), then restarts.
/// Never returns on success.
pub fn run(
    wifi: &mut BlockingWifi<EspWifi<'static>>,
    settings: Settings,
    current: Config,
    timeout: Option<Duration>,
) -> Result<Infallible> {
    // Scan first (station mode) so the form can offer nearby networks.
    let networks = scan(wifi).unwrap_or_default();

    let ap = AccessPointConfiguration {
        ssid: AP_SSID.try_into().unwrap(),
        auth_method: AuthMethod::None,
        channel: 1,
        ..Default::default()
    };
    if wifi.is_started()? {
        wifi.stop()?;
    }
    wifi.set_configuration(&Configuration::AccessPoint(ap))?;
    wifi.start()?;
    wifi.wait_netif_up()?;
    let ip = wifi.wifi().ap_netif().get_ip_info()?.ip;
    log::info!("Setup portal: join Wi-Fi '{AP_SSID}' and open http://{ip}");

    let settings = Arc::new(Mutex::new(settings));
    let current = Arc::new(Mutex::new(current));
    let mut server = EspHttpServer::new(&HttpConfig::default())?;

    {
        let current = current.clone();
        server.fn_handler("/", Method::Get, move |req| -> Result<()> {
            let html = page(&current.lock().unwrap(), &networks, None);
            req.into_response(200, None, &[("Content-Type", "text/html; charset=utf-8")])?
                .write_all(html.as_bytes())?;
            Ok(())
        })?;
    }

    server.fn_handler("/save", Method::Post, move |mut req| -> Result<()> {
        let mut body = Vec::new();
        let mut buf = [0u8; 256];
        loop {
            let n = req.read(&mut buf)?;
            if n == 0 || body.len() > 4096 {
                break;
            }
            body.extend_from_slice(&buf[..n]);
        }
        let mut c = current.lock().unwrap().clone();
        for (k, v) in parse_form(&String::from_utf8_lossy(&body)) {
            match k.as_str() {
                "wifi_ssid" => c.wifi_ssid = v.trim().into(),
                // Secrets are never shown; an empty field keeps the saved value.
                "wifi_pass" if !v.is_empty() => c.wifi_pass = v,
                "token" if !v.trim().is_empty() => c.token = v.trim().into(),
                "server_url" => c.server_url = v.trim().into(),
                "device_id" => c.device_id = v.trim().into(),
                _ => {}
            }
        }
        if !c.is_complete() {
            let html = page(&c, &[], Some("Please fill in Wi-Fi network, server URL, device ID and token."));
            req.into_response(400, None, &[("Content-Type", "text/html; charset=utf-8")])?
                .write_all(html.as_bytes())?;
            return Ok(());
        }
        settings.lock().unwrap().save(&c)?;
        log::info!("Setup saved (ssid '{}', server {}, device {})", c.wifi_ssid, c.server_url, c.device_id);
        req.into_response(200, None, &[("Content-Type", "text/html; charset=utf-8")])?
            .write_all(SAVED_PAGE.as_bytes())?;
        // Give the response time to leave before restarting.
        std::thread::spawn(|| {
            std::thread::sleep(Duration::from_secs(2));
            esp_idf_svc::hal::reset::restart();
        });
        Ok(())
    })?;

    let opened = std::time::Instant::now();
    loop {
        std::thread::sleep(Duration::from_secs(1));
        if timeout.is_some_and(|t| opened.elapsed() >= t) {
            log::info!("Setup portal timed out, restarting to retry Wi-Fi");
            esp_idf_svc::hal::reset::restart();
        }
    }
}

fn scan(wifi: &mut BlockingWifi<EspWifi<'static>>) -> Result<Vec<String>> {
    if !wifi.is_started()? {
        wifi.set_configuration(&Configuration::Client(ClientConfiguration::default()))?;
        wifi.start()?;
    }
    let mut names: Vec<String> = wifi.scan()?.into_iter().map(|ap| ap.ssid.to_string()).filter(|s| !s.is_empty()).collect();
    names.sort();
    names.dedup();
    Ok(names)
}

fn page(c: &Config, networks: &[String], error: Option<&str>) -> String {
    let options: String = networks.iter().map(|n| format!("<option value=\"{}\">", esc(n))).collect();
    let error = error.map(|e| format!("<p class=err>{}</p>", esc(e))).unwrap_or_default();
    let pass_hint = if c.wifi_pass.is_empty() { "" } else { " placeholder=\"(unchanged)\"" };
    let token_attrs = if c.token.is_empty() { " required" } else { " placeholder=\"(unchanged)\"" };
    format!(
        r#"<!doctype html><html><head><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1">
<title>homepaper setup</title><style>
body{{font-family:system-ui,sans-serif;background:#f4f1ea;color:#1d1b18;margin:0;padding:16px}}
main{{max-width:420px;margin:0 auto;background:#fffdf8;border:1px solid #d9d3c7;border-radius:8px;padding:20px}}
h1{{font-family:Georgia,serif;margin:0 0 4px}}p{{color:#6b665d;margin:0 0 16px}}
label{{display:block;font-size:14px;color:#6b665d;margin:12px 0 4px}}
input{{width:100%;box-sizing:border-box;padding:8px;font-size:16px;border:1px solid #d9d3c7;border-radius:6px}}
button{{margin-top:20px;width:100%;padding:10px;font-size:16px;background:#2f5d50;color:#fff;border:0;border-radius:6px}}
.err{{color:#a83232}}
</style></head><body><main>
<h1>homepaper</h1><p>Connect this display to Wi-Fi and to your homepaper server. Device ID and token are shown in the server's web UI.</p>
{error}
<form method=post action=/save>
<label>Wi-Fi network</label><input name=wifi_ssid list=nets value="{ssid}" required><datalist id=nets>{options}</datalist>
<label>Wi-Fi password</label><input name=wifi_pass type=password{pass_hint}>
<label>Server URL</label><input name=server_url value="{url}" placeholder="ws://192.168.1.10:8080/ws/device" required>
<label>Device ID</label><input name=device_id value="{id}" required>
<label>Token</label><input name=token type=password{token_attrs}>
<button>Save and restart</button>
</form></main></body></html>"#,
        ssid = esc(&c.wifi_ssid),
        url = esc(&c.server_url),
        id = esc(&c.device_id),
    )
}

const SAVED_PAGE: &str = r#"<!doctype html><html><head><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><title>homepaper</title></head>
<body style="font-family:system-ui,sans-serif;padding:24px"><h1>Saved</h1><p>The display restarts and connects to your Wi-Fi. You can close this page.</p></body></html>"#;

fn esc(s: &str) -> String {
    s.replace('&', "&amp;").replace('<', "&lt;").replace('>', "&gt;").replace('"', "&quot;")
}

/// Parses an application/x-www-form-urlencoded body.
fn parse_form(body: &str) -> Vec<(String, String)> {
    body.split('&')
        .filter_map(|pair| {
            let (k, v) = pair.split_once('=')?;
            Some((url_decode(k), url_decode(v)))
        })
        .collect()
}

fn url_decode(s: &str) -> String {
    let bytes = s.as_bytes();
    let mut out = Vec::with_capacity(bytes.len());
    let mut i = 0;
    while i < bytes.len() {
        match bytes[i] {
            b'+' => out.push(b' '),
            b'%' if i + 2 < bytes.len() => {
                let hex = std::str::from_utf8(&bytes[i + 1..i + 3]).ok().and_then(|h| u8::from_str_radix(h, 16).ok());
                match hex {
                    Some(b) => {
                        out.push(b);
                        i += 2;
                    }
                    None => out.push(b'%'),
                }
            }
            b => out.push(b),
        }
        i += 1;
    }
    String::from_utf8_lossy(&out).into_owned()
}
