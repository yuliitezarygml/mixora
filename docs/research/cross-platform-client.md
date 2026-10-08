# Mixora: выбор клиента для ПК, Android и iOS

Дата проверки: 2026-10-08. Статус: исследование и рекомендация, не принятое
решение о миграции. Production-код, зависимости и генеральный план не изменены.
Исследование выполнено без агентов, в соответствии с пожеланием владельца.

## Вывод

Если главный приоритет — общий интерфейс на ПК и телефонах с сохранением
уже выполненной работы, кандидат для проверки — **React + Tauri 2**.
Для музыкального приложения это не просто смена упаковки: мобильный плеер
следует вынести в нативный слой и проверить его независимо от WebView.
Это инженерная рекомендация, а не подтверждение готовности мобильной версии.

Если приоритет — именно нативный мобильный интерфейс, альтернативой является
React Native для Android/iOS при сохранении отдельного desktop/web-клиента.
Capacitor — ещё один web-first вариант для мобильной упаковки существующего
React-клиента; он не заменяет desktop-оболочку на всех трёх настольных ОС.
Сравнение стоимости здесь качественное: сроки, RAM и размер сборок не измерялись.

## Проверенные факты

1. Текущий клиент использует React, Vite, Electron и hls.js —
   [package.json](../../mixora-client/package.json). Desktop-оболочка поднимает
   локальный HTTP proxy к `127.0.0.1:8080` —
   [main.cjs:37](../../mixora-client/desktop/main.cjs#L37).
   Текущий аудиоплеер создаёт browser `Audio`, использует HLS и Media Session —
   [AppContext.jsx:1148](../../mixora-client/src/state/AppContext.jsx#L1148),
   [AppContext.jsx:1253](../../mixora-client/src/state/AppContext.jsx#L1253).
   Это не уже готовый Android foreground media service или iOS audio session.
2. Tauri 2 заявляет Linux, macOS, Windows, Android и iOS, допускает существующий
   web frontend, использует Rust и интеграции Swift/Kotlin —
   [официальная страница Tauri 2](https://v2.tauri.app/).
3. Tauri отображает frontend через платформенные WebView. Поэтому общий
   React/HTML/CSS не означает использование нативных UI-контролов и одинаковый
   браузерный движок на всех устройствах —
   [WebView versions](https://v2.tauri.app/reference/webview-versions/).
4. Мобильные плагины Tauri могут выполнять Kotlin/Java и Swift; документация
   отдельно обсуждает вызов общего Rust-кода при suspended WebView —
   [mobile plugin development](https://v2.tauri.app/develop/plugins/develop-mobile/).
   Это возможность интеграции, не готовый музыкальный background-player.
5. React Native создаёт платформенные компоненты: например, `View` соответствует
   Android ViewGroup/iOS UIView, а не браузерному `div` —
   [native components](https://reactnative.dev/docs/intro-react-native-components).
   **Вывод для Mixora:** данные, правила и часть React hooks можно выделить
   для повторного использования, но DOM-компоненты и существующие CSS нужно
   адаптировать, а не просто перенести.
6. Capacitor можно добавить к существующему JavaScript-проекту, его официальные
   targets — Android, iOS и Web, с доступом к native API через плагины —
   [introduction](https://capacitorjs.com/docs),
   [environment setup](https://capacitorjs.com/docs/getting-started/environment-setup).
7. Android Media3 рекомендует размещать Player/MediaSession в отдельном
   MediaSessionService для фонового воспроизведения; нужны соответствующие
   foreground-service permissions и manifest configuration —
   [Android background playback](https://developer.android.com/media/media3/session/background-playback).
8. На iOS категория AVAudioSession playback вместе с audio background mode
   обеспечивает условия для продолжения звука при блокировке экрана/переходе
   в фон —
   [Apple playback category](https://developer.apple.com/documentation/avfaudio/avaudiosession/category-swift.struct/playback).
   Управление очередью, interruptions, metadata и remote commands остаётся
   отдельной работой, не решается одним выбором оболочки.

## Предлагаемое разделение ответственности

- Общие React UI и логика библиотеки/поиска/рекомендаций остаются в клиенте.
- Tauri/Rust — оболочка и узкие системные интеграции, не переписывание Go API.
- На Android — native media service/player через плагин; на iOS — native
  player/audio session через плагин. Web UI отправляет команды и отображает
  состояние, а не является единственным владельцем фоновой очереди.
- Готовый Go backend, PostgreSQL и рекомендательная инфраструктура остаются
  сервером; исходная точка входа —
  [cmd/server/main.go](../../beckend/cmd/server/main.go).

Это проектное предложение. Точный выбор native-player API, плагина и поддержка
всех текущих stream formats требуют отдельного прототипа и device tests.
Наличие готового подходящего Tauri audio plugin в этой проверке не подтверждено.
Конкретная React Native audio-библиотека также не выбрана: документация
React Native Track Player по запрошенной странице не была доступна, и её
совместимость/поддержка не утверждаются.

## Что обязательно проверить до одобрения миграции

1. Один настоящий трек и очередь на Android/iPhone: блокировка экрана,
   фон, next/previous из системного плеера, Bluetooth, звонок и возврат в UI.
2. Реальные SoundCloud HLS, YouTube/VK/Bandcamp media proxy и Spotify preview;
   cookies/авторизация native media requests, Range и истекающие URL.
3. API base URL/HTTPS, cookie session и origin/CSRF при изменении runtime.
   Текущий localhost-proxy привязан к ПК. Из конфигурации
   [desktop/main.cjs](../../mixora-client/desktop/main.cjs) следует, что телефон
   нельзя подключить к PC backend, просто скопировав эти loopback адреса.
4. Offline/outbox и синхронизация позиции без зависимости от работающего
   JS-таймера; события и очередь должен сохранять фактический владелец playback.
5. Desktop: macOS/Windows/Linux WebView, codecs, audio и browser API, а также
   сохранность библиотеки при смене origin и localStorage.

Нельзя считать пунктами миграции «готово» только появление APK/IPA или запуск
экрана. До подтверждённого прототипа текущий Electron-клиент сохраняется.
