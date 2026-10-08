use super::*;
use tokio::io::{AsyncReadExt, AsyncWriteExt};

#[tokio::test]
async fn media_grants_preserve_cookie_and_range_without_exposing_cookie_headers() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let origin = format!("http://{}", listener.local_addr().unwrap());
    let server = tokio::spawn(async move {
        for index in 0..2 {
            let (mut stream, _) = listener.accept().await.unwrap();
            let headers = request_headers(&mut stream).await.to_lowercase();
            let response = if index == 0 {
                "HTTP/1.1 200 OK\r\nSet-Cookie: mixora_session=fixture; Path=/api/v1; HttpOnly\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}"
            } else {
                assert!(headers.contains("cookie: mixora_session=fixture"));
                assert!(headers.contains("range: bytes=10-13"));
                "HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\nContent-Length: 4\r\nContent-Range: bytes 10-13/100\r\nAccept-Ranges: bytes\r\nSet-Cookie: hidden=fixture\r\nConnection: close\r\n\r\naudi"
            };
            stream.write_all(response.as_bytes()).await.unwrap();
        }
    });
    let backend = Backend::new(&origin, true).unwrap();
    backend
        .request(ApiRequest {
            path: "/api/v1/auth/login".into(),
            method: "GET".into(),
            body: None,
        })
        .await
        .unwrap();
    let token = backend
        .prepare_media(
            "/api/v1/media/stream?url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3Dfixture",
        )
        .unwrap();
    let response = backend
        .media_response(
            tauri::http::Request::builder()
                .uri(format!("/{token}"))
                .header("Range", "bytes=10-13")
                .body(Vec::new())
                .unwrap(),
        )
        .await;
    assert_eq!(response.status(), 206);
    assert_eq!(response.body(), b"audi");
    assert_eq!(response.headers()["Content-Range"], "bytes 10-13/100");
    assert!(response.headers().get("Set-Cookie").is_none());
    backend.clear_session().await.unwrap();
    let denied = backend
        .media_response(
            tauri::http::Request::builder()
                .uri(format!("/{token}"))
                .body(Vec::new())
                .unwrap(),
        )
        .await;
    assert_eq!(denied.status(), 403);
    server.await.unwrap();
}

#[test]
fn media_grants_cannot_access_json_files_or_arbitrary_origins() {
    let backend = Backend::new("https://api.example.test", false).unwrap();
    for path in [
        "/api/v1/me",
        "//elsewhere.test/api/v1/media/stream?url=https://example.test",
        "/api/v1/media/stream?url=file:///secret",
        "/api/v1/media/stream?url=https://user:secret@example.test",
        "/api/v1/media/stream?url=https://example.test&token=secret",
    ] {
        assert!(backend.prepare_media(path).is_err());
    }
}

#[tokio::test]
async fn loopback_media_server_rejects_unknown_grants_and_rebinding_hosts() {
    let backend = std::sync::Arc::new(Backend::new("https://api.example.test", false).unwrap());
    let origin = crate::media::start_server(backend.clone()).unwrap();
    assert!(origin.0.starts_with("http://127.0.0.1:"));
    let client = reqwest::Client::new();
    let response = client
        .get(format!("{}/unknown", origin.0))
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 403);
    let token = backend
        .prepare_media("/api/v1/media/stream?url=https://www.youtube.com/watch?v=fixture")
        .unwrap();
    let response = client
        .get(format!("{}/{token}", origin.0))
        .header("Host", "attacker.example")
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 403);
    let response = client
        .post(format!("{}/{token}", origin.0))
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 405);
    backend.media.lock().unwrap().get_mut(&token).unwrap().1 =
        std::time::Instant::now() - std::time::Duration::from_secs(3601);
    let response = client
        .get(format!("{}/{token}", origin.0))
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 403);
}

#[tokio::test]
async fn loopback_media_server_serves_real_http_ranges_and_head_without_cookies() {
    let (backend, server) = fixture_backend_response("HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mp4\r\nContent-Range: bytes 0-3/100\r\nContent-Length: 4\r\nAccept-Ranges: bytes\r\nSet-Cookie: private=fixture\r\nConnection: close\r\n\r\naudi".into()).await;
    let backend = std::sync::Arc::new(backend);
    let token = backend
        .prepare_media("/api/v1/media/stream?url=https://www.youtube.com/watch?v=fixture")
        .unwrap();
    let origin = crate::media::start_server(backend).unwrap();
    let response = reqwest::Client::new()
        .head(format!("{}/{token}", origin.0))
        .header("Range", "bytes=0-3")
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 206);
    assert_eq!(response.headers()["Content-Range"], "bytes 0-3/100");
    assert_eq!(response.headers()["Content-Length"], "4");
    assert!(response.headers().get("Set-Cookie").is_none());
    assert!(response.bytes().await.unwrap().is_empty());
    server.await.unwrap();
}

#[tokio::test]
async fn media_rejects_html_and_oversized_or_truncated_audio() {
    for response in [
        "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 4\r\nConnection: close\r\n\r\naudi",
        "HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\nContent-Length: 9000000\r\nConnection: close\r\n\r\n",
        "HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\nContent-Length: 10\r\nConnection: close\r\n\r\naudi",
    ] {
        let (backend, server) = fixture_backend_response(response.into()).await;
        let token = backend
            .prepare_media("/api/v1/media/stream?url=https://www.youtube.com/watch?v=fixture")
            .unwrap();
        let result = backend
            .media_response(
                tauri::http::Request::builder()
                    .uri(format!("/{token}"))
                    .body(Vec::new())
                    .unwrap(),
            )
            .await;
        assert_eq!(result.status(), 502);
        server.await.unwrap();
    }
}

#[tokio::test]
async fn loopback_stream_fulfils_the_whole_resource_or_requested_range_not_a_short_clip() {
    for (range, status, expected_range) in [
        (None, 200, None),
        (Some("bytes=0-"), 206, Some("bytes 0-7/8")),
    ] {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let upstream = format!("http://{}", listener.local_addr().unwrap());
        let server = tokio::spawn(async move {
            for index in 0..2 {
                let (mut stream, _) = listener.accept().await.unwrap();
                let headers = request_headers(&mut stream).await.to_lowercase();
                assert!(headers.contains(if index == 0 {
                    "range: bytes=0-"
                } else {
                    "range: bytes=4-7"
                }));
                let bounds = if index == 0 { "0-3" } else { "4-7" };
                let body = if index == 0 { "abcd" } else { "efgh" };
                stream.write_all(format!("HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\nContent-Length: 4\r\nContent-Range: bytes {bounds}/8\r\nConnection: close\r\n\r\n{body}").as_bytes()).await.unwrap();
            }
        });
        let backend = std::sync::Arc::new(Backend::new(&upstream, true).unwrap());
        let token = backend
            .prepare_media("/api/v1/media/stream?url=https://www.youtube.com/watch?v=fixture")
            .unwrap();
        let origin = crate::media::start_server(backend).unwrap();
        let mut request = reqwest::Client::new().get(format!("{}/{token}", origin.0));
        if let Some(range) = range {
            request = request.header("Range", range)
        }
        let response = request.send().await.unwrap();
        assert_eq!(response.status(), status);
        assert_eq!(response.headers()["Content-Length"], "8");
        assert_eq!(
            response
                .headers()
                .get("Content-Range")
                .and_then(|value| value.to_str().ok()),
            expected_range
        );
        assert_eq!(response.bytes().await.unwrap(), b"abcdefgh"[..]);
        server.await.unwrap();
    }
}

#[tokio::test]
async fn loopback_stream_retries_one_transient_failure_without_restarting_audio() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let upstream = format!("http://{}", listener.local_addr().unwrap());
    let server = tokio::spawn(async move {
        for index in 0..3 {
            let (mut stream, _) = listener.accept().await.unwrap();
            let headers = request_headers(&mut stream).await.to_lowercase();
            assert!(headers.contains(if index == 0 {
                "range: bytes=0-"
            } else {
                "range: bytes=4-7"
            }));
            let response = match index {
                0 => {
                    "HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\nContent-Length: 4\r\nContent-Range: bytes 0-3/8\r\nConnection: close\r\n\r\nabcd"
                }
                1 => "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n",
                _ => {
                    "HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\nContent-Length: 4\r\nContent-Range: bytes 4-7/8\r\nConnection: close\r\n\r\nefgh"
                }
            };
            stream.write_all(response.as_bytes()).await.unwrap();
        }
    });
    let backend = std::sync::Arc::new(Backend::new(&upstream, true).unwrap());
    let token = backend
        .prepare_media("/api/v1/media/stream?url=https://www.youtube.com/watch?v=fixture")
        .unwrap();
    let origin = crate::media::start_server(backend).unwrap();
    let response = reqwest::get(format!("{}/{token}", origin.0)).await.unwrap();
    assert_eq!(response.status(), 200);
    assert_eq!(response.bytes().await.unwrap(), b"abcdefgh"[..]);
    server.await.unwrap();
}

#[tokio::test]
async fn loopback_stream_fulfils_a_suffix_larger_than_the_provider_chunk() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let upstream = format!("http://{}", listener.local_addr().unwrap());
    let server = tokio::spawn(async move {
        for (requested, bounds, body) in [
            ("bytes=-6", "4-7", "efgh"),
            ("bytes=2-7", "2-5", "cdef"),
            ("bytes=6-7", "6-7", "gh"),
        ] {
            let (mut stream, _) = listener.accept().await.unwrap();
            let headers = request_headers(&mut stream).await.to_lowercase();
            assert!(headers.contains(&format!("range: {requested}")));
            stream.write_all(format!("HTTP/1.1 206 Partial Content\r\nContent-Type: audio/mpeg\r\nContent-Length: {}\r\nContent-Range: bytes {bounds}/8\r\nConnection: close\r\n\r\n{body}", body.len()).as_bytes()).await.unwrap();
        }
    });
    let backend = std::sync::Arc::new(Backend::new(&upstream, true).unwrap());
    let token = backend
        .prepare_media("/api/v1/media/stream?url=https://www.youtube.com/watch?v=fixture")
        .unwrap();
    let origin = crate::media::start_server(backend).unwrap();
    let response = reqwest::Client::new()
        .get(format!("{}/{token}", origin.0))
        .header("Range", "bytes=-6")
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 206);
    assert_eq!(response.headers()["Content-Length"], "6");
    assert_eq!(response.headers()["Content-Range"], "bytes 2-7/8");
    assert_eq!(response.bytes().await.unwrap(), b"cdefgh"[..]);
    server.await.unwrap();
}

async fn request_headers(stream: &mut tokio::net::TcpStream) -> String {
    let mut bytes = Vec::new();
    loop {
        let mut chunk = [0; 1024];
        let length = stream.read(&mut chunk).await.unwrap();
        assert!(
            length > 0,
            "fixture connection closed before request headers"
        );
        bytes.extend_from_slice(&chunk[..length]);
        assert!(bytes.len() <= 16 * 1024, "fixture headers exceed limit");
        if bytes.windows(4).any(|window| window == b"\r\n\r\n") {
            break;
        }
    }
    String::from_utf8(bytes).unwrap()
}

#[test]
fn release_requires_a_plain_https_origin_without_credentials_or_paths() {
    assert!(backend_origin("https://api.example.test", false).is_ok());
    for value in [
        "http://api.example.test",
        "https://user:secret@api.example.test",
        "https://api.example.test/api",
        "https://api.example.test/?token=secret",
        "https://api.example.test/#fragment",
        "file:///etc/passwd",
    ] {
        assert!(
            backend_origin(value, false).is_err(),
            "unexpected accepted origin"
        );
    }
}

#[test]
fn debug_http_is_limited_to_explicit_local_development_hosts() {
    for value in [
        "http://127.0.0.1:8080",
        "http://localhost:8080",
        "http://10.0.2.2:8080",
        "http://192.168.1.10:8080",
        "http://[::1]:8080",
    ] {
        assert!(backend_origin(value, true).is_ok());
    }
    for value in [
        "http://api.example.test",
        "http://169.254.169.254",
        "http://0.0.0.0",
        "http://8.8.8.8",
    ] {
        assert!(backend_origin(value, true).is_err());
    }
}

#[test]
fn requests_cannot_escape_the_api_or_supply_non_json_bodies() {
    let origin = backend_origin("https://api.example.test", false).unwrap();
    for path in [
        "https://elsewhere.test/api/v1/me",
        "//elsewhere.test/api/v1/me",
        "/api/v1/../../health",
        "/api/v1/%2e%2e/health",
        "/api/v1/me#fragment",
        "/api/v1/media/stream?url=fixture",
        "/api/v1/%2fhealth",
        "/api/v1/me\\extra",
    ] {
        assert!(
            ApiRequest {
                path: path.into(),
                method: "GET".into(),
                body: None
            }
            .validate(&origin)
            .is_err()
        );
    }
    for (method, body) in [
        ("TRACE", None),
        ("POST", Some("not-json".into())),
        ("GET", Some("{}".into())),
    ] {
        assert!(
            ApiRequest {
                path: "/api/v1/me".into(),
                method: method.into(),
                body
            }
            .validate(&origin)
            .is_err()
        );
    }
    assert!(
        ApiRequest {
            path: "/api/v1/search?q=ODESZA%20A%20Moment".into(),
            method: "GET".into(),
            body: None
        }
        .validate(&origin)
        .is_ok()
    );
    assert!(
        ApiRequest {
            path: "/api/v1/external/resolve?url=https%3A%2F%2Fartist.bandcamp.com%2Ftrack%2Fsong"
                .into(),
            method: "GET".into(),
            body: None
        }
        .validate(&origin)
        .is_ok()
    );
}

#[tokio::test]
async fn cookie_stays_in_rust_and_local_session_reset_does_not_need_server_logout() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let origin = format!("http://{}", listener.local_addr().unwrap());
    let server = tokio::spawn(async move {
        for index in 0..3 {
            let (mut stream, _) = listener.accept().await.unwrap();
            let request = request_headers(&mut stream).await.to_lowercase();
            if index == 1 {
                assert!(request.contains("cookie: mixora_session=fixture"));
            }
            if index == 2 {
                assert!(!request.contains("mixora_session=fixture"));
            }
            let cookie = if index == 0 {
                "Set-Cookie: mixora_session=fixture; Path=/api/v1; HttpOnly; SameSite=Lax\r\n"
            } else {
                ""
            };
            stream.write_all(format!("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 2\r\nConnection: close\r\n{cookie}\r\n{{}}").as_bytes()).await.unwrap();
        }
    });
    let backend = Backend::new(&origin, true).unwrap();
    let request = |path: &str| ApiRequest {
        path: path.into(),
        method: "GET".into(),
        body: None,
    };
    let login = backend
        .request(request("/api/v1/auth/login"))
        .await
        .unwrap();
    assert_eq!(login.body, "{}");
    backend.request(request("/api/v1/me")).await.unwrap();
    // Resetting local session must not depend on a successful remote logout.
    backend.clear_session().await.unwrap();
    backend.request(request("/api/v1/me")).await.unwrap();
    server.await.unwrap();
}

async fn fixture_backend_response(response: String) -> (Backend, tokio::task::JoinHandle<()>) {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let origin = format!("http://{}", listener.local_addr().unwrap());
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.unwrap();
        request_headers(&mut stream).await;
        let _ = stream.write_all(response.as_bytes()).await;
    });
    (Backend::new(&origin, true).unwrap(), server)
}

#[tokio::test]
async fn redirect_is_not_followed_or_exposed_to_renderer() {
    let (backend, server) = fixture_backend_response(
        "HTTP/1.1 302 Found\r\nLocation: http://169.254.169.254/fixture-secret\r\nContent-Length: 0\r\nConnection: close\r\n\r\n".into(),
    ).await;
    let error = backend
        .request(ApiRequest {
            path: "/api/v1/me".into(),
            method: "GET".into(),
            body: None,
        })
        .await
        .err()
        .unwrap();
    assert_eq!(error, "API redirect запрещён");
    server.await.unwrap();
}

#[tokio::test]
async fn oversized_response_is_rejected_before_reading_the_body() {
    let (backend, server) = fixture_backend_response(format!(
        "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        MAX_RESPONSE_BYTES + 1,
    ))
    .await;
    let error = backend
        .request(ApiRequest {
            path: "/api/v1/me".into(),
            method: "GET".into(),
            body: None,
        })
        .await
        .err()
        .unwrap();
    assert_eq!(error, "Ответ API слишком большой");
    server.await.unwrap();
}

#[tokio::test]
async fn offline_logout_replaces_the_private_cookie_jar() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap();
    let origin = format!("http://{address}");
    let backend = Backend::new(&origin, true).unwrap();
    let login_server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.unwrap();
        request_headers(&mut stream).await;
        stream.write_all(b"HTTP/1.1 200 OK\r\nSet-Cookie: mixora_session=fixture; Path=/api/v1; HttpOnly\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}").await.unwrap();
    });
    backend
        .request(ApiRequest {
            path: "/api/v1/auth/session".into(),
            method: "GET".into(),
            body: None,
        })
        .await
        .unwrap();
    login_server.await.unwrap(); // listener is now gone; logout cannot connect
    let error = backend
        .request(ApiRequest {
            path: "/api/v1/auth/logout".into(),
            method: "POST".into(),
            body: None,
        })
        .await
        .err()
        .unwrap();
    assert_eq!(error, "Не удалось связаться с сервером");
    let restarted = tokio::net::TcpListener::bind(address).await.unwrap();
    let me_server = tokio::spawn(async move {
        let (mut stream, _) = restarted.accept().await.unwrap();
        let request = request_headers(&mut stream).await;
        assert!(!request.contains("mixora_session=fixture"));
        stream
            .write_all(
                b"HTTP/1.1 401 Unauthorized\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}",
            )
            .await
            .unwrap();
    });
    let response = backend
        .request(ApiRequest {
            path: "/api/v1/me".into(),
            method: "GET".into(),
            body: None,
        })
        .await
        .unwrap();
    assert_eq!(response.status, 401);
    me_server.await.unwrap();
}

#[tokio::test]
#[ignore = "read-only smoke against the running local Mixora backend"]
async fn live_backend_anonymous_session_returns_the_existing_auth_contract() {
    let backend = Backend::new(env!("MIXORA_API_URL"), cfg!(debug_assertions)).unwrap();
    let response = backend
        .request(ApiRequest {
            path: "/api/v1/auth/session".into(),
            method: "GET".into(),
            body: None,
        })
        .await
        .unwrap();
    assert_eq!(response.status, 401);
    let value: serde_json::Value = serde_json::from_str(&response.body).unwrap();
    assert!(value.get("error").is_some());
    assert!(!response.body.contains("mixora_session="));
}
