fn main() {
    embuild::espidf::sysenv::output();

    // Optional compile-time defaults from .env (HOMEPAPER_WIFI_SSID, ...). They
    // are only used when nothing has been saved via the setup portal yet.
    println!("cargo:rerun-if-changed=.env");
    if let Ok(content) = std::fs::read_to_string(".env") {
        for line in content.lines() {
            let line = line.trim();
            if line.is_empty() || line.starts_with('#') {
                continue;
            }
            if let Some((key, val)) = line.split_once('=') {
                let val = val.trim().trim_matches('"').trim_matches('\'');
                println!("cargo:rustc-env={}={}", key.trim(), val);
            }
        }
    }
}
