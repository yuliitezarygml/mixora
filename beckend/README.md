# Mixora API и Universal Music Engine (SoundCloud + Spotify Librespot + YouTube / Bandcamp / VK)

Готовый музыкальный движок на Go остаётся отдельным слоем SDK, REST-маршрутов и CLI. `cmd/server` теперь также поднимает app-layer Mixora: аутентификацию и сессии, библиотеку пользователя, события, рекомендации, синхронизацию плеера, PostgreSQL-миграции и почтовый outbox. Неизвестные app-layer маршруты передаются в существующий музыкальный router.

Движок объединяет три источника:
1. **SoundCloud v2** (реверс-инжиниринг с авто-скрапингом `client_id`)
2. **Spotify & Librespot** (Spotify Connect приемник, удаленное управление и Zero-Config парсинг)
3. **Universal Extractor на базе `yt-dlp`** (YouTube / YouTube Music, Bandcamp, ВКонтакте (VK) и ещё 1000+ медиа-сервисов)

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
  - Полный обход алгоритмов троттлинга `n-sig` (всегда максимальная скорость отдачи).
  - Стриминг наивысшего качества: **Opus 160 kbps** (`itag 251`) и **AAC 128 kbps** (`itag 140`).
  - Быстрый поиск треков через `ytsearch` без квот Google Cloud API.
- **Bandcamp**:
  - Прямые CDN-ссылки на стриминг MP3 128/160 kbps с `bcbits.com`.
  - Парсинг альбомов, треклистов и оригинальных обложек высокого разрешения.
- **ВКонтакте (VK)**:
  - Извлечение треков, альбомов и плейлистов сообществ/пользователей со склейкой HLS m3u8.
- **1000+ сервисов**: поддержка Mixcloud, Vimeo, TikTok и любых других источников.

---

## 🏗 Архитектура проекта

```text
mixora/
├── compose.yaml              # API, PostgreSQL, Redis и Mailpit для локального запуска
├── .env.example              # Пример app-layer конфигурации
└── beckend/
    ├── cmd/
    │   ├── server/main.go       # Mixora app-layer + музыкальный REST API
    │   └── cli/main.go          # CLI музыкального движка
    ├── internal/
    │   ├── api/                 # Существующие SoundCloud / Spotify / yt-dlp routes
    │   ├── httpapi/             # App-layer routes и middleware Mixora
    │   ├── auth/, database/     # Учётные записи, сессии, PostgreSQL
    │   ├── library/, events/    # Библиотека и события пользователя
    │   ├── mail/                # SMTP и надёжный почтовый outbox
    │   └── playback/, recommendation/
    ├── pkg/
    │   ├── soundcloud/          # Go SDK для SoundCloud v2
    │   ├── spotify/             # Go SDK для Spotify & Librespot Connect
    │   └── ytdlp/               # Go SDK-обёртка над yt-dlp
    ├── migrations/              # Встроенные SQL-миграции
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

Для базового запуска **не нужны** SoundCloud или Spotify Client ID. SoundCloud автоматически получает текущий `client_id`, а Spotify умеет работать в Zero-Config режиме. `SOUNDCLOUD_CLIENT_ID`, `SOUNDCLOUD_AUTH_TOKEN`, `SPOTIFY_CLIENT_ID` и `SPOTIFY_CLIENT_SECRET` — только опциональные переопределения для прямого запуска Go-процесса; Compose-сервису их нужно явно передать в `environment`, если переопределение всё-таки нужно.

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
| `POST` | `/api/v1/events` | События прослушивания |
| `POST` | `/api/v1/wave` | Подбор треков «Моей волны» |
| `GET` | `/api/v1/playback/ws` | WebSocket-синхронизация плеера |

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
| `GET` | `/api/v1/spotify/connect/status` | Текущий статус воспроизведения Spotify Connect |
| `POST` | `/api/v1/spotify/connect/player/{play\|pause\|next\|volume\|load}` | Управление плеером Connect |

#### 🔴 Universal Extractor (YouTube, VK, Bandcamp via yt-dlp)
| Метод | Эндпоинт | Описание |
|---|---|---|
| `GET` | `/api/v1/extract?url=<url>` | **Универсальный экстрактор**: отдает название, автора, обложку и прямой CDN Audio URL для любого сайта |
| `GET` | `/api/v1/youtube/search?q=<query>&limit=5` | Поиск треков на YouTube / YouTube Music |
| `GET` | `/api/v1/youtube/stream?url=<yt_url_or_id>` | Извлечение прямого аудио-стрима YouTube (Opus 160k / AAC 128k) |
| `GET` | `/api/v1/bandcamp/resolve?url=<bc_url>` | Резолв трека или альбома Bandcamp с прямыми MP3-потоками |
| `GET` | `/api/v1/vk/resolve?url=<vk_url>` | Резолв аудиозаписей ВКонтакте (VK) |

---

### 3. Использование CLI

Команды ниже выполняются из каталога `beckend`:

```bash
# 1. Универсальный экстрактор для любых ссылок (YouTube, VK, Bandcamp)
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
* **yt-dlp** *(опционально для Universal Extractor)*: `brew install yt-dlp` (macOS) или `sudo apt install yt-dlp` (Linux).
* **ffmpeg** *(опционально для медиа-обработки)*: `brew install ffmpeg` (macOS) или `sudo apt install ffmpeg` (Linux).
* **go-librespot** *(опционально для Spotify Connect)*: `brew install go-librespot`.

Внешние бинарники `yt-dlp` и `go-librespot` не входят в текущий Docker image; без них сервер продолжает работать, а зависящие от них функции остаются отключёнными.

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
