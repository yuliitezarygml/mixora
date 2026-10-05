# Mixora API и музыкальный engine (SoundCloud + Spotify + YouTube / Bandcamp / VK)

Готовый музыкальный движок на Go остаётся отдельным слоем SDK, REST-маршрутов и CLI. `cmd/server` теперь также поднимает app-layer Mixora: аутентификацию и сессии, библиотеку пользователя, события, рекомендации, синхронизацию плеера, PostgreSQL-миграции и почтовый outbox. Неизвестные app-layer маршруты передаются в существующий музыкальный router.

Движок объединяет три источника:
1. **SoundCloud v2** (реверс-инжиниринг с авто-скрапингом `client_id`)
2. **Spotify & Librespot** (Spotify Connect приемник, удаленное управление и Zero-Config парсинг)
3. **Внешние источники на базе `yt-dlp`**: allowlist для YouTube / YouTube
   Music, отдельных треков Bandcamp и публичных media-страниц VK/VK Video.

---

## 🌟 Ключевые возможности

### 🟠 1. SoundCloud v2:
- **Авто-скрапинг `client_id`**: без регистрации Developer App и без API-ключей.
- **Прямые CDN-ссылки**: Progressive MP3 (128 kbps) и HLS (Opus / MP3).
- **Синхронизированные караоке-тексты (LRC)**: точные таймкоды строк в миллисекундах.
- **Звуковая волна (Waveform)**: сэмплы амплитуд для визуализации в плеере.
- **Чарты и тренды**: выборки New & Hot по жанрам.

### 🟢 2. Spotify & Librespot Connect:
- **Zero-Config парсинг**: получение информации о треках, альбомах, исполнителях и плейлистах без ключей разработчика.
- **Прямой MP3-стриминг (Preview)**: 30-секундные MP3 превью-потоки без авторизации.
- **Spotify Connect (librespot / go-librespot integration)**:
  - Автономный программный спикер Spotify Connect в локальной сети (mDNS/Zeroconf).
  - Удаленное управление плеером через локальный REST API (`/player/play`, `/pause`, `/volume`, `/next`, `/prev`, `/seek`, `/load`).
  - Статус воспроизведения в реальном времени (`/status`), мониторинг текущего трека и громкости.
  - Детекция и автозапуск локального демона `go-librespot` или Rust `librespot`.
- **Синхронизированные тексты песен**: поддержка таймкодов караоке через LRCLIB.

### 🔴 3. Universal Extractor (`yt-dlp`):
- **YouTube & YouTube Music**:
  - Поиск через `ytsearch` без квот Google Cloud API.
  - `yt-dlp` запрашивает лучший доступный audio-only формат; формат, bitrate,
    скорость и доступность определяет провайдер, поэтому они не гарантируются
    контрактом Mixora.
- **Bandcamp**:
  - Разрешены только страницы вида `https://<artist>.bandcamp.com/track/<slug>`.
  - Metadata и временная media-ссылка выдаются после server-side resolve;
    доступность зависит от публичности страницы. Import альбомов/треклистов
    пока не является контрактом API.
- **ВКонтакте (VK)**:
  - Разрешены только публичные URL поддерживаемых media-форматов; доступность
    конкретной записи зависит от самого провайдера и его прав доступа.
- **Граница безопасности**: HTTP API не принимает «любой сайт», не ходит к
  localhost/IP и не использует shared provider cookies в обычной конфигурации.

---

## 🏗 Архитектура проекта

```text
mixora/
├── compose.yaml              # API, PostgreSQL/pgvector, Redis, Mailpit, Gorse и Ollama
├── .env / .env.example       # Локальная конфигурация / безопасный шаблон
├── infra/gorse/config.toml   # Конфигурация локального recommender-а
└── beckend/
    ├── cmd/
    │   ├── server/main.go       # Mixora app-layer + музыкальный REST API
    │   └── cli/main.go          # CLI музыкального движка
    ├── internal/
    │   ├── api/                 # Существующие SoundCloud / Spotify / yt-dlp routes
    │   ├── httpapi/             # App-layer routes и middleware Mixora
    │   ├── auth/, database/     # Учётные записи, сессии, PostgreSQL
    │   ├── library/, events/    # Snapshot-библиотека, current preferences,
    │   │                        # события и durable projection/outbox
    │   ├── mail/                # SMTP и надёжный почтовый outbox
    │   ├── embedding/           # Ollama-клиент, worker и content-based рекомендации
    │   └── playback/, recommendation/, gorse/
    ├── pkg/
    │   ├── soundcloud/          # Go SDK для SoundCloud v2
    │   ├── spotify/             # Go SDK для Spotify & Librespot Connect
    │   └── ytdlp/               # Go SDK-обёртка над yt-dlp
    ├── migrations/              # Встроенные SQL-миграции, включая pgvector/HNSW
    ├── examples/                # Примеры SDK
    ├── Dockerfile
    └── go.mod
```

---

## 🚀 Быстрый старт

### 1. Запуск локального стека

Основной сценарий запуска идёт из корня репозитория, где находится `compose.yaml`:

```bash
cd /path/to/mixora
docker compose up --build -d
docker compose ps

curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/ready
```

Compose собирает `beckend/Dockerfile` и запускает API вместе с PostgreSQL, Redis и Mailpit. При старте API ждёт healthy-сервисы, применяет встроенные PostgreSQL-миграции и запускает почтовый worker. API доступен на `http://127.0.0.1:8080`, а письма в development видны в Mailpit на `http://127.0.0.1:8025`.

Для запуска локального ML-рекомендателя и API одной командой:

```bash
make recommendations
docker compose --profile recommendations ps
curl http://127.0.0.1:8088/api/health/ready
```

Команда включает зафиксированный `gorse-in-one 0.5.11`. Реальный локальный
`.env` уже содержит Docker URL и согласованные ключи; файл исключён из Git.
Без профиля API не ломается и автоматически использует `rules-v0`.

Для полного гибридного контура с локальными нейронными эмбеддингами:

```bash
make embeddings
docker compose --profile recommendations --profile embeddings ps

curl http://127.0.0.1:8080/ready
curl http://127.0.0.1:11434/api/tags
docker compose --profile embeddings exec ollama ollama list
```

`make embeddings` включает оба профиля — `recommendations` и `embeddings` —
и запускает PostgreSQL/pgvector, Redis, Mailpit, Gorse, Ollama и API. Одноразовый
сервис `ollama-pull` загружает зафиксированную модель
`embeddinggemma:300m-qat-q4_0`; API стартует только после успешного завершения
этой загрузки. Модель сохраняется в Docker volume `mixora_ollama`, поэтому при
следующем запуске повторно скачивать её не нужно. Ollama доступен только на
loopback-адресе `127.0.0.1:11434`.

`make recommendations` намеренно запускает Gorse без Ollama. Обычный
`docker compose up --build -d` не включает профильные сервисы и оставляет
`MIXORA_GORSE_URL`/`MIXORA_EMBEDDINGS_URL` пустыми, если они не заданы явно.

### Конфигурация локальных рекомендаций

| Переменная | Значение по умолчанию | Назначение и ограничения |
|---|---|---|
| `MIXORA_GORSE_URL` | пусто | Включает collaborative-рекомендации. `make recommendations` и `make embeddings` передают `http://gorse:8088`. |
| `MIXORA_GORSE_API_KEY` | пусто в Go-конфигурации | Ключ API Gorse; Compose передаёт значение `GORSE_API_KEY`. |
| `MIXORA_GORSE_TIMEOUT` | `3s` | Таймаут запросов к Gorse, допускается значение больше нуля и не более `30s`. |
| `MIXORA_EMBEDDINGS_URL` | пусто | Включает content-based слой. В Docker используется `http://ollama:11434`; для Go-процесса на хосте — `http://127.0.0.1:11434`. |
| `MIXORA_EMBEDDINGS_MODEL` | `embeddinggemma:300m-qat-q4_0` | Имя модели, которое передаётся в Ollama `/api/embed`. |
| `MIXORA_EMBEDDINGS_VERSION` | `embeddinggemma-q4-768-doc-v1` | Версия формата эмбеддинга в БД и в `model_version` ответа Wave. Смена версии ставит каталог на переиндексацию. |
| `MIXORA_EMBEDDINGS_DIMENSIONS` | `768` | Сейчас поддерживается строго `768`; другое значение останавливает запуск с ошибкой конфигурации. |
| `MIXORA_EMBEDDINGS_TIMEOUT` | `2m` | HTTP-таймаут Ollama, должен быть больше нуля и не более `10m`; embedding поискового запроса дополнительно ограничен `4s`. |
| `MIXORA_EMBEDDINGS_BATCH_SIZE` | `16` | Число треков в одном запросе worker-а, допустимый диапазон `1..128`. |
| `MIXORA_YTDLP_TIMEOUT` | `30s` | Deadline одной внешней yt-dlp операции; допустимый диапазон `1s..2m`. |
| `MIXORA_YTDLP_MAX_CONCURRENT` | `2` | Лимит параллельных yt-dlp subprocess в API, допустимый диапазон `1..8`. |
| `MIXORA_ENV` | `development` | Среда процесса. Непустой `MIXORA_YTDLP_COOKIES_FILE` разрешён конфигурацией только при точном значении `development`. |
| `MIXORA_YTDLP_COOKIES_FILE` | пусто | Путь к cookies-файлу только для изолированного local development; сам сервер не может доказать, что runtime single-user. В production его не монтируют. |
| `MIXORA_ALLOW_SHARED_YTDLP_COOKIES` | `false` | Второй обязательный явный opt-in для cookies-файла. Вместе с `MIXORA_ENV=development` разрешает запуск, но не делает shared cookies безопасными для нескольких пользователей. |
| `MIXORA_EMBEDDINGS_REQUIRED` | `false` | Только Compose-флаг зависимости API от `ollama-pull`; `make embeddings` временно устанавливает `true`. |
| `OLLAMA_IMAGE` | `ollama/ollama:0.34.1` | Версия Docker image Ollama. |
| `OLLAMA_PORT` | `11434` | Локальный loopback-порт Ollama. |

Локальные значения находятся в корневом `.env`; шаблон и полный перечень
переменных — в `.env.example`. В production нужно заменить все пароли и ключи,
включить secure-cookie за HTTPS и не публиковать порты PostgreSQL, Redis, Gorse
или Ollama во внешнюю сеть.

Для базового запуска **не нужны** SoundCloud или Spotify Client ID. SoundCloud автоматически получает текущий `client_id`, а Spotify умеет работать в Zero-Config режиме. `SOUNDCLOUD_CLIENT_ID`, `SOUNDCLOUD_AUTH_TOKEN`, `SPOTIFY_CLIENT_ID` и `SPOTIFY_CLIENT_SECRET` — только опциональные переопределения для прямого запуска Go-процесса; Compose-сервису их нужно явно передать в `environment`, если переопределение всё-таки нужно.

External yt-dlp и Spotify Connect status/info/control routes требуют Mixora
cookie-сессию.
Базовый `compose.yaml` намеренно не монтирует cookies-файл. Нельзя добавлять
один авторизованный cookies-файл в multi-user API: иначе один пользователь
сможет получить медиа через сессию другого.

Полезные команды:

```bash
docker compose logs -f api
docker compose down
```

Для запуска Go-процесса без Docker нужны доступный PostgreSQL и обязательная `MIXORA_DATABASE_URL`; для отправки писем также нужен SMTP. Остальные app-layer переменные перечислены в корневом `.env.example`.

### 2. Эндпоинты REST API

#### App-layer Mixora

| Метод | Эндпоинт | Описание |
|---|---|---|
| `GET` | `/health` | Liveness API Mixora |
| `GET` | `/ready` | Readiness API и доступность PostgreSQL |
| `POST` | `/api/v1/auth/{register\|login\|logout}` | Регистрация и cookie-сессии |
| `GET` | `/api/v1/auth/session` | Текущая сессия |
| `GET/POST` | `/api/v1/auth/verify-email` | Подтверждение email |
| `POST` | `/api/v1/auth/password/{request\|reset}` | Сброс пароля через mail outbox |
| `GET/PUT` | `/api/v1/library` | Серверная библиотека пользователя |
| `GET/PUT` | `/api/v1/me/track-preferences` | Текущее `liked` / `disliked` / `neutral` состояние трека |
| `GET` | `/api/v1/history` | Нормализованная история прослушивания аккаунта |
| `PUT/DELETE` | `/api/v1/me/history` | Идемпотентная запись или очистка истории аккаунта |
| `GET` | `/api/v1/me/playlists` | Собственные аккаунтные плейлисты с порядком треков |
| `PUT/DELETE` | `/api/v1/me/playlists/{playlistID}` | Полная идемпотентная запись или удаление аккаунтного плейлиста |
| `POST` | `/api/v1/events` | События прослушивания |
| `POST` | `/api/v1/wave` | Подбор треков «Моей волны» |
| `POST` | `/api/v1/wave/{sessionId}/feedback` | Быстрый feedback текущей волны |
| `GET` | `/api/v1/playback/ws` | WebSocket-синхронизация плеера |

### Как работает персонализация

1. Готовый music engine остаётся единственным источником музыки, текстов и
   playable-метаданных.
2. Проверенные результаты поиска музыкального движка попадают в
   `track_catalog`. Для модели формируется provider-neutral документ из
   названия, артиста, альбома, жанра и тегов; stream URL, токены и внутренние
   данные провайдера в Ollama не отправляются.
3. Embedding-worker раз в 15 секунд атомарно забирает до
   `MIXORA_EMBEDDINGS_BATCH_SIZE` изменившихся записей, вызывает Ollama
   `/api/embed` и сохраняет 768-мерные векторы в PostgreSQL `pgvector`.
   Косинусный поиск использует HNSW-индекс, а SHA-256 содержимого не даёт
   пересчитывать неизменившиеся метаданные.
4. Плеер, поиск и Wave отправляют `play`, `listen_30s`, `complete`, `skip`,
   `repeat`, значимый `seek` и добавление в плейлист с идемпотентным ключом.
   Wave feedback остаётся защищённым append-only событием в пределах своей
   выдачи.
5. Явные like, dislike и «Вернуть» из библиотеки идут через
   `PUT /api/v1/me/track-preferences`. Одна транзакция сохраняет текущее состояние,
   idempotency receipt и coalesced durable outbox. Повтор того же ключа
   возвращает прежний результат; тот же ключ с другим запросом даёт `409`.
   `neutral` — явное снятие ранее выбранного состояния, а не отсутствие записи.
6. PostgreSQL хранит исходные события. Фоновый worker повторяемо пересчитывает
   агрегаты и передаёт в локальный Gorse через `PUT /api/feedback` только
   `source:id`, уже подтверждённые в `track_catalog`; item сначала создаётся из
   server-side metadata. Неизвестное событие остаётся в журнале продукта, но не
   может создать Gorse item; недоступность Gorse или catalog оставляет batch
   retryable.
7. Для `liked`/`disliked` отдельный outbox worker перед публикацией повторно
   подтверждает `source:id` в `track_catalog` и отправляет в Gorse только
   server-side metadata snapshot: снимает противоположный feedback и ставит
   актуальный. Для `neutral` он удаляет оба сигнала без upsert item. Pending
   версии coalesced, поэтому после недоступности Gorse доставляется последний
   выбор, а не устаревшая последовательность кликов. Неизвестный legacy/client
   positive snapshot сохраняется как локальный desired state, но намеренно не
   создаёт item в Gorse. Outbox ставит durable catalog-unverified marker и
   автоматически переотправляет актуальное состояние после первого
   подтверждённого наблюдения этого `source:id`; временная ошибка каталога
   остаётся retryable.
8. Content-based слой строит профиль вкуса по положительным событиям за
   последние 180 дней, ищет ближайшие треки по среднему вектору и отдельно
   векторизует текущий запрос Wave. Текущее состояние имеет приоритет над старыми
   одноимёнными like/dislike-событиями. Два списка объединяются reciprocal-rank
   fusion с весами `0.65` для вкуса и `0.35` для запроса.
9. Gorse возвращает только provider-neutral ключи вида `source:id`.
   Collaborative- и content-списки смешиваются в пропорции 2:1. К ним всегда
   добавляется до 100 недавних nonblocked snapshots `track_catalog` (SoundCloud,
   Spotify preview, YouTube, Bandcamp, VK), затем сервер применяет
   explicit/language/dislike-фильтры и ограничение повторов артиста. Если
   персональная page полностью одного source, до 10% позиций получает
   детерминированная source-diversity квота из уже отфильтрованного catalog.
10. В `model_version` ответа `/api/v1/wave` перечислены реально использованные
   слои, например `gorse-v1+embeddinggemma-q4-768-doc-v1+rules-v0`.

### Текущие предпочтения трека

`GET /api/v1/me/track-preferences` возвращает все текущие состояния, включая
`neutral`. `PUT` требует cookie-сессию и тело следующей формы:

```json
{
  "idempotency_key": "uuid-or-other-unique-key",
  "preference": "liked",
  "track": {
    "source": "soundcloud",
    "id": "12345",
    "title": "Название",
    "artist": "Исполнитель"
  }
}
```

Дополнительные отображаемые поля snapshot (`artwork`, `duration`, `artistId`,
`explicit`, `access`, `permalink`) допускаются, но URL аудиопотока и provider
credentials не сохраняются и не передаются в Gorse. Нормализация SoundCloud
URN/path-like ID происходит в music API-adapter клиента, на границе API и в
миграциях. Переходный `/api/v1/library`
больше не является источником истины для `likes`, `dislikes`, истории и
собственных плейлистов.

### История прослушивания

`GET /api/v1/history?limit=1..100` возвращает `{ "generation": N,
"history": [...] }` с агрегированными уникальными треками по убыванию
последнего прослушивания. `PUT /api/v1/me/history` принимает компактный
provider-neutral snapshot, idempotency key и generation этого GET:

```json
{
  "idempotency_key": "uuid-or-other-unique-key",
  "generation": 0,
  "occurred_at": "2026-10-03T12:00:00Z",
  "track": {
    "source": "soundcloud",
    "id": "12345",
    "title": "Название",
    "artist": "Исполнитель"
  }
}
```

Повтор того же ключа возвращает прежний результат; другой запрос с тем же
ключом получает `409`. `DELETE /api/v1/me/history` идемпотентно очищает
историю и её replay receipts, повышает generation и отвечает, например,
`{ "generation": 1 }`. Generation — durable clear fence: listen со старым
значением получает `409 history_generation_conflict`, даже если он дошёл до
сервера после DELETE. Запись, чтение и очистка также сериализованы per-user
lock, чтобы GET видел согласованный snapshot.

### Плейлисты аккаунта

`GET /api/v1/me/playlists` возвращает только Mixora-плейлисты пользователя.
`PUT /api/v1/me/playlists/{playlistID}` заменяет полный desired state одного
плейлиста: имя, описание, флаги, упорядоченный набор уникальных compact track
snapshots, `idempotency_key` и обязательный `expected_revision`. Создание
использует revision `0`; обновление и DELETE должны передать точную текущую
revision, иначе возвращается `409 playlist_revision_conflict`. Сервер
атомарно ограничивает аккаунт 50 собственными плейлистами. Поле `legacy_id`
разрешено только при первом browser-legacy backfill, чтобы разные устройства
сопоставили один старый плейлист, и не является обычным редактируемым полем.
Маршруты являются отдельными от существующего music engine и не меняют его
поиск, метаданные или воспроизведение.

### Безопасный fallback и проверки

- Пустой `MIXORA_EMBEDDINGS_URL` полностью отключает neural content layer;
  Wave продолжает работать через Gorse и/или `rules-v0`.
- Если Ollama временно недоступен уже после запуска, ошибки worker-а
  записываются в каталог, claim снимается, а повтор выполняется с
  экспоненциальной задержкой до 5 минут. Ошибка content-рекомендации не
  блокирует Gorse и правила.
- Если Gorse недоступен, исходные события остаются в PostgreSQL и будут
  повторно спроецированы. Wave использует content-based слой, если он доступен,
  и всегда сохраняет rules-ранжирование как последний слой.
- Если Gorse недоступен при изменении like/dislike, текущее состояние уже сохранено
  локально в PostgreSQL. Durable outbox повторит публикацию с backoff; Wave
  читает серверное состояние и без ответа Gorse.
- Ollama-клиент отклоняет ответ с неверным числом векторов, размерностью не
  `768`, `NaN`/`Inf` или HTTP-ошибкой; размер JSON-ответа ограничен 32 MiB.
- Несколько embedding-worker-ов не берут одну запись одновременно благодаря
  `FOR UPDATE SKIP LOCKED` и пятиминутному claim. Gorse projection защищён
  PostgreSQL advisory lock и использует идемпотентные `PUT`-агрегаты.

Базовые проверки после запуска полного контура:

```bash
curl --fail http://127.0.0.1:8080/ready
curl --fail http://127.0.0.1:8088/api/health/ready
curl --fail http://127.0.0.1:11434/api/tags

docker compose exec postgres sh -lc \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
  "SELECT model_version, dimensions, count(*) FROM track_embeddings GROUP BY 1,2;"'
```

В логах API включённые слои отмечаются строками `Gorse recommendations enabled`
и `Local embeddings enabled`. При отключении соответствующего URL сервер явно
сообщает о fallback.

#### 🟠 SoundCloud
| Метод | Эндпоинт | Описание |
|---|---|---|
| `GET` | `/api/v1/resolve?url=<sc_url>` | Резолвит любую ссылку SoundCloud |
| `GET` | `/api/v1/tracks/{id}` | Метаданные трека |
| `GET` | `/api/v1/tracks/{id}/stream` | Прямая ссылка на воспроизведение (MP3/HLS) |
| `GET` | `/api/v1/tracks/{id}/lyrics` | Текст песни (LRC караоке с таймкодами) |
| `GET` | `/api/v1/tracks/{id}/waveform` | Данные звуковой волны для визуализатора |
| `GET` | `/api/v1/charts/trending?genre=...` | Тренды и чарты |
| `GET` | `/api/v1/search?q=<query>` | Поиск треков |

#### 🟢 Spotify & Librespot Connect
| Метод | Эндпоинт | Описание |
|---|---|---|
| `GET` | `/api/v1/spotify/resolve?url=<spotify_url>` | Резолвит любую ссылку `open.spotify.com` |
| `GET` | `/api/v1/spotify/tracks/{id}` | Метаданные трека Spotify |
| `GET` | `/api/v1/spotify/tracks/{id}/stream` | MP3 превью-стрим + статус Connect спикера |
| `GET` | `/api/v1/spotify/tracks/{id}/lyrics` | Синхронизированный караоке-текст (LRC) |
| `GET` | `/api/v1/spotify/albums/{id}` | Метаданные альбома и треклист |
| `GET` | `/api/v1/spotify/artists/{id}` | Профиль артиста и топ-треки |
| `GET` | `/api/v1/spotify/search?q=<query>` | Поиск по каталогу Spotify |
| `GET` | `/api/v1/spotify/connect/status` | 🔒 Текущий статус воспроизведения Spotify Connect |
| `GET` | `/api/v1/spotify/connect/info` | 🔒 Диагностика локального Connect daemon и binary |
| `POST` | `/api/v1/spotify/connect/player/{play\|resume\|pause\|play-pause\|next\|prev\|volume\|seek\|load}` | 🔒 Управление плеером Connect |

#### 🔴 Universal Extractor (YouTube, VK, Bandcamp via yt-dlp)
| Метод | Эндпоинт | Описание |
|---|---|---|
| `GET` | `/api/v1/extract?url=<url>` | 🔒 Разрешённый HTTPS URL YouTube/YouTube Music, Bandcamp `/track/…`, VK/VK Video media; возвращает metadata и краткоживущую media-ссылку |
| `GET` | `/api/v1/youtube/search?q=<query>&limit=5` | 🔒 Поиск треков на YouTube / YouTube Music |
| `GET` | `/api/v1/youtube/stream?url=<yt_url>` или `?id=<video_id>` | 🔒 Извлечение краткоживущего аудио-стрима YouTube |
| `GET` | `/api/v1/bandcamp/resolve?url=<bc_url>` | 🔒 Резолв одного разрешённого Bandcamp-трека |
| `GET` | `/api/v1/vk/resolve?url=<vk_url>` | 🔒 Резолв публичной разрешённой media-страницы VK/VK Video |

Ошибки extractor-а намеренно не возвращают stderr `yt-dlp`: ошибка внешнего
источника — `502`, недоступный extractor — `503`, deadline — `504`. Клиент
повторяет воспроизведение по сохранённому permalink и получает новую временную
media-ссылку, а не хранит или переиспользует старую CDN URL.

---

### 3. Использование CLI

Команды ниже выполняются из каталога `beckend`:

```bash
# 1. Экстрактор поддерживаемых ссылок (YouTube, VK, Bandcamp)
go run ./cmd/cli extract "https://disasterpeace.bandcamp.com/track/compass"
go run ./cmd/cli extract "https://www.youtube.com/watch?v=UDVtMYqUAyw"

# 2. YouTube команды:
go run ./cmd/cli yt search "Hans Zimmer Interstellar"
go run ./cmd/cli yt stream "UDVtMYqUAyw"

# 3. Bandcamp:
go run ./cmd/cli bc "https://disasterpeace.bandcamp.com/track/compass"

# 4. Spotify команды:
go run ./cmd/cli spotify track 6rqhFgbbKwnb9MLmUQDhG6
go run ./cmd/cli spotify lyrics 4u7EnebtmKWzUH433cf5Qv
go run ./cmd/cli spotify album 4m2880jivSbbyEGAKfITCa
go run ./cmd/cli spotify connect status
go run ./cmd/cli spotify connect play spotify:track:6rqhFgbbKwnb9MLmUQDhG6

# 5. SoundCloud команды:
go run ./cmd/cli search "Daft Punk"
go run ./cmd/cli stream "https://soundcloud.com/user/track-name"
go run ./cmd/cli lyrics "https://soundcloud.com/user/track-name"
```

---

## 🎧 Требования к окружению

* **Docker с Compose plugin** — для рекомендуемого запуска всего app-layer стека.
* **Go 1.27.1+** — для локальной сборки сервера, CLI и SDK.
* **PostgreSQL** — обязателен для `cmd/server`, но уже включён в корневой Compose-стек.
* **yt-dlp** *(только для локального запуска Go/CLI)*: `brew install yt-dlp` (macOS) или `sudo apt install yt-dlp` (Linux).
* **ffmpeg** *(опционально для медиа-обработки)*: `brew install ffmpeg` (macOS) или `sudo apt install ffmpeg` (Linux).
* **go-librespot** *(опционально для Spotify Connect)*: `brew install go-librespot`.

`yt-dlp` входит в API Docker image. `go-librespot` намеренно не входит: Spotify
Connect остаётся дополнительной локальной интеграцией и требует отдельной
настройки демона.

---

## 💻 Использование Go SDK

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

func main() {
	ctx := context.Background()
	yt := ytdlp.New()

	// Извлечение прямой ссылки на аудиопоток
	item, err := yt.Extract(ctx, "https://disasterpeace.bandcamp.com/track/compass")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Трек: %s — %s\n", item.Artist, item.Title)
	fmt.Printf("Прямой аудио-поток (%s @ %.0f kbps):\n%s\n", item.AudioFormat, item.Bitrate, item.AudioURL)
}
```

---

## 🧪 Тестирование

```bash
go test -v ./...
```
