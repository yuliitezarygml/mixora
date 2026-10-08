use super::*;
use tokio::io::{AsyncReadExt, AsyncWriteExt};

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
