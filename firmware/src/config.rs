//! Device settings, stored in NVS by the setup portal. Compile-time values
//! from `.env` act as defaults until something has been saved.

use anyhow::Result;
use esp_idf_svc::nvs::{EspDefaultNvsPartition, EspNvs, NvsDefault};

const NAMESPACE: &str = "homepaper";

#[derive(Clone, Debug, Default)]
pub struct Config {
    pub wifi_ssid: String,
    pub wifi_pass: String,
    pub server_url: String,
    pub device_id: String,
    pub token: String,
}

impl Config {
    /// True when everything needed to connect is present.
    pub fn is_complete(&self) -> bool {
        !self.wifi_ssid.is_empty()
            && !self.server_url.is_empty()
            && !self.device_id.is_empty()
            && !self.token.is_empty()
    }

    fn defaults() -> Self {
        Self {
            wifi_ssid: option_env!("HOMEPAPER_WIFI_SSID").unwrap_or("").into(),
            wifi_pass: option_env!("HOMEPAPER_WIFI_PASS").unwrap_or("").into(),
            server_url: option_env!("HOMEPAPER_SERVER_URL").unwrap_or("").into(),
            device_id: option_env!("HOMEPAPER_DEVICE_ID").unwrap_or("").into(),
            token: option_env!("HOMEPAPER_TOKEN").unwrap_or("").into(),
        }
    }
}

pub struct Settings {
    nvs: EspNvs<NvsDefault>,
}

const FORCE_SETUP: &str = "force_setup";

/// Asks the next boot to open the setup portal. Uses its own NVS handle so it
/// can be called from the button thread.
pub fn request_setup(partition: EspDefaultNvsPartition) -> Result<()> {
    let nvs: EspNvs<NvsDefault> = EspNvs::new(partition, NAMESPACE, true)?;
    nvs.set_u8(FORCE_SETUP, 1)?;
    Ok(())
}

const KEYS: [&str; 5] = ["wifi_ssid", "wifi_pass", "server_url", "device_id", "token"];

impl Settings {
    pub fn new(partition: EspDefaultNvsPartition) -> Result<Self> {
        Ok(Self { nvs: EspNvs::new(partition, NAMESPACE, true)? })
    }

    /// Loads the saved config. Fields never saved fall back to `.env` defaults.
    pub fn load(&self) -> Config {
        let mut c = Config::defaults();
        let mut buf = [0u8; 256];
        for key in KEYS {
            if let Ok(Some(v)) = self.nvs.get_str(key, &mut buf) {
                *field(&mut c, key) = v.to_string();
            }
        }
        c
    }

    /// Returns whether setup was requested (BOOT held) and clears the request.
    pub fn take_force_setup(&mut self) -> bool {
        let requested = matches!(self.nvs.get_u8(FORCE_SETUP), Ok(Some(1)));
        if requested {
            let _ = self.nvs.remove(FORCE_SETUP);
        }
        requested
    }

    pub fn save(&mut self, c: &Config) -> Result<()> {
        let mut c = c.clone();
        for key in KEYS {
            self.nvs.set_str(key, field(&mut c, key))?;
        }
        Ok(())
    }
}

fn field<'a>(c: &'a mut Config, key: &str) -> &'a mut String {
    match key {
        "wifi_ssid" => &mut c.wifi_ssid,
        "wifi_pass" => &mut c.wifi_pass,
        "server_url" => &mut c.server_url,
        "device_id" => &mut c.device_id,
        _ => &mut c.token,
    }
}
