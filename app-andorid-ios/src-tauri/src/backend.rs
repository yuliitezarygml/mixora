use reqwest::Url;
use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
pub struct ApiRequest {
    pub path: String,
    pub method: String,
    pub body: Option<String>,
}

#[derive(Serialize)]
pub struct ApiResponse {
    pub status: u16,
    pub body: String,
}

fn backend_origin(_value: &str, _debug: bool) -> Result<Url, String> {
    Err("Origin validation not implemented".into())
}

impl ApiRequest {
    fn validate(&self, _origin: &Url) -> Result<(Url, reqwest::Method), String> {
        Err("Request validation not implemented".into())
    }
}

pub struct Backend;

impl Backend {
    pub fn new(_value: &str, _debug: bool) -> Result<Self, String> {
        Err("Native session not implemented".into())
    }
    pub async fn request(&self, _request: ApiRequest) -> Result<ApiResponse, String> {
        Err("Native API not implemented".into())
    }
    pub async fn clear_session(&self) -> Result<(), String> { Ok(()) }
}

#[cfg(test)]
#[path = "backend_tests.rs"]
mod tests;
