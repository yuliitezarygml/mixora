fn main() {
    println!("cargo:rerun-if-env-changed=MIXORA_API_URL");
    println!("cargo:rerun-if-changed=../.env");
    let local = dotenvy::from_path_iter("../.env").ok().and_then(|values| {
        values.filter_map(Result::ok).find(|(key, _)| key == "MIXORA_API_URL").map(|(_, value)| value)
    });
    let origin = std::env::var("MIXORA_API_URL").ok().or(local)
        .unwrap_or_else(|| "http://127.0.0.1:8080".into());
    assert!(!origin.contains(['\n', '\r']), "Invalid MIXORA_API_URL");
    if std::env::var("PROFILE").as_deref() == Ok("release") {
        assert!(origin.starts_with("https://"), "Release requires an explicit HTTPS MIXORA_API_URL; use --debug for local testing");
    }
    println!("cargo:rustc-env=MIXORA_API_URL={origin}");
    let attributes = tauri_build::Attributes::new().app_manifest(
        tauri_build::AppManifest::new().commands(&["api_request"]),
    );
    tauri_build::try_build(attributes).expect("Tauri configuration failed");
}
