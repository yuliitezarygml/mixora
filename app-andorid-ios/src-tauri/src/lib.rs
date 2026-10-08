mod backend;
mod media;
use backend::{ApiRequest, ApiResponse, Backend};
use std::sync::Arc;
use tauri::Manager;

#[tauri::command]
async fn api_request(
    backend: tauri::State<'_, Arc<Backend>>,
    request: ApiRequest,
) -> Result<ApiResponse, String> {
    backend.request(request).await
}

#[tauri::command]
fn prepare_media(
    backend: tauri::State<'_, Arc<Backend>>,
    origin: tauri::State<'_, media::MediaOrigin>,
    path: String,
) -> Result<String, String> {
    backend
        .prepare_media(&path)
        .map(|token| format!("{}/{token}", origin.0))
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .setup(|app| {
            let backend = Arc::new(
                Backend::new(env!("MIXORA_API_URL"), cfg!(debug_assertions))
                    .map_err(std::io::Error::other)?,
            );
            app.manage(media::start_server(backend.clone())?);
            app.manage(backend);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![api_request, prepare_media])
        .run(tauri::generate_context!())
        .expect("Failed to run Mixora Mobile");
}
