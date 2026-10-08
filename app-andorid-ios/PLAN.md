# Mixora Android / iOS — рабочий план

Выбор подтверждён пользователем 2026-10-08: **Tauri 2 + существующий React**.
Имя `app-andorid-ios` сохранено точно как в запросе. GitHub не используется.

## Архитектура

- `../mixora-client/src` и `public` — единый интерфейс, плеер, библиотека и
  рекомендации. Не копируем приложение и не переписываем готовый Go backend.
- Эта папка — отдельная точка входа, Vite build и Rust/Tauri-оболочка.
- Native API transport — узкий Rust-модуль: фиксированный адрес сервера,
  собственная cookie-сессия, ограниченные JSON-запросы. Cookie не выдаётся JS.
- Native audio — отдельный будущий модуль: Android Media3 + MediaSessionService,
  iOS AVPlayer + AVAudioSession. Web Audio не считается фоновым native-плеером.

## Этапы и критерии приёмки

### M1. Рабочая оболочка и подключение к API (текущий этап)

- [ ] Общий React UI без второй копии страниц и ресурсов; hash-routing в сборке.
- [ ] Отдельные команды web / Tauri / Android / iOS и lock-файлы.
- [ ] Настоящий локальный `.env`, пример и безопасная проверка адреса API.
- [ ] Rust transport: session cookie, JSON, logout, таймауты, ограничения URL,
      отсутствие произвольных headers/credentials/redirect и утечки ошибок.
- [ ] Нативная сессия пока только в памяти; это явно указано пользователю.
- [ ] Native HTTP-сессия не подменяется browser-cookie. Неподключённые
      authenticated media и WebSocket явно отмечены, а не считаются рабочими.
- [ ] Regression tests общего клиента, transport tests, web build, Rust checks.
- [ ] Генерация Android/iOS проектов и проверка доступного toolchain.

### M2. Главный риск — музыка при заблокированном экране

- [ ] Общий `PlayerEngine` interface; существующий HTMLAudio engine сохранён.
- [ ] Android Media3, foreground service и notification / lock-screen controls.
- [ ] iOS AVPlayer, audio-session interruptions / routes и Now Playing controls.
- [ ] Безопасная передача session в media engine без токена в URL/localStorage.
- [ ] Воспроизведение, seek, next / previous, Bluetooth, incoming call,
      background / screen lock на настоящих Android и iPhone.
- [ ] SoundCloud, YouTube, VK, Bandcamp, доступный Spotify preview; недоступные
      provider-потоки объясняются честно. URL-поиск VK/Bandcamp не назван каталогом.

### M3. Аккаунт и продолжение между устройствами

- [ ] Keychain / Keystore для сохранения native session, expiry и logout.
- [ ] Authenticated native WebSocket и sync без двух одновременно играющих устройств.
- [ ] Регистрация → вход → поиск → playback → like → playlist → reopen.
- [ ] Backend HTTPS для реального телефона, deep links подтверждения/сброса пароля.
- [ ] Персональные рекомендации обновляются по реальным server-side событиям.

### M4. Распространение

- [ ] Android signing / AAB и iOS signing / archive (ключи не в репозитории).
- [ ] Иконки, safe-area, доступность, network failure / low battery QA.
- [ ] Privacy / provider restrictions review и release checklist.

Сборка оболочки не означает выполнение M2–M4. Старый Electron-клиент остаётся
рабочим до полной приёмки нового native плеера.
