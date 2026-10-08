mod backend;
use backend::{ApiRequest, ApiResponse, Backend};
use tauri::Manager;

#[tauri::command]
async fn api_request(
    backend: tauri::State<'_, Backend>,
    request: ApiRequest,
) -> Result<ApiResponse, String> {
    backend.request(request).await
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .setup(|app| {
            let backend = Backend::new(env!("MIXORA_API_URL"), cfg!(debug_assertions))
                .map_err(std::io::Error::other)?;
            app.manage(backend);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![api_request])
        .run(tauri::generate_context!())
        .expect("Failed to run Mixora Mobile");
}
