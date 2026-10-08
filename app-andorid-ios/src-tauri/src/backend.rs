use reqwest::Url;
use serde::{Deserialize, Serialize};
use std::{net::IpAddr, sync::Arc, time::Duration};
use tokio::sync::{RwLock, Semaphore};

const MAX_REQUEST_BYTES: usize = 1024 * 1024;
const MAX_RESPONSE_BYTES: usize = 8 * 1024 * 1024;

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

fn backend_origin(value: &str, debug: bool) -> Result<Url, String> {
    let url = Url::parse(value).map_err(|_| "Некорректный адрес сервера в api.json")?;
    if !url.username().is_empty()
        || url.password().is_some()
        || url.host_str().is_none()
        || url.path() != "/"
        || url.query().is_some()
        || url.fragment().is_some()
    {
        return Err(
            "В api.json нужен только origin сервера, без credentials, path, query или fragment"
                .into(),
        );
    }
    if url.scheme() == "https" {
        return Ok(url);
    }
    let host = url.host_str().unwrap_or_default();
    let local = host == "localhost"
        || match host.trim_matches(['[', ']']).parse::<IpAddr>() {
            Ok(IpAddr::V4(ip)) => ip.is_loopback() || ip.is_private(),
            Ok(IpAddr::V6(ip)) => ip.is_loopback(),
            Err(_) => false,
        };
    if debug && url.scheme() == "http" && local {
        return Ok(url);
    }
    Err("Для сервера требуется HTTPS; HTTP разрешён только в debug для локальной сети".into())
}

impl ApiRequest {
    fn validate(&self, origin: &Url) -> Result<(Url, reqwest::Method), String> {
        let lower = self
            .path
            .split('?')
            .next()
            .unwrap_or_default()
            .to_ascii_lowercase();
        if !self.path.starts_with("/api/v1/")
            || self.path.contains(['\\', '#', '\n', '\r'])
            || lower.contains("%2f")
            || lower.contains("%5c")
            || lower.contains("%00")
        {
            return Err("Недопустимый путь API".into());
        }
        let url = origin
            .join(&self.path)
            .map_err(|_| "Недопустимый путь API")?;
        if url.origin() != origin.origin()
            || !url.path().starts_with("/api/v1/")
            || url.path().starts_with("/api/v1/media/")
        {
            return Err("Этот путь недоступен через JSON API transport".into());
        }
        let method = reqwest::Method::from_bytes(self.method.as_bytes())
            .map_err(|_| "Недопустимый метод API")?;
        if !matches!(
            method,
            reqwest::Method::GET
                | reqwest::Method::POST
                | reqwest::Method::PUT
                | reqwest::Method::PATCH
                | reqwest::Method::DELETE
        ) {
            return Err("Недопустимый метод API".into());
        }
        if let Some(body) = &self.body {
            if method == reqwest::Method::GET || body.len() > MAX_REQUEST_BYTES {
                return Err("Недопустимое тело API запроса".into());
            }
            serde_json::from_str::<serde_json::Value>(body)
                .map_err(|_| "API принимает только JSON")?;
        }
        Ok((url, method))
    }
}

pub struct Backend {
    origin: Url,
    client: RwLock<reqwest::Client>,
    permits: Semaphore,
}

fn session_client() -> Result<reqwest::Client, String> {
    reqwest::Client::builder()
        .cookie_provider(Arc::new(reqwest::cookie::Jar::default()))
        .redirect(reqwest::redirect::Policy::none())
        .connect_timeout(Duration::from_secs(5))
        .timeout(Duration::from_secs(30))
        .user_agent("Mixora-Mobile/0.1")
        .build()
        .map_err(|_| "Не удалось создать native API session".into())
}

impl Backend {
    pub fn new(value: &str, debug: bool) -> Result<Self, String> {
        Ok(Self {
            origin: backend_origin(value, debug)?,
            client: RwLock::new(session_client()?),
            permits: Semaphore::new(8),
        })
    }
    pub async fn request(&self, request: ApiRequest) -> Result<ApiResponse, String> {
        let (url, method) = request.validate(&self.origin)?;
        let logout = url.path() == "/api/v1/auth/logout" && method == reqwest::Method::POST;
        let result = self.send(url, method, request.body).await;
        // Even a network failure must not leave a logged-out session locally.
        if logout {
            self.clear_session().await?;
        }
        result
    }
    async fn send(
        &self,
        url: Url,
        method: reqwest::Method,
        body: Option<String>,
    ) -> Result<ApiResponse, String> {
        let _permit = self
            .permits
            .try_acquire()
            .map_err(|_| "Слишком много одновременных API запросов")?;
        let client = self.client.read().await.clone();
        let mut builder = client
            .request(method, url)
            .header(reqwest::header::ACCEPT, "application/json");
        if let Some(body) = body {
            builder = builder
                .header(reqwest::header::CONTENT_TYPE, "application/json")
                .body(body);
        }
        let mut response = builder
            .send()
            .await
            .map_err(|_| "Не удалось связаться с сервером")?;
        let status = response.status().as_u16();
        if response.status().is_redirection() {
            return Err("API redirect запрещён".into());
        }
        if status == 204 {
            return Ok(ApiResponse {
                status,
                body: String::new(),
            });
        }
        if response
            .content_length()
            .is_some_and(|length| length > MAX_RESPONSE_BYTES as u64)
        {
            return Err("Ответ API слишком большой".into());
        }
        let mut bytes = Vec::new();
        while let Some(chunk) = response
            .chunk()
            .await
            .map_err(|_| "Не удалось прочитать API ответ")?
        {
            if bytes.len().saturating_add(chunk.len()) > MAX_RESPONSE_BYTES {
                return Err("Ответ API слишком большой".into());
            }
            bytes.extend_from_slice(&chunk);
        }
        let body = String::from_utf8(bytes).map_err(|_| "API ответ не является UTF-8 JSON")?;
        Ok(ApiResponse { status, body })
    }
    pub async fn clear_session(&self) -> Result<(), String> {
        *self.client.write().await = session_client()?;
        Ok(())
    }
}

#[cfg(test)]
#[path = "backend_tests.rs"]
mod tests;
