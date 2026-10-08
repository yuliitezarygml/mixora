# Mixora Mobile — Android / iOS

Tauri 2 + React + Rust, начало мобильной версии существующего Mixora.
Текущий ПК-клиент не удалён и не заменён. Все страницы, плеер, библиотека и
рекомендации импортируются из `../mixora-client/src`; ресурсы и порядок CSS
используются оттуда же. В этой папке нет второй копии UI.

## Единственный файл адресов сервера — `api.json`

```json
{
  "serverUrl": "http://127.0.0.1:8080",
  "androidEmulatorUrl": "http://10.0.2.2:8080",
  "apiPrefix": "/api/v1",
  "playbackSocketPath": "/api/v1/playback/ws"
}
```

- На этом Mac и в iOS-симуляторе используется `serverUrl`.
- В Android-сборке локальный loopback автоматически заменяется адресом
  хоста эмулятора `androidEmulatorUrl`. `127.0.0.1` на телефоне — сам телефон.
- Для настоящего телефона измените `serverUrl` на доступный адрес вашего
  backend. Debug допускает HTTP только для localhost / приватной IPv4-сети;
  release требует HTTPS. Не отключайте проверку сертификатов.
- После изменения адреса пересоберите native-приложение: Rust получает его
  при сборке. Для `npm run dev` перезапустите Vite.
- Пути — контракт существующего backend, не произвольный новый API. Изменять
  их нужно вместе с сервером и общим клиентом. URL музыкальных провайдеров
  приходят от backend и не задаются вручную в конфигурации.
- `src/api.js` — единая JS-точка входа: конфигурация + transport + общий API.
  В `.env` остаются только настройки разработки. Не храните в `api.json`
  пароли, session cookies, private keys и provider credentials. Всё в этом
  файле является публичной конфигурацией приложения.
- Необязательный CI override `MIXORA_API_URL` передаётся через process env;
  обычная разработка не требует дублировать URL в `.env`.

## Запуск

Из корня этой папки:

```sh
npm ci --prefix ../mixora-client
npm ci
npm run dev
```

Web preview: `http://127.0.0.1:5176/#/collection`. Backend должен быть поднят
существующими командами из корня Mixora. Порт 5174 текущего клиента не занят.

```sh
npm run native:dev             # Tauri на Mac, тот же код оболочки
npm run android:apk            # ARM64 debug APK с bundled UI
npm run android:dev            # Android device/emulator + hot reload
npm run ios:simulator          # unsigned iOS simulator archive
npm run ios:dev                # запуск на выбранном симуляторе / устройстве
```

Приложение сразу открывает основной интерфейс: отдельной заставки и кнопки
«Открыть приложение» нет. Ограничения текущего native-этапа перечислены ниже.

Android/iOS проекты уже сгенерированы в `src-tauri/gen`. Команды
`android:init` / `ios:init` нужны для нового checkout без этих проектов,
а не для обычного запуска; повторная генерация может затронуть native edits.

На этом Mac wrapper автоматически выбирает установленный Homebrew JDK 21,
Android SDK и NDK 28.2, если свои `JAVA_HOME`, `ANDROID_HOME`, `NDK_HOME` не
заданы. На других ОС установите prerequisites и задайте их самостоятельно.
Wrapper не меняет shell profile или глобальную конфигурацию компьютера.

Для физического iPhone / App Store потребуются Apple signing certificate и
development team. Неподписанный simulator archive не является готовым IPA.
Android release также требует собственных ключей и HTTPS backend.

## Что действительно работает на этом этапе

- Сборка общего интерфейса, hash navigation и локальные assets.
- Native JSON API bridge с фиксированным origin, cookie jar в Rust,
  ограничением body/response/concurrency, таймаутами и запретом redirect.
- Вход / аккаунт / поиск используют существующие endpoint-ы. Cookie не
  передаётся JS и не записывается в localStorage. Пока сессия только в памяти;
  после закрытия приложения нужно снова войти.
- Browser dev использует прежний same-origin API/WS/audio. В native оболочке
  прямые provider preview / foreground streams используют текущий web engine.
- Authenticated media proxy YouTube / VK / Bandcamp в native пока явно
  возвращает сообщение «подключается нативный плеер», не ложное «играет».
  Native WebSocket намеренно не открывается с чужой WebView cookie jar.
- Фоновый звук и управление с lock screen **ещё не реализованы**. Все пять
  источников в готовом ПК-клиенте этим этапом не ограничиваются.

Дальше — `PLAN.md`: native audio Android Media3 / iOS AVPlayer, Keychain /
Keystore session и authenticated native sync, затем пяти-source journey.

## Проверки

```sh
npm test
npm run build
cargo test --locked --manifest-path src-tauri/Cargo.toml
cargo clippy --locked --manifest-path src-tauri/Cargo.toml --all-targets -- -D warnings
npm test --prefix ../mixora-client
```

Native crate проверяет config при сборке; для release сначала укажите реальный
HTTPS backend в `api.json`. Не подписывайте тестовую сборку production-ключами.

Официальные основы: [Tauri Vite setup](https://v2.tauri.app/start/frontend/vite/),
[mobile prerequisites](https://v2.tauri.app/start/prerequisites/),
[Android background playback](https://developer.android.com/media/media3/session/background-playback),
[iOS audio session](https://developer.apple.com/documentation/avfaudio/avaudiosession/category-swift.struct/playback).
