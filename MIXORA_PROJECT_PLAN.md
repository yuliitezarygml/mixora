# Mixora — генеральный план разработки и миграции

Статус документа: рабочий план, версия 2 от 2026-09-29.

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

- React 19, Vite 7 и Electron 36.
- Реализована оболочка, маршруты, основные страницы, плеер, HLS, очередь,
  эквалайзер, Media Session, экран авторизации и локальная «волна».
- Большая часть состояния собрана в одном `AppContext.jsx`; это мешает
  независимо развивать авторизацию, каталог, библиотеку и плеер.
- В проект перенесены стили и визуальные ресурсы референса, но большинство
  страниц пока являются адаптированными шаблонами, а не полноценными
  пользовательскими сценариями.
- Клиент уже ожидает прикладной API (`/auth`, `/me`, `/library`, `/wave`,
  `/playback/ws`). Музыкальные запросы нужно подключить к фактическому контракту
  готового backend, не вводя новый provider-specific слой.
- Electron использует случайный локальный порт для production-сборки. Из-за
  origin-scoped storage это может ломать устойчивость локального состояния.

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
   внутренних source ID и способе получения аудио.
2. Готовый музыкальный engine остаётся источником музыки, поиска, метаданных и
   текстов. Прикладной backend хранит только устойчивую ссылку `track_ref`,
   которую возвращает engine; отдельный provider rewrite не планируется.
3. Авторизация серверная: пароль хранится как Argon2id-хеш, сессия — в БД,
   браузер получает случайный `HttpOnly` cookie.
4. Секреты и временные URL аудиопотоков не сохраняются в клиенте. Внешние
   provider credentials, если они вообще нужны внутри engine, не входят в
   публичную конфигурацию прикладного слоя.
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
- Пользовательские таблицы хранят непрозрачный `track_ref`, полученный от
  engine. Клиент не разбирает и не собирает его по правилам SoundCloud/Spotify.
- Для истории и устойчивого UI разрешён небольшой metadata snapshot: название,
  исполнитель, обложка и длительность на момент события.
- `track_embeddings(track_ref, embedding vector(...), model_version)`
  добавляется позже как индекс рекомендаций, а не как новый источник каталога.

### Пользовательские данные

- `user_track_state`: track_ref, like/dislike/hidden с датами.
- `user_artist_follows`, `user_album_state`.
- `playlists`, `playlist_tracks`, `playlist_follows`.
- `search_history`.
- `listening_events`: impression, play, listen_30s, complete, skip, repeat,
  seek, like, dislike, add_to_playlist.
- `recommendation_impressions`: что было предложено, модель, позиция и контекст.
- `recommendation_jobs`: версия расчёта и состояние фоновой обработки.

Для первого совместимого API разрешён `user_libraries.payload JSONB` как
переходный snapshot. После стабилизации интерфейса данные постепенно переходят
в нормализованные таблицы; события записываются нормально с самого начала.

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
- `PUT /api/v1/library` — переходная синхронизация snapshot.
- `PUT/DELETE /api/v1/me/tracks/{trackId}/like`
- `PUT/DELETE /api/v1/me/tracks/{trackId}/dislike`
- CRUD `/api/v1/playlists` и изменение порядка треков.
- `GET /api/v1/history`.

### Музыка и воспроизведение

- Поиск, музыка, метаданные, тексты, артист/альбом и воспроизведение используют
  уже существующий контракт music engine.
- На P2 фактические маршруты и payload фиксируются contract tests. Если UI нужна
  единая форма, тонкий app facade или mapper адаптирует ответ без переписывания
  engine и без provider-specific URL в компонентах.
- Клиент обращается с непрозрачным `track_ref` и использует заявленный engine
  способ воспроизведения; он не строит SoundCloud/Spotify ID самостоятельно.

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

### Генерация кандидатов

1. Collaborative filtering по действиям похожих пользователей.
2. Item-to-item по совместным прослушиваниям.
3. Похожие жанры, артисты и теги текущего трека.
4. Content embeddings текста и позднее аудио embeddings (например, CLAP).
5. Популярное с затуханием по времени.
6. Контролируемая доля exploration для новых треков и артистов.

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
- V1: Gorse в Docker, implicit feedback, cold-start по метаданным.
- V2: pgvector + локальные text/audio embeddings, гибридное ранжирование.
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
- [ ] Добавить формы UI для подтверждения email и password reset.
- [ ] Loading/error/offline состояния.
- [ ] Безопасное переключение аккаунта без токена в localStorage.
- [ ] Splash показывается только до готовности session + initial data.

### Сценарий B: поиск → карточка → проигрывание

- [x] Есть поисковый экран и базовый плеер.
- [x] Зафиксировать текущие payload поиска/playback unit-тестами mapper-а.
- [x] Подключить UI через единый API-модуль без provider credentials.
- [x] Разделить progressive/HLS по ответу готового engine.
- [ ] Проверить фактическое воспроизведение в браузере/Electron.
- [ ] Обработать недоступность engine, сети и конкретного трека.
- [ ] Записывать impression/play/30s/complete/skip.
- [ ] Проверить очередь, next/previous/repeat/shuffle.

### Сценарий C: библиотека и плейлисты

- [x] Есть локальная библиотека и оптимистичный UI.
- [x] Переходная серверная синхронизация snapshot библиотеки.
- [ ] Нормализованная серверная синхронизация likes/dislikes/history.
- [ ] CRUD плейлистов и порядок треков.
- [ ] Разрешение конфликтов нескольких устройств.
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
- [ ] Gorse candidate generation.
- [ ] Dislike/skip/like feedback без задержки UI.
- [ ] Бесконечная дозагрузка и восстановление сессии.
- [ ] Объяснимые короткие причины рекомендации для отладки.

### Сценарий F: desktop

- [ ] Стабильный custom protocol/origin вместо случайного порта.
- [ ] Один экземпляр приложения и deep links.
- [ ] Tray/system media controls.
- [ ] Безопасный preload IPC с явным allowlist.
- [ ] Запуск/поиск backend либо настройка удалённого HTTPS API.
- [ ] Подпись, auto-update и crash reporting — только после MVP.

## 12. Управление состоянием клиента

Текущий `AppContext` делится постепенно, без big-bang переписывания:

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
- `worker`: добавляется при подключении outbox/Gorse.
- `gorse-master`, `gorse-server`, `gorse-worker`: профиль recommendations.
- Клиент обычно запускается Vite локально для HMR; production image добавляется
  после стабилизации API.

Нужны `.env.example`, healthchecks, named volumes и отсутствие секретов по
умолчанию. Provider credentials не являются частью app-layer `.env`: music
engine сам инкапсулирует свою готовую выдачу. Production не публикует
Postgres/Redis/Mailpit наружу.

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
- [x] Подключить существующий AuthModal к реальному API.
- [x] Убрать session token из localStorage.

Критерий: новый пользователь регистрируется, видит письмо в Mailpit,
перезапускает клиент и остаётся в сессии; библиотека синхронизируется.

### P2 — integration verification готового music engine

- [x] Инвентаризировать фактические маршруты и payload поиска, музыки,
  метаданных, текстов и воспроизведения.
- [ ] Закрепить рабочий контракт contract/regression тестами без переделки
  engine и без конструирования provider ID в app layer.
- [x] Подключить один API/mapper клиента к фактическому контракту.
- [ ] Проверить timeout, unavailable track, offline и controlled error states.
- [ ] Пройти Search → play → next → like → reopen в браузере и Electron.

Критерий: существующая выдача музыки и SoundCloud подтверждена тестами и
проходит сквозной сценарий; app backend добавляет пользовательские данные, не
заменяя музыкальный engine.

### P3 — события и Wave V0/V1

- [x] Event API и отправка основных событий клиента.
- [ ] Добавить offline-буфер и повторную отправку событий клиента.
- [x] Wave V0 на правилах и кандидатах готового engine.
- [ ] Поднять Gorse в отдельном Compose profile.
- [ ] Экспорт пользователей/items/feedback и hybrid ranking.
- [ ] Метрики качества и диагностические причины ranking.

Критерий: два пользователя с разной историей получают разные подборки;
dislike исключает трек, early skip влияет мягко.

### P4 — UI migration и дизайн-система

- [ ] Разбить AppContext.
- [ ] Заменить хешированные reference-классы semantic-компонентами.
- [ ] Реализовать сценарии C/D и состояния ошибок.
- [ ] Visual regression на ключевых размерах.
- [ ] Accessibility pass.

Критерий: интерфейс воспроизводит нужное поведение референса, но поддерживается
как самостоятельный код Mixora.

### P5 — рекомендации V2 и качество каталога

- [ ] Проверка устойчивости `track_ref` и metadata snapshots music engine.
- [ ] pgvector, text embeddings, затем CLAP audio embeddings.
- [ ] Cold-start onboarding и управляемая exploration.
- [ ] Offline evaluation и A/B-ready assignment.

### P6 — desktop/production

- [ ] Стабильный app protocol/origin, IPC и deep links.
- [ ] Production reverse proxy/TLS/SMTP/secrets/backups.
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

1. P0: Compose, конфигурация, migrations, health/readiness.
2. P1: auth/session/mail/library.
3. P2: contract tests и интеграция уже готового music engine с клиентом.
4. Подключить события клиента до дальнейшей миграции UI.
5. Реализовать Wave V0, затем Gorse.
6. Мигрировать оставшиеся страницы вертикальными сценариями.
7. Стабилизировать Electron и production deployment.

Такой порядок важен: красивый экран без работающего контракта придётся
переписывать, а раннее логирование событий даст реальные данные для волны.

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

## 19. Definition of Done всего проекта

Проект считается законченным, когда новый пользователь может в чистой среде
одной документированной командой поднять backend, открыть клиент, создать и
подтвердить аккаунт, найти и воспроизвести доступный трек, управлять библиотекой
и плейлистами, получить персональную бесконечную волну, продолжить с другого
устройства и корректно пережить временную недоступность music engine. Все эти
сценарии должны иметь автоматические проверки, а production-конфигурация — не
содержать тестовых паролей, открытых БД или секретов в репозитории.
