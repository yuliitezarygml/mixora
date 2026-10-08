use crate::backend::Backend;
use http_body_util::BodyExt;
use reqwest::Url;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tauri::http::{Method, Request, Response, header};

const MAX_MEDIA_CHUNK: usize = 8 * 1024 * 1024;
const GRANT_TTL: Duration = Duration::from_secs(3600);

pub struct MediaOrigin(pub String);

// WKWebView custom schemes can repeatedly request the same partial file or
// treat it as a complete short clip. Real loopback HTTP preserves AVFoundation
// Range semantics without handing the backend cookie to the WebView.
pub fn start_server(backend: Arc<Backend>) -> Result<MediaOrigin, std::io::Error> {
    let listener = std::net::TcpListener::bind((std::net::Ipv4Addr::LOCALHOST, 0))?;
    listener.set_nonblocking(true)?;
    let address = listener.local_addr()?;
    tauri::async_runtime::spawn(async move {
        let Ok(listener) = tokio::net::TcpListener::from_std(listener) else {
            return;
        };
        let connections = Arc::new(tokio::sync::Semaphore::new(16));
        while let Ok((stream, _)) = listener.accept().await {
            let Ok(permit) = connections.clone().try_acquire_owned() else {
                continue;
            };
            let backend = backend.clone();
            tauri::async_runtime::spawn(async move {
                let _permit = permit;
                let service = hyper::service::service_fn(
                    move |request: hyper::Request<hyper::body::Incoming>| {
                        let backend = backend.clone();
                        async move {
                            let host = request
                                .headers()
                                .get(header::HOST)
                                .and_then(|value| value.to_str().ok());
                            let expected_host = address.to_string();
                            let response = if host != Some(expected_host.as_str())
                                || request.uri().query().is_some()
                            {
                                stream_error(403)
                            } else {
                                backend.stream_response(request.map(|_| Vec::new())).await
                            };
                            Ok::<_, std::convert::Infallible>(response)
                        }
                    },
                );
                let _ = hyper::server::conn::http1::Builder::new()
                    .keep_alive(false)
                    .max_buf_size(16 * 1024)
                    .max_headers(32)
                    .timer(hyper_util::rt::TokioTimer::new())
                    .header_read_timeout(Duration::from_secs(5))
                    .serve_connection(hyper_util::rt::TokioIo::new(stream), service)
                    .await;
            });
        }
    });
    Ok(MediaOrigin(format!("http://{address}")))
}

pub(crate) fn error(status: u16) -> Response<Vec<u8>> {
    Response::builder()
        .status(status)
        .header(header::CACHE_CONTROL, "no-store")
        .body(Vec::new())
        .unwrap()
}

type MediaBody = http_body_util::combinators::UnsyncBoxBody<hyper::body::Bytes, std::io::Error>;

fn full_body(bytes: Vec<u8>) -> MediaBody {
    http_body_util::Full::new(hyper::body::Bytes::from(bytes))
        .map_err(|never| match never {})
        .boxed_unsync()
}

fn stream_error(status: u16) -> Response<MediaBody> {
    error(status).map(full_body)
}

fn content_range(value: &str) -> Option<(u64, u64, u64)> {
    let (bounds, total) = value.strip_prefix("bytes ")?.split_once('/')?;
    let (start, end) = bounds.split_once('-')?;
    let (start, end, total) = (start.parse().ok()?, end.parse().ok()?, total.parse().ok()?);
    (start <= end && end < total).then_some((start, end, total))
}

fn requested_bounds(value: Option<&str>, total: u64) -> Option<(u64, u64)> {
    let Some(value) = value else {
        return Some((0, total.checked_sub(1)?));
    };
    let (start, end) = value.strip_prefix("bytes=")?.split_once('-')?;
    if start.is_empty() {
        let suffix: u64 = end.parse().ok()?;
        return (suffix > 0).then_some((total.saturating_sub(suffix), total - 1));
    }
    let start = start.parse().ok()?;
    let end = if end.is_empty() {
        total - 1
    } else {
        end.parse::<u64>().ok()?.min(total - 1)
    };
    (start <= end && start < total).then_some((start, end))
}

struct StreamState {
    backend: Arc<Backend>,
    uri: String,
    first: Option<Vec<u8>>,
    next: u64,
    end: u64,
    total: u64,
    mime: String,
}

impl Backend {
    // Fulfil the media engine's *entire* requested range. The Go proxy clamps
    // each transfer to 1 MB; forwarding just its first 206 makes WebKit treat
    // that chunk as a short clip. Stream consecutive bounded chunks instead,
    // and return 200/full length for an ordinary GET without Range.
    async fn stream_response(
        self: Arc<Self>,
        mut request: Request<Vec<u8>>,
    ) -> Response<MediaBody> {
        let head = request.method() == Method::HEAD;
        let range = request
            .headers()
            .get(header::RANGE)
            .and_then(|value| value.to_str().ok())
            .map(str::to_owned);
        let uri = request.uri().to_string();
        if range.is_none() {
            request
                .headers_mut()
                .insert(header::RANGE, "bytes=0-".parse().unwrap());
        }
        let mut first = self.media_chunk(request).await;
        if first.status() != 206 {
            return first.map(full_body);
        }
        let parsed = first
            .headers()
            .get(header::CONTENT_RANGE)
            .and_then(|value| value.to_str().ok())
            .and_then(content_range);
        let Some((mut start, mut end, total)) = parsed else {
            return stream_error(502);
        };
        // Match the existing server's maximum resource size; memory stays
        // bounded by one chunk regardless of the length of an album.
        if total > 512 * 1024 * 1024 {
            return stream_error(502);
        }
        let Some((wanted_start, wanted_end)) = requested_bounds(range.as_deref(), total) else {
            return stream_error(416);
        };
        // The provider proxy also caps suffix requests to one chunk. Once its
        // total is known, restart at the actual requested suffix's first byte.
        if start != wanted_start && range.as_deref().is_some_and(|v| v.starts_with("bytes=-")) {
            let request = Request::builder()
                .method(if head { Method::HEAD } else { Method::GET })
                .uri(&uri)
                .header(header::RANGE, format!("bytes={wanted_start}-{wanted_end}"))
                .body(Vec::new())
                .unwrap();
            first = self.media_chunk(request).await;
            let parsed = first
                .headers()
                .get(header::CONTENT_RANGE)
                .and_then(|v| v.to_str().ok())
                .and_then(content_range);
            match parsed {
                Some((new_start, new_end, new_total))
                    if first.status() == 206 && new_total == total =>
                {
                    start = new_start;
                    end = new_end;
                }
                _ => return stream_error(502),
            }
        }
        if start != wanted_start || end > wanted_end {
            return stream_error(502);
        }
        let (mut parts, bytes) = first.into_parts();
        let mime = parts
            .headers
            .get(header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok())
            .unwrap_or("")
            .to_owned();
        parts.status = if range.is_some() {
            tauri::http::StatusCode::PARTIAL_CONTENT
        } else {
            tauri::http::StatusCode::OK
        };
        parts.headers.insert(
            header::CONTENT_LENGTH,
            (wanted_end - wanted_start + 1).to_string().parse().unwrap(),
        );
        if range.is_some() {
            parts.headers.insert(
                header::CONTENT_RANGE,
                format!("bytes {wanted_start}-{wanted_end}/{total}")
                    .parse()
                    .unwrap(),
            );
        } else {
            parts.headers.remove(header::CONTENT_RANGE);
        }
        if head {
            return Response::from_parts(parts, full_body(Vec::new()));
        }
        if bytes.len() as u64 != end - start + 1 {
            return stream_error(502);
        }
        let state = StreamState {
            backend: self,
            uri,
            first: Some(bytes),
            next: end + 1,
            end: wanted_end,
            total,
            mime,
        };
        let stream = futures_util::stream::unfold(state, |mut state| async move {
            if let Some(bytes) = state.first.take() {
                return Some((
                    Ok(hyper::body::Frame::data(hyper::body::Bytes::from(bytes))),
                    state,
                ));
            }
            if state.next > state.end {
                return None;
            }
            let request = Request::builder()
                .uri(&state.uri)
                .header(header::RANGE, format!("bytes={}-{}", state.next, state.end))
                .body(Vec::new())
                .unwrap();
            let response = state.backend.media_chunk(request).await;
            let parsed = response
                .headers()
                .get(header::CONTENT_RANGE)
                .and_then(|value| value.to_str().ok())
                .and_then(content_range);
            let matches_mime = response
                .headers()
                .get(header::CONTENT_TYPE)
                .and_then(|value| value.to_str().ok())
                == Some(state.mime.as_str());
            let valid = response.status() == 206
                && matches_mime
                && parsed.is_some_and(|(start, end, total)| {
                    start == state.next
                        && end <= state.end
                        && total == state.total
                        && response.body().len() as u64 == end - start + 1
                });
            if !valid {
                state.next = state.end + 1;
                return Some((Err(std::io::Error::other("Media range unavailable")), state));
            }
            state.next = parsed.unwrap().1 + 1;
            Some((
                Ok(hyper::body::Frame::data(hyper::body::Bytes::from(
                    response.into_body(),
                ))),
                state,
            ))
        });
        Response::from_parts(
            parts,
            http_body_util::StreamBody::new(stream).boxed_unsync(),
        )
    }

    // A signed provider URL may expire or its CDN may fail between chunks.
    // One retry asks our server to resolve it again. Never retry auth failures,
    // rate limits or invalid ranges, and never change provider/track silently.
    async fn media_chunk(&self, request: Request<Vec<u8>>) -> Response<Vec<u8>> {
        let retry = Request::builder()
            .method(request.method().clone())
            .uri(request.uri().clone())
            .header(
                header::RANGE,
                request
                    .headers()
                    .get(header::RANGE)
                    .cloned()
                    .unwrap_or_else(|| "bytes=0-999999".parse().unwrap()),
            )
            .body(Vec::new())
            .unwrap();
        let response = self.media_response(request).await;
        if matches!(response.status().as_u16(), 502 | 504) {
            self.media_response(retry).await
        } else {
            response
        }
    }

    // An opaque, bounded grant identifies one server-owned media path. Never
    // expose the session cookie, accept a CDN URL or turn this into a URL proxy.
    pub fn prepare_media(&self, path: &str) -> Result<String, String> {
        if path.len() > 8192 || !path.starts_with("/api/v1/media/stream?") {
            return Err("Недопустимый путь аудио".into());
        }
        let url = self
            .origin
            .join(path)
            .map_err(|_| "Недопустимый путь аудио")?;
        if url.origin() != self.origin.origin()
            || url.path() != "/api/v1/media/stream"
            || url.fragment().is_some()
        {
            return Err("Недопустимый путь аудио".into());
        }
        let params: Vec<_> = url.query_pairs().collect();
        if params.len() != 1 || params[0].0 != "url" || params[0].1.is_empty() {
            return Err("Недопустимые параметры аудио".into());
        }
        let target = Url::parse(&params[0].1).map_err(|_| "Недопустимый источник аудио")?;
        if target.scheme() != "https"
            || !target.username().is_empty()
            || target.password().is_some()
        {
            return Err("Недопустимый источник аудио".into());
        }
        let token = uuid::Uuid::new_v4().to_string();
        let mut grants = self.media.lock().map_err(|_| "Media state unavailable")?;
        grants.retain(|_, (_, created)| created.elapsed() < GRANT_TTL);
        if grants.len() >= 32 {
            let oldest = grants
                .iter()
                .min_by_key(|(_, (_, time))| time)
                .map(|(key, _)| key.clone());
            if let Some(oldest) = oldest {
                grants.remove(&oldest);
            }
        }
        grants.insert(token.clone(), (url, Instant::now()));
        Ok(token)
    }

    pub async fn media_response(&self, request: Request<Vec<u8>>) -> Response<Vec<u8>> {
        if request.method() != Method::GET && request.method() != Method::HEAD {
            return error(405);
        }
        let token = request.uri().path().trim_start_matches('/');
        let url = self.media.lock().ok().and_then(|grants| {
            grants
                .get(token)
                .filter(|(_, created)| created.elapsed() < GRANT_TTL)
                .map(|(url, _)| url.clone())
        });
        let Some(url) = url else { return error(403) };
        let Ok(_permit) = self.permits.try_acquire() else {
            return error(429);
        };
        let client = self.client.read().await.clone();
        let range = request
            .headers()
            .get(header::RANGE)
            .cloned()
            .unwrap_or_else(|| "bytes=0-999999".parse().unwrap());
        // Backend validates single ranges and limits each provider transfer.
        let Ok(mut upstream) = client
            .get(url)
            .header(header::RANGE, range)
            .header(header::ACCEPT_ENCODING, "identity")
            .send()
            .await
        else {
            return error(502);
        };
        let status = upstream.status().as_u16();
        if status != 200 && status != 206 {
            let mut result = error(if [401, 403, 416, 429, 503, 504].contains(&status) {
                status
            } else {
                502
            });
            if status == 416
                && let Some(value) = upstream.headers().get(header::CONTENT_RANGE)
            {
                result
                    .headers_mut()
                    .insert(header::CONTENT_RANGE, value.clone());
            }
            return result;
        }
        let mime = upstream
            .headers()
            .get(header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok())
            .unwrap_or("");
        let expected_length = upstream.content_length();
        if !(mime.starts_with("audio/")
            || mime.starts_with("video/mp4")
            || mime.starts_with("application/ogg"))
            || expected_length.is_some_and(|length| length > MAX_MEDIA_CHUNK as u64)
        {
            return error(502);
        }
        let mut builder = Response::builder()
            .status(status)
            .header(header::CACHE_CONTROL, "private, no-store");
        for name in [
            header::CONTENT_TYPE,
            header::CONTENT_RANGE,
            header::ACCEPT_RANGES,
        ] {
            if let Some(value) = upstream.headers().get(&name) {
                builder = builder.header(name, value);
            }
        }
        let mut bytes = Vec::new();
        loop {
            match upstream.chunk().await {
                Ok(Some(chunk)) => {
                    if bytes.len().saturating_add(chunk.len()) > MAX_MEDIA_CHUNK {
                        return error(502);
                    }
                    bytes.extend_from_slice(&chunk);
                }
                Ok(None) => break,
                Err(_) => return error(502),
            }
        }
        if expected_length.is_some_and(|length| length != bytes.len() as u64) {
            return error(502);
        }
        builder = builder.header(header::CONTENT_LENGTH, bytes.len());
        if request.method() == Method::HEAD {
            bytes.clear();
        }
        builder.body(bytes).unwrap_or_else(|_| error(502))
    }
}
