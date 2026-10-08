fn main() {
    println!("cargo:rerun-if-env-changed=MIXORA_API_URL");
    println!("cargo:rerun-if-changed=../api.json");
    let config: serde_json::Value =
        serde_json::from_slice(&std::fs::read("../api.json").expect("api.json is required"))
            .expect("Invalid api.json");
    let server = config["serverUrl"]
        .as_str()
        .expect("api.json serverUrl is required");
    let android = std::env::var("CARGO_CFG_TARGET_OS").as_deref() == Ok("android");
    let local = server.starts_with("http://127.0.0.1:") || server.starts_with("http://localhost:");
    let configured = if android && local {
        config["androidEmulatorUrl"]
            .as_str()
            .expect("api.json androidEmulatorUrl is required")
    } else {
        server
    };
    let origin = std::env::var("MIXORA_API_URL")
        .ok()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| configured.into());
    assert_eq!(
        config["apiPrefix"], "/api/v1",
        "apiPrefix must match the existing backend contract"
    );
    assert_eq!(
        config["playbackSocketPath"], "/api/v1/playback/ws",
        "Invalid playback socket path"
    );
    assert!(!origin.contains(['\n', '\r']), "Invalid MIXORA_API_URL");
    if std::env::var("PROFILE").as_deref() == Ok("release") {
        assert!(
            origin.starts_with("https://"),
            "Release requires an explicit HTTPS MIXORA_API_URL; use --debug for local testing"
        );
    }
    println!("cargo:rustc-env=MIXORA_API_URL={origin}");
    let attributes = tauri_build::Attributes::new()
        .app_manifest(tauri_build::AppManifest::new().commands(&["api_request"]));
    tauri_build::try_build(attributes).expect("Tauri configuration failed");
}
