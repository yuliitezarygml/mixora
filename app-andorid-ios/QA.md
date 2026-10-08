# Mobile M1 — проверка 2026-10-08

## Подтверждено

- `npm ci` воспроизводимо устанавливает pinned dependencies из lock-файла.
- 11/11 mobile JS tests: IPC contract, cancellation, 204, pending native media,
  единый API facade/config, shared 43 stylesheets, toolchain wrapper и dev startup.
- 129/129 общих client tests: существующий web/Electron transport остаётся
  default, native adapter установлен отдельно; provider-neutral playback
  проходит через новый seam без изменения browser-логики.
- 7/7 обычных Rust tests: origin validation, debug-only private HTTP,
  request path/method/body boundaries, private cookie jar, reset session,
  offline logout, запрет redirect и bounded response.
- Отдельно выполнен ignored read-only live test `/api/v1/auth/session`:
  работающий Go backend возвращает 401 / JSON без cookie в renderer payload.
- `cargo clippy --all-targets --locked -- -D warnings`, Rust fmt, общий Prettier
  и SHA-256 проверка 43 reference stylesheets проходят.
- Vite production build проходит. Старое предупреждение размера HLS chunk
  (~508 kB) остаётся; это не ошибка сборки и не оценка mobile performance.
- `npm run android:apk`: ARM64 bundled debug APK собран с JDK 21 / NDK 28.2.
- `npm run ios:simulator`: unsigned archive собран; Mach-O `LC_BUILD_VERSION`
  проверен, платформа **IOSSIMULATOR**, minimum iOS 16.0. Это не device IPA.
- Mac Tauri debug `.app` открыта через native UI: hash navigation работает,
  поиск ODESZA без native-сессии корректно требует вход, не использует
  чужую browser-cookie и не выдаёт fixture catalog за результат поиска.
- Browser preview на `4176/#/collection`, ширина 390 × 844: общая коллекция,
  мобильная навигация, обложки и нижний плеер отображаются; ошибок JS нет.
  Browser использует отдельный существующий тестовый аккаунт, это **не**
  подтверждение native login или mobile playback.
- Исправлен чёрный экран `native:dev`: Vite proxy `/api` перехватывал
  `/api.json?import` и возвращал backend 404 вместо JS-модуля. Теперь proxy
  ограничен `/api/`; `api.json` остаётся единственным файлом server settings.
  Новый тест с настоящим Vite и изолированным HTTP backend сначала воспроизвёл
  404, после исправления проверяет загрузку модуля и проксирование auth route.
  Правило общее для dev и preview. Подробности prefix matching:
  [Vite server.proxy](https://vite.dev/config/server-options#server-proxy).
- Промежуточный native intro удалён по запросу пользователя. Проверен прямой
  старт основного UI в macOS WebView на `5176`: тот же dev-бинарник временно
  помещён в ignored QA `.app` для доступа UI-проверки. Главная и плеер видны
  сразу, без кнопки «Открыть приложение». В browser dev ошибок JS не найдено.
  Это не проверка background audio или login на физическом телефоне.

## Артефакты (ignored build output)

- Android: `src-tauri/gen/android/app/build/outputs/apk/universal/debug/app-universal-debug.apk`
- iOS simulator: `src-tauri/gen/apple/build/mixora-mobile_iOS.xcarchive`
- Mac QA shell: `src-tauri/target/debug/bundle/macos/Mixora Mobile.app`
- Responsive screenshot: `qa/collection-phone.png`
- Native direct startup screenshot: `qa/native-direct-start.png`

## Не подтверждено / не завершено

- Физические Android/iPhone: register/login/play/like/playlist/reopen journey.
- Native background playback, lock-screen controls, Bluetooth/interruption QA.
- Authenticated media proxy YouTube/VK/Bandcamp в native: явно pending.
- Native WebSocket и secure persisted session (Keychain/Keystore).
- iOS certificate/team, Android release keys, production HTTPS backend.
- Windows/Linux builds, production mobile perf, store/privacy review.

Tauri `ios init` установил необходимый `libimobiledevice` через Homebrew;
его dependencies включали обновления `ca-certificates` и `openssl@3`.
Профиль shell, текущий backend и Electron-клиент не менялись.

Никакие пользовательские session tokens/пароли не выводились и не добавлялись
в `api.json`. Обычный `.env` содержит только публичные dev settings; приватные
локальные настройки следует хранить в ignored `.env.local`, не коммитить.
