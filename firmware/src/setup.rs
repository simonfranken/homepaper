//! Setup portal: an open Wi-Fi access point "homepaper-setup" with a form at
//! http://192.168.71.1 for Wi-Fi credentials, server URL, device ID and
//! token. Saving stores the values in NVS and restarts the device.
//!
//! The server's web UI links to the form with `server_url`, `device_id` and
//! `token` as query parameters. The form then arrives filled in and the user
//! only chooses their Wi-Fi.

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
            let mut c = current.lock().unwrap().clone();
            let prefilled = match req.uri().split_once('?') {
                Some((_, query)) => apply_form(&mut c, query),
                None => false,
            };
            let html = page(&c, &networks, None, prefilled);
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
        apply_form(&mut c, &String::from_utf8_lossy(&body));
        if !c.is_complete() {
            let html = page(&c, &[], Some("Please fill in Wi-Fi network, server URL, device ID and token."), false);
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

/// Applies form fields (a POST body or a query string) to `c`. Returns
/// whether any connection value was set.
fn apply_form(c: &mut Config, form: &str) -> bool {
    let mut changed = false;
    for (k, v) in parse_form(form) {
        match k.as_str() {
            "wifi_ssid" => c.wifi_ssid = v.trim().into(),
            // Secrets are never shown; an empty field keeps the saved value.
            "wifi_pass" if !v.is_empty() => c.wifi_pass = v,
            "token" if !v.trim().is_empty() => {
                c.token = v.trim().into();
                changed = true;
            }
            "server_url" => {
                c.server_url = v.trim().into();
                changed = true;
            }
            "device_id" => {
                c.device_id = v.trim().into();
                changed = true;
            }
            _ => {}
        }
    }
    changed
}

/// Renders the form. With `prefilled`, the connection values came from the
/// link in the server's web UI: the token is carried in a hidden field (it is
/// not saved yet) and the page says that only Wi-Fi is left to enter.
fn page(c: &Config, networks: &[String], error: Option<&str>, prefilled: bool) -> String {
    let options: String = networks.iter().map(|n| format!("<option value=\"{}\">", esc(n))).collect();
    let error = error.map(|e| format!("<p class=err>{}</p>", esc(e))).unwrap_or_default();
    let pass_hint = if c.wifi_pass.is_empty() { "" } else { " placeholder=\"(unchanged)\"" };
    let token_attrs = if c.token.is_empty() { " required" } else { " placeholder=\"(unchanged)\"" };
    let token_field = if prefilled && !c.token.is_empty() {
        format!("<input name=token type=hidden value=\"{}\">", esc(&c.token))
    } else {
        format!("<label for=token>Token</label><input id=token name=token type=password autocomplete=off{token_attrs}>")
    };
    let intro = if prefilled {
        "The server details are filled in. Choose your Wi-Fi network, enter its password and save."
    } else {
        "Connect this display to Wi-Fi and to your homepaper server. The server's web UI shows the server URL, device ID and token under Display."
    };
    format!(
        r#"<!doctype html><html><head><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1">
<title>homepaper setup</title><style>
body{{font-family:system-ui,sans-serif;background:#e3e8eb;color:#14191c;margin:0;padding:16px;line-height:1.45}}
main{{max-width:420px;margin:0 auto;background:#f8fafb;border:1px solid #c2cbd1;border-radius:6px;padding:20px}}
h1{{font-size:22px;margin:0 0 6px}}p{{color:#55616a;margin:0 0 16px}}
label{{display:block;font-size:14px;font-weight:600;margin:14px 0 4px}}
input{{width:100%;box-sizing:border-box;padding:9px;font-size:16px;border:1px solid #c2cbd1;border-radius:4px;background:#fff;color:inherit}}
button{{margin-top:22px;width:100%;padding:11px;font-size:16px;font-weight:600;background:#1269b8;color:#fff;border:0;border-radius:4px}}
.err{{color:#b3312a}}
</style></head><body><main>
<h1>homepaper setup</h1><p>{intro}</p>
{error}
<form method=post action=/save>
<label for=ssid>Wi-Fi network</label><input id=ssid name=wifi_ssid list=nets value="{ssid}" autocapitalize=none autocorrect=off required><datalist id=nets>{options}</datalist>
<label for=pass>Wi-Fi password</label><input id=pass name=wifi_pass type=password{pass_hint}>
<label for=url>Server URL</label><input id=url name=server_url value="{url}" placeholder="ws://192.168.1.10:8080/ws/device" autocapitalize=none autocorrect=off required>
<label for=id>Device ID</label><input id=id name=device_id value="{id}" autocapitalize=none autocorrect=off required>
{token_field}
<button>Save and restart</button>
</form></main></body></html>"#,
        ssid = esc(&c.wifi_ssid),
        url = esc(&c.server_url),
        id = esc(&c.device_id),
    )
}

const SAVED_PAGE: &str = r#"<!doctype html><html><head><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><title>homepaper</title></head>
<body style="font-family:system-ui,sans-serif;padding:24px;line-height:1.45"><h1>Saved</h1><p>The display restarts and joins your Wi-Fi. Switch this device back to your own network; the display then shows up as online in the homepaper web UI.</p><p>If it doesn't within a minute, the Wi-Fi password was probably wrong. The setup network comes back by itself so you can try again.</p></body></html>"#;

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
