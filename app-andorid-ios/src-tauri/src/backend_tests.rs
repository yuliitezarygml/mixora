use super::*;
use tokio::io::{AsyncReadExt, AsyncWriteExt};

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
async fn cookie_stays_in_rust_and_logout_clears_it_even_when_server_is_unavailable() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let origin = format!("http://{}", listener.local_addr().unwrap());
    let server = tokio::spawn(async move {
        for index in 0..3 {
            let (mut stream, _) = listener.accept().await.unwrap();
            let mut bytes = vec![0; 8192];
            let length = stream.read(&mut bytes).await.unwrap();
            let request = String::from_utf8_lossy(&bytes[..length]).to_lowercase();
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
