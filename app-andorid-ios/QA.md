# Mobile foundation / foreground audio / taste — проверка 2026-10-08

## Подтверждено

- `npm ci` воспроизводимо устанавливает pinned dependencies из lock-файла.
- 12/12 mobile JS tests: IPC contract, cancellation, 204, opaque media grants,
  единый API facade/config, shared 43 stylesheets, toolchain wrapper и dev startup.
- 134/134 общих client tests: существующий web/Electron transport остаётся
  default, native adapter установлен отдельно; provider-neutral playback
  проходит через новый seam без изменения browser-логики. Onboarding deferred
  Wave, profile context и late-response/account isolation также проверены.
- 15/15 обычных Rust tests: origin validation, debug-only private HTTP,
  request path/method/body boundaries, private cookie jar, reset session,
  offline logout, запрет redirect и bounded response; media grant/private cookie,
  Host/GET/HEAD/TTL, full/open/suffix ranges, целый multi-chunk stream и bounded
  retry. Один отдельный live smoke остаётся ignored при обычном `cargo test`.
- Отдельно выполнен ignored read-only live test `/api/v1/auth/session`:
  работающий Go backend возвращает 401 / JSON без cookie в renderer payload.
- `cargo clippy --all-targets --locked -- -D warnings`, Rust fmt, общий Prettier
  и SHA-256 проверка 43 reference stylesheets проходят.
- Vite production build проходит. Старое предупреждение размера HLS chunk
  (~508 kB) остаётся; это не ошибка сборки и не оценка mobile performance.
- `npm run android:apk`: ARM64 bundled debug APK собран с JDK 21 / NDK 28.2.
- `npm run ios:simulator`: unsigned archive собран; Mach-O `LC_BUILD_VERSION`
  проверен, платформа **IOSSIMULATOR**, minimum iOS 16.0. Это не device IPA.
- APK и iOS simulator archive пересобраны после текущих media/taste изменений.
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

## Новый нативный пользователь и музыка

- Живой Docker API обновлён без удаления volumes; migration 014 применена,
  `/health` и `/ready` возвращают 200. `go test ./...` проходит, включая taste
  validation, authenticated routes, account isolation и cold-start ranking.
- На отдельном QA-аккаунте macOS Tauri: вход → автоматически открывшийся
  выбор интересов → пять артистов и electronic → сохранение → отложенная Wave.
  Profile повторно прочитан с сервера, completed=true, лайки не сфабрикованы.
  После перезапуска и повторного входа onboarding не появляется повторно.
- Отдельный live cold-start API smoke: пять артистов с первым Tycho, Wave200,
  причина/первый подходящий трек Tycho. Это проверка влияния профиля, не
  доказательство высокого качества рекомендаций для всех музыкальных вкусов.
- Исправлена не только заглушка media transport, но и частичные ответы:
  backend отдаёт по 1 МБ, а WebView должен получить полную длину/Range.
  Первые варианты с custom URI и единичным HTTP chunk давали короткий clip,
  повтор или неверную длительность; они заменены настоящим HTTP streaming.
- YouTube `ODESZA - A Moment Apart - Official Audio`: 3:54 в macOS Tauri,
  прогресс 1:42 после первой порции, seek 2:57, продолжение до 3:03 и pause.
  Отдельный ранее проверенный upstream давал 403 → backend502: это не
  маскируется как успешное воспроизведение. Новый мост повторяет 502/504 один
  раз на том же Range; постоянная недоступность остаётся ошибкой.
- SoundCloud remix: foreground воспроизведение и прогресс подтверждены;
  ограниченный preview не объявляется полной песней.
- Bandcamp `Disasterpeace - Compass` через URL-based вход: resolve → play,
  реальная длительность 2:44, прогресс 1:39 и pause на 1:44 в macOS Tauri
  подтверждены.
- VK: общий media transport покрыт fixture-контрактом, live native playback
  с реальным доступным треком **не проверен**. Spotify: preview contract
  сохранён, полный каталог/полная музыка не заявлены.

## Артефакты (ignored build output)

- Android: `src-tauri/gen/android/app/build/outputs/apk/universal/debug/app-universal-debug.apk`
- iOS simulator: `src-tauri/gen/apple/build/mixora-mobile_iOS.xcarchive`
- Mac QA shell: `src-tauri/target/debug/bundle/macos/Mixora Mobile.app`
- Current native dev QA wrapper: `qa/Mixora Mobile Dev QA.app`
- Responsive screenshot: `qa/collection-phone.png`
- Native direct startup screenshot: `qa/native-direct-start.png`
- Native five-artist selection: `qa/taste-onboarding-native.png`
- Reopened persisted profile: `qa/taste-profile-reopened-native.png`
- Native YouTube after seek/pause: `qa/native-youtube-playback.png`
- Native Bandcamp after pause: `qa/native-bandcamp-playback.png`

## Не подтверждено / не завершено

- Физические Android/iPhone: register/login/play/like/playlist/reopen journey.
- Native background playback, lock-screen controls, Bluetooth/interruption QA.
- Полный пяти-source native journey: отдельные реальные треки/ограничения
  провайдеров, codecs/ATS на iOS и phone network policy ещё требуют device QA.
- Native WebSocket и secure persisted session (Keychain/Keystore).
- iOS certificate/team, Android release keys, production HTTPS backend.
- Windows/Linux builds, production mobile perf, store/privacy review.

Tauri `ios init` установил необходимый `libimobiledevice` через Homebrew;
его dependencies включали обновления `ca-certificates` и `openssl@3`.
Профиль shell и Electron-клиент не менялись. Backend текущего среза обновлён
для taste API/migration; music engine/источники не заменены.

Никакие пользовательские session tokens/пароли не выводились и не добавлялись
в `api.json`. Обычный `.env` содержит только публичные dev settings; приватные
локальные настройки следует хранить в ignored `.env.local`, не коммитить.
