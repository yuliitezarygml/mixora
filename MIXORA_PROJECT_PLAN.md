# Mixora — генеральный план разработки и миграции

Статус документа: рабочий план, версия 9 от 2026-10-08.

Этот файл — главная точка входа в проект. Он описывает фактическое состояние
локальных исходников, целевую архитектуру, порядок миграции интерфейса, контракт
между клиентом и сервером, инфраструктуру и критерии готовности. План нужно
обновлять вместе с кодом: выполненные пункты отмечаются `[x]`, решения и
отклонения фиксируются в журнале в конце файла.

## 1. Цель продукта

Mixora — настольный и веб-клиент музыкального сервиса с единым аккаунтом,
каталогом из нескольких легальных источников, библиотекой, синхронизацией
воспроизведения и персональной бесконечной подборкой «Моя волна».

Результат лабораторной работы должен быть самостоятельным приложением, а не
копией серверов Яндекс Музыки. Локальный экспорт `yandex-music-src-main`
используется как эталон визуального поведения, структуры экранов и сценариев.
Его закрытые API, аналитика, авторизация и рекомендательные алгоритмы в проект
не переносятся. GitHub и старые удалённые версии не являются источником истины.

## 2. Источник истины и границы каталогов

- `mixora-client/` — основной React/Vite/Electron-клиент Mixora.
- `beckend/` — существующий готовый музыкальный backend/engine и место для
  нового прикладного слоя Mixora. Музыкальную выдачу не переписываем. Имя
  каталога временно сохраняется, чтобы не ломать существующие команды; после
  стабилизации можно отдельно переименовать его в `backend/`.
- `yandex-music-src-main/` — локальный референс интерфейса. Файлы здесь не
  изменяются и не импортируются напрямую в production-код.
- `MIXORA_PROJECT_PLAN.md` — этот план и журнал архитектурных решений.

В рабочем дереве уже находится крупная незакоммиченная реорганизация. Нельзя
восстанавливать или удалять эти изменения через Git без отдельного решения.

## 3. Что есть сейчас

### Клиент

- React 19, Vite 7 и Electron 44.
- Реализована оболочка, маршруты, основные страницы, плеер, HLS, очередь,
  эквалайзер, Media Session, экран авторизации и локальная «волна».
- Большая часть состояния собрана в одном `AppContext.jsx`; это мешает
  независимо развивать авторизацию, каталог, библиотеку и плеер.
- Playback WebSocket и account-scoped сохранение очереди/позиции вынесены в
  `usePlaybackSync` и `usePlayerPersistence`; публичный `useApp()` сохранён.
- Нижний плеер адаптирован по второму скриншоту от 2026-10-08: постоянная
  жёлтая полоса, центрированные transport-кнопки, dislike/like, очередь,
  настройки звука и меню трека. Визуальная проверка: `design-qa.md`.
- В проект перенесены стили и визуальные ресурсы референса, но большинство
  страниц пока являются адаптированными шаблонами, а не полноценными
  пользовательскими сценариями.
- Клиент уже ожидает прикладной API (`/auth`, `/me`, `/library`, `/wave`,
  `/playback/ws`). Музыкальные запросы нужно подключить к фактическому контракту
  готового backend, не вводя новый provider-specific слой.
- Один source-aware adapter уже объединяет SoundCloud, Spotify preview,
  YouTube/YouTube Music и разрешённые внешние ссылки VK/Bandcamp. Поиск VK и
  Bandcamp на этом этапе работает по URL трека, а Spotify воспроизводит preview
  только там, где он доступен. Устойчивая страница источника хранится в
  библиотеке и очереди; временный media URL остаётся внутри серверного
  same-origin proxy и не попадает в persistence клиента.
- Electron использует фиксированный loopback-origin `127.0.0.1:5174` и не
  откатывается на случайный порт. Поэтому cookie и origin-scoped storage
  сохраняются между production-запусками; занятый порт приводит к явной ошибке.

### Сервер

- Существующий Go backend/музыкальный engine считается готовым: он уже отдаёт
  музыку, метаданные, тексты и результаты SoundCloud. Клиенту не требуется
  конструировать SoundCloud/Spotify ID или знать внутренние идентификаторы
  источников.
- Музыкальный engine не является объектом обязательной переделки или
  канонизации. На P2 его фактический контракт фиксируется integration/contract
  тестами и подключается к клиенту через один клиентский API-модуль.
- Новый объём backend-работ — прикладной слой Mixora: пользователи, сессии,
  почта, библиотека, история, события, рекомендации, миграции и фоновые задачи.
- Прикладной слой уже нормализует устаревшие SoundCloud URN/path-like ссылки на
  границе, ведёт provider-neutral каталог для рекомендаций и строит локальные
  text embeddings. Это не меняет контракт готового музыкального engine.
- Если фактические payload готового engine отличаются от текущих ожиданий UI,
  преобразование выполняется в тонкой границе интеграции без изменения самого
  engine. Конкретные расхождения сначала подтверждаются тестом, а не
  предполагаются из структуры SDK.

### Референс Яндекс Музыки

- Это production/export-сборка Next.js/Electron, а не исходный TypeScript/React
  проект: имена компонентов и бизнес-логика уже собраны в минифицированные
  чанки.
- Экспорт полезен для проверки маршрутов, размеров, состояний, анимаций,
  клавиатурного управления, поведения плеера и desktop-интеграции.
- Серверная рекомендательная система, настоящий backend и модель авторизации в
  экспорт не входят.

## 4. Главные архитектурные правила

1. Клиент зависит от стабильного Mixora API и не знает о provider credentials,
   внутренней грамматике URL или способе получения аудио. Устойчивая identity
   сущности — provider-neutral пара `source:id`; она никогда не является
   секретом или временной media-ссылкой.
2. Готовый музыкальный engine остаётся источником музыки, поиска, метаданных и
   текстов. Прикладной backend хранит каноническую пару `source:id` и компактный
   metadata snapshot, который возвращает единый adapter; отдельный provider
   rewrite внутри компонентов или таблиц не планируется.
3. Авторизация серверная: пароль хранится как Argon2id-хеш, сессия — в БД,
   браузер получает случайный `HttpOnly` cookie.
4. Секреты и временные URL аудиопотоков не сохраняются в клиенте. Внешние
   provider credentials, если они вообще нужны внутри engine, не входят в
   публичную конфигурацию прикладного слоя.
   Общий cookies-файл внешнего провайдера запрещён в multi-user API; он может
   быть включён только явно для локальной single-user разработки.
5. Любое действие, влияющее на рекомендации, записывается как событие, а не
   вычисляется только по текущему JSON библиотеки.
6. Рекомендации — гибридный ранжировщик. LLM не нужна для выбора следующего
   трека и не должна находиться в критическом пути воспроизведения.
7. UI мигрируется сценариями и состояниями, а не копированием минифицированных
   JS-чанков.
8. Граница с музыкальным engine имеет таймаут, контролируемые ошибки и
   healthcheck; внутреннее устройство его источников остаётся инкапсулированным.
9. Локальная разработка поднимается одной командой Docker Compose; production
   использует те же контейнеры, но отдельные секреты, TLS и SMTP.
10. Каждый вертикальный сценарий заканчивается тестом и ручной проверкой в
    Electron, а не только существованием экрана.

## 5. Целевая схема системы

```text
React UI / Electron
        |
        | HTTPS + JSON + WebSocket
        v
Mixora App API (новый прикладной слой)
  |-- Auth / Users / Sessions / Email
  |-- Library / Likes / Playlists / History
  |-- Queue sync / Devices
  |-- Events / Recommendation facade / Wave
  |-- Music integration facade
  |
  +--> Existing Music Engine   музыка, поиск, метаданные, тексты, SoundCloud
  +--> PostgreSQL + pgvector   аккаунты, библиотека, события и embeddings
  +--> Redis                   cache, rate limits, короткие блокировки
  +--> Worker                  почта, импорт, пересчёт признаков
  +--> Gorse                   collaborative candidates и ranking signals

Наблюдаемость: structured logs + health/readiness + Prometheus (после MVP).
```

На первом этапе API и worker могут собираться из одного Go-модуля. Разделение
на микросервисы раньше появления реальной нагрузки не требуется.

## 6. План структуры backend

```text
beckend/
  cmd/
    server/          запуск HTTP API
    worker/          фоновые задания
    cli/             диагностика провайдеров
  internal/
    config/          загрузка и валидация окружения
    database/        pool, migrations, health
    auth/            пароль, сессии, verification/reset tokens
    mail/            SMTP и шаблоны писем
    library/         коллекция и плейлисты
    music/           тонкая интеграция с готовым музыкальным engine
    events/          события прослушивания и обратная связь
    recommendation/  кандидаты, фильтры и ранжирование волны
    playback/        синхронизация очереди и устройств
    httpapi/         маршруты, middleware, JSON errors
  migrations/        последовательные SQL-миграции
  pkg/               низкоуровневые внешние SDK
```

Существующие музыкальные пакеты и HTTP-методы остаются рабочим engine. Новый
прикладной слой использует их через узкую границу и не переносит внутрь них
пользователей, сессии, библиотеку или рекомендательные события.

## 7. Данные PostgreSQL

### Учетные записи

- `users`: id, email, password_hash, display_name, email_verified_at, status,
  created_at, updated_at.
- `sessions`: hash случайного токена, user_id, device_id, expires_at,
  last_seen_at, ip/user-agent metadata, revoked_at.
- `email_tokens`: hash одноразового токена, purpose (`verify`/`reset`),
  expires_at, consumed_at.
- `devices`: пользовательские устройства и последнее состояние плеера.
- `outbox`: транзакционные задания почты с повторными попытками.

### Ссылки на музыкальные сущности

- Каталог, музыка, метаданные и тексты принадлежат готовому music engine и не
  дублируются обязательной схемой `tracks`/`track_sources`.
- Пользовательские таблицы хранят каноническую provider-neutral пару
  `source:id`, полученную от единственного music API-adapter. Компоненты не
  разбирают и не собирают provider URL/credentials: adapter нормализует legacy
  SoundCloud URN/path-like значения и приводит Spotify/yt-dlp результаты к
  общей форме перед очередью, API и persistence. Нормализация не рассеяна по
  UI, а временная media URL никогда не сохраняется.
- Для истории и устойчивого UI разрешён небольшой metadata snapshot: название,
  исполнитель, обложка и длительность на момент события.
- `track_catalog`: provider-neutral metadata snapshot, подготовленный текст,
  hash содержимого и состояние durable-задачи построения embedding.
- `track_embeddings`: один активный `vector(768)` на каноническую пару
  source/id с зафиксированным полем версии модели; HNSW cosine-индекс
  используется только для поиска похожих кандидатов, а не как новый источник
  каталога.

### Пользовательские данные

- `user_track_preferences`: реализованное текущее состояние
  `liked`/`disliked`/`neutral` для канонической пары `source/id`, с revision и
  компактным metadata snapshot.
- `user_track_preference_idempotency`: receipt идемпотентной записи; повтор
  возвращает тот же результат, а повтор ключа с другим запросом отклоняется.
- `user_track_preference_outbox`: coalesced durable-публикация последнего
  состояния в recommender. Это не журнал действий и не новый источник музыки.
- `user_track_history` и `user_track_history_idempotency`: нормализованная
  агрегированная история уникальных треков с первым/последним прослушиванием,
  счётчиком и безопасным replay offline-записи. `user_history_state` хранит
  монотонную generation очистки, поэтому старая offline-запись не может
  вернуть историю после clear. Реализованы миграциями `007` и `011`.
- `user_playlists`, `user_playlist_tracks`, `user_playlist_idempotency`:
  аккаунтные плейлисты с упорядоченными уникальными track snapshots и
  идемпотентным полным desired-state обновлением. Каждая запись несёт
  ожидаемую revision; сервер не принимает устаревшую версию и атомарно держит
  продуктовый лимит 50 плейлистов на аккаунт. Реализованы миграциями `008`,
  corrective `009`/`010` для безопасного legacy-import и API-слоем revision.
- `user_artist_follows`, `user_album_state` — следующие нормализованные
  состояния, пока не реализованы.
- `playlist_follows` и `search_history` — следующие нормализованные состояния,
  пока не реализованы.
- `listening_events`: impression, play, listen_30s, complete, skip, repeat,
  seek, like, dislike, add_to_playlist.
- `recommendation_impressions`: что было предложено, модель, позиция и контекст.
- `recommendation_jobs`: версия расчёта и состояние фоновой обработки.

Для первого совместимого API разрешён `user_libraries.payload JSONB` как
переходный snapshot. Миграции `006_track_preferences.sql`,
`007_track_history.sql` и `008_user_playlists.sql` переносят из него валидные
likes/dislikes, историю и собственные плейлисты. После этого snapshot больше не
является источником истины для этих полей. Остальные библиотечные данные
переходят в нормализованные таблицы по мере реализации; события записываются
нормально с самого начала.

`009`/`010` можно применять только пока переходный snapshot ещё содержит
исходный legacy-набор треков: они намеренно не придумывают отсутствующие данные.
При выкладке их нужно применить вместе с сервером до первого transition-write,
который очищает `payload.playlists`; если источник уже утрачен, требуется
отдельный аудит данных, а не разрушительный «ремонт» по догадке.

## 8. API v1 — обязательный контракт

Ответ успешного нового API — прямой JSON-объект. Ошибка:

```json
{
  "error": {
    "code": "invalid_credentials",
    "message": "Неверная почта или пароль",
    "request_id": "..."
  }
}
```

Переходно клиент принимает и строковое `error`, но сервер должен перейти на
структурированные коды.

### Auth и профиль

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `GET  /api/v1/auth/session`
- `POST /api/v1/auth/verify-email`
- `POST /api/v1/auth/resend-verification`
- `POST /api/v1/auth/password/request`
- `POST /api/v1/auth/password/reset`
- `GET  /api/v1/me`
- `PATCH /api/v1/me`
- `GET  /api/v1/me/devices`
- `DELETE /api/v1/me/devices/{id}`

Переключение аккаунта не должно хранить bearer/session token в `localStorage`.
Клиент запоминает только безопасный профиль и повторно запрашивает пароль.

### Библиотека

- `GET /api/v1/library`
- `PUT /api/v1/library` — переходная синхронизация snapshot без likes/dislikes.
- `GET /api/v1/me/track-preferences` — серверное текущее состояние треков.
- `PUT /api/v1/me/track-preferences` — идемпотентная запись
  `liked`/`disliked`/`neutral` с треком из уже полученного engine/UI контекста.
- `GET /api/v1/history` возвращает `{ generation, history }`; `PUT
  /api/v1/me/history` несёт ту же generation и идемпотентную запись. `DELETE
  /api/v1/me/history` повышает generation и возвращает её. Поздняя запись
  старого поколения получает `409 history_generation_conflict`, а клиент
  перезагружает server-owned историю вместо её повторной отправки.
- `GET /api/v1/me/playlists`, `PUT|DELETE /api/v1/me/playlists/{playlistID}` —
  аккаунтные плейлисты и полный порядок треков. PUT/DELETE обязаны нести
  `expected_revision`: создание использует `0`, дальнейшая запись — точную
  revision сервера; stale write получает `409 playlist_revision_conflict`.
  Сервер допускает не более 50 собственных плейлистов. `legacy_id` разрешён
  только при одноразовом backfill и сопоставляется внутри одного аккаунта.
  Маршрут чтения плейлиста готового music engine не заменяется этим API.

### Музыка и воспроизведение

- Поиск, музыка, метаданные, тексты, артист/альбом и воспроизведение используют
  уже существующий контракт music engine.
- На P2 фактические маршруты и payload фиксируются contract tests. Если UI нужна
  единая форма, тонкий app facade или mapper адаптирует ответ без переписывания
  engine и без provider-specific URL в компонентах.
- Клиент передаёт `source:id` и стабильный permalink через единый music
  API-adapter, а способ воспроизведения определяется source-specific adapter-ом.
  SoundCloud legacy, Spotify и yt-dlp provider normalization изолированы там,
  а не повторяются в компонентах. Для YouTube/VK/Bandcamp клиент использует
  защищённый сессией same-origin media proxy: extractor остаётся на сервере,
  direct media URL не раскрывается и не сохраняется клиентом, а Range-запросы
  ограничены по размеру, времени и параллелизму с проверкой допустимых адресов.

### События и рекомендации

- `POST /api/v1/events` — пачка событий с idempotency id.
- `POST /api/v1/wave` — новая пачка треков и `session_id`.
- `POST /api/v1/wave/{sessionId}/feedback`.
- `GET /api/v1/recommendations/home`.
- `GET /api/v1/playback/ws` — синхронизация устройств.

## 9. Авторизация и безопасность

- Argon2id с параметрами не слабее базовой рекомендации OWASP; параметры
  измеряются на реальном сервере и сохраняются рядом с хешем.
- Случайный session token минимум 256 бит; в БД хранится только SHA-256.
- Cookie: `HttpOnly`, `SameSite=Lax`, `Secure` в production, ограниченный path,
  ротация после входа и чувствительных операций.
- Единообразные ответы login не раскрывают существование email.
- Rate limit на register/login/reset и публичные ресурсоёмкие endpoints.
- Ограничение размера JSON body, таймауты HTTP, проверка методов и content type.
- Проверка Origin/CSRF для cookie-auth mutating requests перед production.
- Секреты только через environment/secret manager, никогда в репозитории.
- SMTP в разработке идёт в Mailpit, в production — в выбранный SMTP-провайдер.
- Удаление аккаунта и экспорт данных проектируются до публичного запуска.

## 10. «Моя волна» — рекомендательная система

### Почему не одна нейросеть

Качественная музыкальная лента состоит из генерации кандидатов, фильтров,
ранжирования и исследования новых интересов. Большая языковая модель для этого
не нужна. Для первой версии используется локальный Gorse + собственные правила;
для content similarity — pgvector и embeddings. Всё работает на имеющемся
Apple M2/24 GB без CUDA.

### Сигналы

- Сильные положительные: like, повтор, добавление в плейлист, завершение.
- Слабые положительные: 30 секунд прослушивания, поиск и открытие артиста.
- Нейтральные/read: impression, короткий preview.
- Отрицательные: явный dislike/«не рекомендовать».
- Skip — контекстный сигнал, не безусловный dislike: ранний skip весит сильнее,
  поздний может означать нормальное завершение интереса.
- Явные like/dislike дополнительно имеют server-side desired state. Для трека,
  у которого оно задано, оно имеет приоритет над устаревшими одноимёнными
  событиями при фильтрации и построении вкуса.

### Генерация кандидатов

1. Collaborative filtering по действиям похожих пользователей.
2. Item-to-item по совместным прослушиваниям.
3. Похожие жанры, артисты и теги текущего трека.
4. Content embeddings текста; позднее — аудио embeddings (например, CLAP).
5. Популярное с затуханием по времени.
6. До 100 свежих доступных provider-neutral snapshots `track_catalog`
   (SoundCloud, Spotify preview, YouTube, Bandcamp, VK) как независимый
   fallback/novelty pool.
7. Контролируемая доля exploration для новых треков, артистов и sources.

### Ранжирование и ограничения

- Фильтр недоступного/explicit-контента и явных dislikes.
- Запрет повтора трека в сессии и ограничение подряд одного артиста.
- Учет activity, mood, language и режима familiar/discovery.
- Баланс 70–85% уверенных кандидатов и 15–30% exploration.
- Логирование impression обязательно: без него нельзя правильно оценить CTR и
  качество рекомендаций.

### Этапы ML

- V0: текущие правила + кандидаты готового music engine, серверно и с event
  logging.
- V1: Gorse в Docker, implicit feedback и durable-проекция событий.
- V2a (реализовано): pgvector + локальные text embeddings EmbeddingGemma,
  серверный taste-vector, query cold-start и гибрид Gorse/content/rules.
- V2b (следующий этап): audio embeddings, измеряемая exploration и offline
  evaluation; аудио-модель не включается до проверки пользы на реальных данных.
- V3: собственный learning-to-rank/two-tower только при достаточном объёме
  реальных событий и наличии offline/online метрик.

Метрики: completion rate, early-skip rate, likes per 100 impressions, diversity,
новизна, доля недоступных треков, latency p95. «Количество кликов» само по себе
не является достаточной метрикой качества.

## 11. Миграция интерфейса

Миграция проводится не по количеству URL, а по законченным сценариям. Для
каждого сценария сохраняются reference screenshot/описание, состояния loading,
empty, error, offline, keyboard и responsive, после чего пишется реализация на
компонентах Mixora.

### Общая дизайн-система

- [ ] Инвентаризация цветов, типографики, spacing, radii, shadows, z-index.
- [ ] Собственные семантические design tokens без хешированных Yandex-классов.
- [ ] Button, IconButton, Menu, Dialog, Tooltip, Skeleton, EmptyState.
- [ ] MediaCard, TrackRow, Shelf/Carousel, EntityHeader.
- [ ] Стабильный Shell: sidebar, content, player bar, modals, toasts.
- [ ] Темы dark/light и `prefers-reduced-motion`.
- [ ] Focus states, screen reader labels и управление с клавиатуры.

### Сценарий A: старт и авторизация

- [x] Есть визуальные экраны login/register/switch.
- [x] Реальный register/login/logout/session.
- [x] Backend verify email и password reset через SMTP outbox.
- [x] Добавить формы UI для подтверждения email и password reset.
- [~] Controlled loading/error/offline для каталога, источников, Wave и
  плеера реализованы; остаётся полный ручной audit форм и desktop-сценария.
- [x] Безопасное переключение аккаунта без токена в localStorage.
- [ ] Splash показывается только до готовности session + initial data.

### Сценарий B: поиск → карточка → проигрывание

- [x] Есть поисковый экран и базовый плеер.
- [x] Зафиксировать текущие payload поиска/playback unit-тестами mapper-а.
- [x] Подключить UI через единый API-модуль без provider credentials.
- [x] Разделить progressive/HLS по ответу готового engine.
- [~] Фактическое воспроизведение в браузере проверено для SoundCloud, YouTube,
  VK и Bandcamp, а также Spotify preview там, где он доступен; packaged Electron
  воспроизвёл YouTube Official Audio. Полный desktop smoke пяти источников
  ещё не выполнен. Для VK/Bandcamp текущий вход — разрешённый URL трека.
- [x] Обработать недоступность engine, сети и конкретного трека на уровне
  transport/proxy и автоматических тестов; полный ручной desktop-audit остаётся.
- [x] Записывать impression/play/30s/complete/skip, repeat-one и значимые seek.
- [~] Browser smoke для `next`, паузы/перемотки, очереди и перестановки треков
  пройден. React integration-тесты покрывают удаление, previous/repeat/shuffle,
  resume и синхронизацию; полный ручной desktop-сценарий остаётся.

### Сценарий C: библиотека и плейлисты

- [x] Есть локальная библиотека и оптимистичный UI.
- [x] Переходная серверная синхронизация snapshot библиотеки.
- [x] Нормализованное серверное состояние likes/dislikes/neutral: migration
  из snapshot, идемпотентный API, offline-очередь на аккаунт и durable Gorse
  outbox.
- [x] Нормализованная история и её UI/API: `007`, GET/PUT/DELETE,
  account-scoped offline-очередь и серверный source для Wave.
- [x] CRUD плейлистов и порядок треков: `008`/`009`, полная desired-state
  запись, account-scoped очередь и одношаговое «создать и добавить трек».
- [x] Защита от тихой потери данных между устройствами: history generation,
  playlist revision, rebase очереди одного устройства, `409` + загрузка
  актуального серверного состояния.
- [ ] Пользовательский выбор/field-level merge двух одновременно изменённых
  плейлистов, если такой UX окажется нужен продукту.
- [ ] Пустые состояния и восстановление после offline.

### Сценарий D: сущности каталога

- [ ] Страница трека.
- [ ] Страница артиста + tracks/albums/similar.
- [ ] Страница альбома.
- [ ] Страница плейлиста пользователя и редакции.
- [ ] Жанры, чарты, новые релизы.

### Сценарий E: Моя волна

- [x] Есть UI настроек и локальный heuristic fallback.
- [x] Серверная Wave V0 и базовый event logging.
- [x] Gorse candidate generation.
- [x] Локальные text embeddings, server-side taste и cold-start по запросу.
- [x] Optimistic like/dislike/«Вернуть» с текущим серверным состоянием;
  skip и feedback Wave остаются событиями.
- [x] Бесконечная дозагрузка и безопасное восстановление Wave-сессии после
  перезапуска: сохраняется только компактная привязка показанного трека к
  server-validated session, без cookie и URL потока.
- [x] Короткое объяснение активного пути: персонализация или rules fallback.
- [ ] Объяснение причины для каждого отдельного трека.

### Сценарий F: desktop

- [x] Стабильный origin вместо случайного порта.
- [x] Локальная неподписанная macOS ARM64 `.app` через `npm run desktop:pack`;
  backend запускается отдельно. Подписанный установщик пока не готов.
- [ ] Custom protocol и deep links для desktop-ссылок из писем.
- [ ] Один экземпляр приложения и deep links.
- [ ] Tray/system media controls.
- [ ] Безопасный preload IPC с явным allowlist.
- [ ] Запуск/поиск backend либо настройка удалённого HTTPS API.
- [ ] Подпись, auto-update и crash reporting — только после MVP.

## 12. Управление состоянием клиента

`AppContext` делится постепенно, без big-bang переписывания. Уже вынесены
browser-library persistence, listener event queue, а также независимые
server/offline synchronizers для preferences, history и playlists; внешний
контракт `useApp()` при этом сохранён. Playback WebSocket и persistence также
выделены в самостоятельные hooks с проверками same-track reorder, remote pause,
account isolation и сохранения позиции перед закрытием:

- `AuthProvider`: session, profile, login/register/logout.
- `LibraryProvider`: likes, playlists, history, offline queue mutations.
- `CatalogProvider`: queries, normalized entities, cache.
- `PlayerProvider`: audio element, queue, HLS, Media Session.
- `WaveProvider`: session, preferences, feedback, pagination.
- `SettingsProvider`: theme, quality, explicit, accessibility.

Сначала из контекста выносятся чистые API-клиенты и reducer/domain functions,
затем providers. Это позволяет сохранять рабочий UI между этапами.

## 13. Docker и окружения

Development Compose:

- `postgres`: образ с pgvector.
- `redis`: cache/rate limits.
- `mailpit`: SMTP `1025`, web UI `8025`.
- `api`: Go API `8080`, выполняет migrations перед readiness.
- Фоновая durable-проекция feedback работает внутри API и дочитывает backlog.
- `gorse-in-one 0.5.11`: single-node профиль `recommendations`; split на
  master/server/worker нужен только при доказанной нагрузке.
- `ollama`: локальный CPU runtime в профиле `embeddings`; одноразовый
  `ollama-pull` загружает закреплённую q4-модель EmbeddingGemma, а API включает
  фоновую индексацию только при заданном `MIXORA_EMBEDDINGS_URL`.
- Клиент обычно запускается Vite локально для HMR; production image добавляется
  после стабилизации API.

Текущие команды разделяют лёгкий и полный режим: `make recommendations`
поднимает Gorse без нейросети, `make embeddings` дополнительно запускает Ollama,
загружает модель и включает content-рекомендации. При недоступности embeddings
Wave продолжает работать через Gorse/rules fallback.

В Compose уже есть `.env.example`, healthchecks и named volumes; секреты по
умолчанию не хранятся в репозитории. Provider credentials не являются частью
app-layer `.env`: music engine сам инкапсулирует свою готовую выдачу. Production
не должен публиковать Postgres/Redis/Mailpit наружу.

Для текущего рабочего каталога создан реальный корневой `.env`: в нём явно
заданы порты и подключения PostgreSQL, Redis, Mailpit, API и Gorse. Compose
читает его автоматически, а `.gitignore` запрещает случайно добавить файл с
локальными или production-секретами в Git. `.env.example` остаётся только
синхронизированным шаблоном для новой машины.

## 14. Тестовая стратегия

- Go unit: password/session, validation, event weights и ranking.
- Go integration: чистая PostgreSQL, migrations, auth/library/event endpoints.
- Music integration: contract/regression tests готового engine без проверки его
  внутренних provider ID.
- Contract tests: JSON, status codes и cookie behavior для запросов клиента.
- React unit: чистые reducers/mappers/routing.
- Component tests: auth/search/player/wave states.
- E2E: Playwright для шести сценариев из раздела 11.
- Desktop smoke: запуск production build, стабильное хранение, deep link,
  клавиши Media Session.
- Performance: API p95, cold/warm wave latency, memory during long playback.

Перед завершением этапа обязательны format, vet/lint, unit tests, build и
ручной smoke test. Недоступность music engine проверяется на границе app API.

## 15. Этапы выполнения

### P0 — зафиксировать основу

- [x] Провести локальный аудит трёх частей проекта.
- [x] Создать единый план и целевую архитектуру.
- [x] Добавить `.env.example`, root Compose, Dockerfile и команды запуска.
- [x] Создать migrations и database bootstrap.
- [x] Ввести versioned app API без поломки готового music engine.

Критерий: `docker compose up` даёт healthy Postgres/Redis/Mailpit/API.

### P1 — учетная запись и библиотека

- [x] Register/login/logout/session на Argon2id + cookie.
- [x] Verification mail и password reset через outbox.
- [x] `/me`, server library snapshot и нормализованные events.
- [x] Текущее состояние likes/dislikes/neutral с migration legacy snapshot,
  идемпотентностью и offline-синхронизацией клиента.
- [x] Подключить существующий AuthModal к реальному API.
- [x] Убрать session token из localStorage.

Критерий: новый пользователь регистрируется, видит письмо в Mailpit,
перезапускает клиент и остаётся в сессии; библиотека синхронизируется.

### P2 — integration verification готового music engine

- [x] Инвентаризировать фактические маршруты и payload поиска, музыки,
  метаданных, текстов и воспроизведения.
- [x] Закрепить рабочий контракт contract/regression тестами без переделки
  engine и без конструирования provider ID в app layer.
- [x] Подключить один API/mapper клиента к фактическому контракту.
- [x] Сделать source-aware клиентский adapter для SoundCloud, Spotify,
  YouTube/YouTube Music, VK и Bandcamp: source входит в ключ сущности,
  history/плейлисты и player snapshot сохраняют permalink, а Spotify в
  браузере воспроизводит только легально доступное preview.
- [x] Ограничить yt-dlp границей авторизации и allowlist-ом разрешённых HTTPS
  URL; внедрить timeout и лимит параллельных subprocess-ов. Внешнее аудио
  отдаётся через authenticated same-origin proxy с bounded Range, MIME/DNS/IP
  проверками, redirect validation и лимитами размера/времени/параллелизма.
  Named Bandcamp endpoint принимает только `/track/…`; album import остаётся
  будущей задачей.
- [x] Проверить timeout, unavailable track, offline и controlled error states:
  transport нормализует network/HTTP failures, yt-dlp не отдаёт stderr клиенту,
  каталог показывает source-aware recovery, а playback retry заново получает
  временную media URL.
- [~] Search → play в браузере пройден для SoundCloud, YouTube, VK, Bandcamp и
  доступного Spotify preview. Browser `next → like → reopen`, создание
  плейлиста, добавление трека и reload также пройдены; весь packaged Electron
  сценарий ещё не пройден.

Критерий: существующая выдача музыки и SoundCloud подтверждена тестами и
проходит сквозной сценарий; app backend добавляет пользовательские данные, не
заменяя музыкальный engine.

### P3 — события и Wave V0/V1

- [x] Event API и отправка основных событий клиента; Wave session ownership,
  обязательный idempotency key и защита от повторного feedback в одной выдаче.
- [x] Добавить offline-буфер и повторную отправку событий клиента.
- [x] Wave V0 на правилах и кандидатах готового engine.
- [x] Поднять Gorse в отдельном Compose profile.
- [x] Экспорт пользователей/items/feedback и hybrid ranking.
- [x] Durable Gorse outbox для текущего like/dislike/neutral: последнее
  состояние coalesced, `neutral` снимает оба feedback-сигнала.
- [x] Отправка поисковых запросов и impressions результатов поиска.
- [x] Диагностическое объяснение выбранного model path в UI.
- [x] Дать Wave provider-neutral fallback из свежего `track_catalog`, чтобы
  в каждом запросе смешивались до 100 ранее наблюдённых nonblocked
  SoundCloud/Spotify preview/YouTube/Bandcamp/VK-кандидатов. Такие
  fallback-записи не обновляют собственную свежесть при каждой выдаче; если
  collaborative page целиком одного source, до 10% позиций резервируются для
  доступных новых source из catalog.
- [ ] Метрики качества и причины ranking для каждого трека.

Критерий: два пользователя с разной историей получают разные подборки;
dislike исключает трек, early skip влияет мягко.

### P4 — UI migration и дизайн-система

- [x] Вынести browser-library persistence, listener event queue и durable
  synchronizers preferences/history/playlists из `AppContext`, сохранив его
  как совместимый фасад для UI.
- [x] Выделить playback WebSocket и player persistence; добавить React
  integration-тесты очереди, resume, account isolation и feedback Wave.
- [x] Пересобрать нижний плеер по выбранному скриншоту и проверить desktop/mobile.
- [ ] Продолжить разделение оставшихся auth/catalog/player/Wave-координаторов
  без одновременной замены публичного `useApp()`-контракта.
- [ ] Заменить хешированные reference-классы semantic-компонентами.
- [ ] Реализовать сценарии C/D и состояния ошибок.
- [ ] Visual regression на ключевых размерах.
- [ ] Accessibility pass.

Критерий: интерфейс воспроизводит нужное поведение референса, но поддерживается
как самостоятельный код Mixora.

### P5 — рекомендации V2 и качество каталога

- [x] Канонизировать legacy SoundCloud URN/path-like refs на границах API,
  событий, Wave и существующих user/event записей БД; catalog reads/writes
  также приводят остаточные legacy keys к канонической паре.
- [x] Добавить provider-neutral metadata snapshots, hash содержимого и durable
  очередь пересчёта без повторной индексации неизменившихся треков.
- [x] Добавить pgvector, 768-мерные text embeddings и HNSW cosine lookup.
- [x] Поднять локальную EmbeddingGemma q4 через Ollama/Compose и подключить
  безопасный Gorse/content/rules fallback.
- [x] Строить server-side taste из истории пользователя и query cold-start для
  пользователя без достаточной истории.
- [x] Давать сохранённому desired state приоритет над устаревшими
  `like`/`dislike`-событиями в Wave и content taste.
- [x] Перед публикацией preference feedback в Gorse сверять external `source:id`
  с server-side catalog: в Gorse попадает только подтверждённый catalog snapshot,
  а неизвестный `liked`/`disliked` client snapshot остаётся локальным desired
  state и не становится recommendation item. Неизвестный `neutral` удаляет
  только возможный старый Gorse feedback без создания item.
- [x] После первого наблюдения подтверждённого track автоматически
  переотправлять ранее catalog-unverified `liked`/`disliked` preference;
  нормальные source-responses каталогизируются до клика, а offline/legacy
  состояние получает отдельную durable reconciliation-метку.
- [x] Применить такую же server-side catalog-проверку к general listening events
  до их Gorse-проекции: неизвестное событие остаётся в PostgreSQL, но не может
  создать Gorse item; Wave feedback дополнительно связан с server-side impression.
- [ ] Добавить и оценить CLAP/audio embeddings.
- [ ] Cold-start onboarding в UI и управляемая exploration.
- [ ] Offline evaluation и A/B-ready assignment.

### P6 — desktop/production

- [ ] Стабильный app protocol/origin, IPC и deep links.
- [ ] Production reverse proxy/TLS/SMTP/secrets/backups.
- [ ] Перед внешним доступом добавить trusted-origin CSRF-проверку для
  cookie-auth mutation и настроить reverse proxy так, чтобы Origin не терялся.
- [ ] Спроектировать per-user external-provider authorization; общий
  yt-dlp cookies-file не является production-решением.
- [ ] Подпись установщика, обновления и release checklist.
- [ ] Privacy/export/delete account и лицензии источников.

## 16. Что не делаем сейчас

- Не копируем минифицированные чанки Яндекса в исходники Mixora.
- Не пытаемся воспроизвести закрытый алгоритм рекомендаций один в один.
- Не строим Kubernetes и набор микросервисов до рабочего single-node MVP.
- Не обучаем большую модель до накопления качественных событий.
- Не переписываем готовую музыкальную выдачу и не переносим provider-specific
  IDs/credentials в клиент или новый app layer.
- Не делаем выводы о внутренних источниках music engine без отдельного теста;
  его лицензионные и эксплуатационные ограничения проверяются отдельно перед
  production-публикацией.

## 17. Ближайший рабочий порядок

Базовые P0–P3, text-часть P5, текущее состояние likes/dislikes, история и
аккаунтные плейлисты уже реализованы.
Следующая последовательность:

Срез от 2026-10-08 выполнен: карточки коллекции ограничены по ширине;
«Для вас», «Открытия», «Для работы» появились на главной и в коллекции.
Используется существующий `/wave` с server-side history/preferences/Gorse/
embeddings. Есть пересчёт по вкусовым сигналам, изоляция аккаунтов,
loading/error/empty, обновление, просмотр, play и сохранение snapshot.
Лайки и сохранения из открытого preview тоже связаны с impression session.
Рабочий `.env` подключён к уже установленной локальной EmbeddingGemma.
Проверены размеры desktop/mobile, 122 client tests, Go recommendation/
embedding/httpapi tests и browser play/save/reload. Это функциональный этап,
а не подтверждённая оценка качества музыки: малый подходящий каталог всё ещё
может давать пересекающиеся rule-only подборки; UI сообщает об этом.

Дизайн-срез по пользовательскому образцу от 11:12 также выполнен:
подпись заголовка, оригинальная обложка-сердце, реальный счётчик избранного,
две колонки треков и круглые исполнители; рекомендации перенесены ниже них.
Убрано влияние старой фиолетовой карточки на список избранного. Подборки имеют
локальные обложки и ограниченную ширину; ошибка внешней картинки больше не
оставляет невидимое место. Проверены desktop/mobile и новая Mac ARM64-сборка,
126/126 client tests; отчёт и исключения — в `design-qa.md`.

1. Повторить уже подтверждённые browser Search → play и
   next → like → playlist → reopen в packaged Electron. Для VK/Bandcamp
   проверять URL-based вход и отдельно фиксировать недоступный Spotify preview;
   дополнительно закрыть UI queue/previous/repeat/shuffle.
2. Продолжить разделение `AppContext`: playback sync/persistence и durable
   library уже выделены; следующие узкие модули — auth/catalog/Wave.
3. Добавить метрики recommendation quality/latency, offline evaluation и
   объяснение причины для отдельного трека.
4. Реализовать управляемую exploration и cold-start onboarding в UI.
5. Исследовать CLAP/audio embeddings только после сравнения с уже работающими
   text embeddings на накопленных событиях.
6. Закрыть desktop deep links/IPC и production TLS/SMTP/secrets/backups,
   including trusted-origin CSRF и per-user provider auth design.

Метрики, полноценные error/offline-сценарии, пользовательский merge
одновременных правок и production-подготовка остаются незавершёнными и не
считаются закрытыми наличием работающей Wave.

## 18. Журнал решений

- 2026-09-29: локальные каталоги являются единственным источником анализа;
  GitHub намеренно не используется.
- 2026-09-29: референс Яндекса принят как visual/behavior reference, но не как
  переносимый backend или доступный исходный код компонентов.
- 2026-09-29: основной runtime backend — Go + PostgreSQL/pgvector + Redis.
- 2026-09-29: базовый recommender — Gorse; pgvector/content embeddings — второй
  слой, собственная тяжёлая модель откладывается до появления данных.
- 2026-09-29: по уточнению владельца существующий music backend уже полностью
  отдаёт музыку, метаданные, тексты и SoundCloud; его не переписываем.
- 2026-09-29: provider ID и credentials не являются частью контракта клиента и
  нового app layer; P2 заменён на integration verification готового engine.
- 2026-09-29: P0 и backend-часть P1 подняты в Docker и проверены smoke-сценарием:
  register, login, session, Mailpit, verify email, library, event, search и Wave
  V0 работают совместно; музыкальные пакеты при этом не переписывались.
- 2026-09-29: app API закреплён автоматическим HTTP/WebSocket contract suite;
  тесты работают через узкие fake-зависимости и не требуют изменения готового
  музыкального engine.
- 2026-09-29: production Electron использует стабильный loopback-origin
  `127.0.0.1:5174`; случайный порт и потеря origin-scoped состояния исключены.
- 2026-09-29: клиент получил полный UI-поток verify email/password reset и
  устойчивую offline-очередь рекомендательных событий с idempotency keys.
- 2026-09-29: окно «Настроить Мою волну» визуально сверено с локальным
  референсом: убрана лишняя внутренняя панель, восстановлены сетки 3+2,
  цветные character-иконки, mood-градиенты и адаптивный mobile layout.
- 2026-09-30: поднят локальный Gorse 0.5.11 в Compose profile
  `recommendations`; music engine не изменён и остаётся источником треков.
- 2026-09-30: добавлены provider-neutral каталог треков, журнал impressions и
  durable-проекция событий PostgreSQL → Gorse с идемпотентным `PUT` агрегатов.
- 2026-09-30: клиент связывает feedback с `wave session_id`, немедленно
  отправляет ключевые сигналы и сохраняет их в offline-очереди до подтверждения.
- 2026-09-30: live smoke подтвердил переход `rules-v0` →
  `gorse-v1+rules-v0`: 51 item, 60 impressions, feedback `like` и нулевой
  backlog экспорта. Реальный `.env` настроен, согласован и исключён из Git.
- 2026-09-30: поиск пишет отдельное событие запроса и ограниченный набор
  impressions видимых результатов; волна показывает пользователю, сработала
  персональная модель или безопасный rules fallback.
- 2026-09-30: добавлены миграции pgvector и канонических track refs, локальный
  Ollama с `embeddinggemma:300m-qat-q4_0`, durable batch-индексация metadata и
  HNSW cosine-поиск. Server-side taste строится по событиям, а cold-start — по
  embedding запроса; клиентские seed не могут отравить общий каталог.
- 2026-09-30: полный live smoke профиля `embeddings` подтвердил healthy
  API/Gorse/PostgreSQL/Redis/Mailpit/Ollama, 51 из 51 построенных embeddings,
  отсутствие ошибок и backlog, а Wave вернула путь
  `gorse-v1+embeddinggemma-q4-768-doc-v1+rules-v0`. Реальный `.env` настроен и
  остаётся исключённым из Git; CLAP/audio embeddings и продуктовые метрики ещё
  не реализованы.
- 2026-09-30: Wave impressions сохраняются атомарно до ответа клиенту; feedback
  требует настоящий показанный трек, UUID-сессию и idempotency key. Добавлен
  server-side receipt, который не даёт повторно усилить один Wave-сигнал новым
  ключом; клиент пишет repeat-one и значимые seek.
- 2026-10-03: likes/dislikes переведены в отдельное текущее desired state
  `liked`/`disliked`/`neutral`: migration переносит валидный legacy snapshot,
  HTTP API и клиент используют идемпотентные записи, а offline-очередь на
  аккаунт оставляет последний выбор. Coalesced PostgreSQL outbox публикует его
  в Gorse с retry/backoff; neutral удаляет оба feedback-сигнала. Wave и
  content taste читают это состояние раньше устаревших одноимённых событий.
  Целевые Go unit/HTTP contract и клиентские unit tests прошли; Docker smoke
  подтвердил публикацию liked → neutral в Gorse и пустой outbox.
- 2026-10-03: migration `007_track_history.sql` выделила историю из snapshot в
  агрегированное server-side состояние. Wave теперь читает durable историю
  прежде временного клиентского overlay.
- 2026-10-03: migrations `008_user_playlists.sql` и corrective `009`/`010`
  выделили собственные плейлисты и их порядок из snapshot. Коррекция
  переимпортирует только legacy-плейлисты с revision `1`, не завися от
  изменяемого времени всего snapshot. Клиент пишет полный desired state с
  idempotency key, переносит каждый ещё не представленный legacy-плейлист и
  сохраняет новый плейлист с первым треком в одной операции. Реальный Docker
  smoke подтвердил migration, replay, reorder и delete через API.
- 2026-10-04: `011_user_history_generation.sql` добавила durable clear fence:
  GET/PUT/DELETE истории согласованы через generation, поэтому поздний offline
  listen не отменяет очистку. Для плейлистов добавлены optimistic revision,
  атомарный лимит 50 и account-scoped legacy mapping; конфликт другой машины
  явно возвращает `409`, а не перезаписывает её desired state. Клиент хранит
  provider-канонизацию в одном music API-adapter и не дублирует SoundCloud
  grammar по компонентам.
- 2026-10-04: клиент сохраняет короткое account-scoped продолжение «Моей
  волны»: preferences/context/round и mapping `source:id → session_id`.
  После перезапуска восстанавливаются только mappings треков из сохранённой
  очереди, а сервер всё равно проверяет ownership через impressions. Очередь
  плеера и desktop WebSocket теперь всегда передают окно, содержащее текущий
  трек, даже после 30-й позиции.
- 2026-10-04: source-aware adapter расширен на Spotify preview, YouTube/YouTube
  Music и разрешённые VK/Bandcamp URL. Сохранённые external треки переживают
  reload/desktop sync по permalink и повторно получают временный media URL
  только на play; прямой URL и provider cookies не сохраняются.
- 2026-10-04: Wave получила provider-neutral fallback из `track_catalog` и
  миграцию `012_wave_catalog_candidates.sql`. Catalog-кандидаты не
  перезаписываются при выдаче, поэтому recent window не замыкается на себе.
  yt-dlp/Spotify Connect resource endpoints защищены Mixora session, URL
  allowlist проверяет provider-specific маршруты, а extractor имеет bounded
  timeout/concurrency. Shared yt-dlp cookies запрещены вне явно отмеченной
  local development конфигурации.
- 2026-10-04: live API E2E показал, что полный Gorse page из SoundCloud может
  визуально вытеснить catalog fallback. Wave теперь детерминированно сохраняет
  до 10% page для доступных ранее отсутствовавших provider sources, по одному
  треку на source; это ограниченная source-diversity exploration, а не отказ
  от персонального Gorse ranking.
- 2026-10-04: controlled failure/offline слой закрыт автоматическими тестами:
  клиент различает отсутствие сети, session, rate limit, недоступный источник
  и неверную ссылку без показа browser/provider diagnostics. Плеер повторно
  разрешает permalink, а не использует истёкшую CDN URL; fallback Wave явно
  показывает локальную подборку. yt-dlp timeout теперь сохраняет typed cause и
  возвращает 502/503/504 с безопасным сообщением вместо stderr процесса.
- 2026-10-04: preference outbox перед публикацией `liked`/`disliked` в Gorse
  повторно сверяет `source:id` с `track_catalog` и использует только его
  metadata snapshot. Неизвестная запись остаётся локальным выбором и
  terminal-skip для Gorse; `neutral` безопасно удаляет возможный legacy
  feedback без создания item, а временная ошибка каталога ретраится. Все
  успешные SoundCloud track/resolve/chart/related/playlist-ответы теперь также
  каталогизируются до пользовательской реакции.
- 2026-10-05: general event projector получил ту же catalog-проверку перед
  Gorse. Подтверждённые события создают item только из server-side metadata,
  неизвестные сохраняются в журнале продукта, но terminal-skip для Gorse;
  недоступный catalog оставляет batch retryable.
- 2026-10-05: migration `013_preference_catalog_reconciliation.sql` добавила
  durable marker для positive preference, пришедшего раньше catalog-наблюдения.
  После появления подтверждённого `source:id` outbox сам сбрасывает terminal
  skip и повторно публикует актуальное состояние; обычные delivered записи не
  переигрываются.
- 2026-10-05: `AppContext` начал безопасно распадаться на узкие state-модули:
  browser library, listener events, preferences, history и playlists. Очереди,
  user-id fences и внешний фасад не менялись; player/Wave/auth остаются
  следующими отдельными этапами.
- 2026-10-05: browser smoke подтвердил поиск и воспроизведение SoundCloud,
  YouTube, VK, Bandcamp и доступного Spotify preview через единый клиентский
  adapter. VK/Bandcamp пока принимают URL трека, Spotify зависит от наличия
  preview. Для YouTube/VK/Bandcamp добавлен authenticated same-origin media
  proxy: direct extractor URL не попадает в клиент, bounded Range и upstream
  ответы проверяются, SSRF/special-use адреса, небезопасные redirect/MIME,
  зависшие соединения и избыточный параллелизм ограничены. На момент этого
  первого прохода next → like → reopen и packaged Electron ещё не проверялись.
- 2026-10-05: повторный YouTube smoke выявил, что Alpine-пакет `yt-dlp`
  2025.11.12 отдавал рабочий первый byte-range, но получал `403` при глубокой
  перемотке. API image переведён на закреплённый стабильный `yt-dlp` 2026.8.19
  из PyPI с `yt-dlp-ejs` и Node.js runtime. После пересборки same-origin proxy
  вернул `206` для диапазона с ненулевым start, а браузер продолжил трек с
  3:30 до 3:48 без playback error. Обновление версии требует повторного smoke
  пяти источников, а не слепого снятия pin.
- 2026-10-05: browser stateful smoke подтвердил сохранение лайка после reload,
  переход `Next` на следующий доступный YouTube-трек, создание аккаунтного
  плейлиста, добавление YouTube-трека и сохранение содержимого после повторной
  загрузки страницы. Полная desktop-проверка и queue/previous/repeat/shuffle
  остаются отдельными пунктами.

- 2026-10-08: playback WebSocket и persistence вынесены из `AppContext`.
  Исправлены reorder без изменения длины очереди, remote pause/seek после
  загрузки metadata, немедленное сохранение track/queue и изоляция аккаунтов.
  Like/dislike Wave отправляют immediate feedback; отказ localStorage не
  показывается как успешное сохранение.
- 2026-10-08: нижний плеер перестроен по второму пользовательскому скриншоту.
  Убрана hover-плашка, добавлены постоянная full-width жёлтая полоса, dislike,
  рабочее меню трека, настройки звука и действия очереди. Browser smoke:
  SoundCloud «Утро» играет, pause/seek и reorder работают, меню/эквалайзер
  открываются. Desktop/mobile visual QA пройден; 105/105 client tests,
  format/styles/build и неподписанная ARM64 `.app` собраны. YouTube Official
  Audio ранее воспроизведён в packaged Electron; полный пяти-source desktop
  journey остаётся, это не означает готовность всего проекта.
- 2026-10-08: исправлено растягивание карточек коллекции длинными metadata:
  первая карточка имела 1574 px вместо ограниченного размера; после исправления
  все artist cards 180 px (desktop) / 148 px (mobile), page overflow отсутствует.
  Добавлены три рекомендуемых плейлиста, общая загрузка для Home/Collection,
  фильтры всех пяти источников, отмена устаревших запросов и session feedback.
  На тестовом аккаунте «Проверка регистрации» сохранён snapshot «Для вас»
  из 13 треков: воспроизведение и сохранность после reload подтверждены.
  Исправлен crash компонента до загрузки аккаунта и добавлен regression test.
  Runtime API ранее не имел embeddings URL: рабочий `.env` исправлен,
  Docker API перезапущен, local embeddings enabled; индекс 236/236 на момент
  проверки. Фактический rule-only ответ при текущих языковых ограничениях
  не маскируется под ML. 122/122 client tests и проверки формата/стилей пройдены;
  неподписанный Mac ARM64-клиент пересобран. Метрики качества и полный
  пяти-source packaged journey остаются открытыми пунктами.

- 2026-10-08: коллекция переработана по последнему пользовательскому скриншоту
  (11:12): оригинальное сердце, подпись «У вашей музыки есть цвет», реальный
  счётчик, двухколоночный список и исполнители перед рекомендациями. Отдельный
  CSS-класс устранил конфликт со старой фиолетовой карточкой шириной 500 px.
  Рекомендации ограничены 220 px desktop / 160 px mobile и используют локальные
  assets; внешний artwork при ошибке заменяется видимой иконкой. Добавлены
  четыре регрессии, всего 126/126 client tests. Format/styles/build и Mac ARM64
  packaging прошли; приложение открыто, пауза и позиция 176.2 s восстановлены.
  QA охватывает этот дизайн-срез, а не полную готовность сервиса; качество
  metadata исполнителей и пяти-source packaged journey остаются отдельно.

## 19. Definition of Done всего проекта

Проект считается законченным, когда новый пользователь может в чистой среде
одной документированной командой поднять backend, открыть клиент, создать и
подтвердить аккаунт, найти и воспроизвести доступный трек, управлять библиотекой
и плейлистами, получить персональную бесконечную волну, продолжить с другого
устройства и корректно пережить временную недоступность music engine. Все эти
сценарии должны иметь автоматические проверки, а production-конфигурация — не
содержать тестовых паролей, открытых БД или секретов в репозитории.
