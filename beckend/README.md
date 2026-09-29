# Universal Music Backend & Go SDK (SoundCloud + Spotify Librespot + YouTube / Bandcamp / VK)

Полноценная модульная Go-библиотека, автономный REST API бэкенд и CLI-интерфейс, объединяющий три мощнейших аудио-движка:
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
soundcloud-go/
├── cmd/
│   ├── server/
│   │   └── main.go          # Автономный REST API бэкенд (SoundCloud + Spotify + yt-dlp)
│   └── cli/
│       └── main.go          # Единая CLI утилита со всеми командами
├── internal/
│   └── api/                 # Реализация REST API сервиса
│       ├── handlers.go      # Общие и SoundCloud обработчики
│       ├── spotify_handlers.go # Spotify & Spotify Connect обработчики
│       ├── ytdlp_handlers.go   # Universal Extractor (YouTube, VK, Bandcamp) обработчики
│       ├── router.go        # Маршрутизация и middleware (CORS, Logging)
│       └── response.go      # Стандартизация ответов и ошибок
├── pkg/
│   ├── soundcloud/          # Go SDK для SoundCloud v2
│   │   ├── client.go        # Инициализация и Functional Options
│   │   ├── tracks.go        # Метаданные, waveform, stream
│   │   ├── lyrics.go        # Синхронизированные тексты песен
│   │   └── ...
│   ├── spotify/             # Go SDK для Spotify & Librespot Connect
│   │   ├── client.go        # Инициализация клиента Spotify
│   │   ├── tracks.go        # Метаданные треков, альбомов, артистов
│   │   ├── connect.go       # HTTP клиент к Spotify Connect демону (go-librespot)
│   │   ├── supervisor.go    # Менеджер процесса демона Connect
│   │   └── ...
│   └── ytdlp/               # Go SDK для Universal Extractor (yt-dlp)
│       ├── client.go        # Обнаружение и запуск бинарника yt-dlp
│       ├── extract.go       # Извлечение метаданных и прямых audio CDN URL
│       ├── search.go        # Поиск по YouTube Music / YouTube
│       └── models/          # Доменные структуры
├── examples/
│   ├── basic/               # Пример SoundCloud Go SDK
│   ├── spotify/             # Пример Spotify & Connect Go SDK
│   └── ytdlp/               # Пример Universal Extractor (Bandcamp + YouTube) Go SDK
├── go.mod
└── README.md
```

---

## 🚀 Быстрый старт

### 1. Запуск REST API сервера

```bash
go run ./cmd/server -port=8080
```

Сервер автоматически определит `yt-dlp`, извлечет `client_id` для SoundCloud и подключит Spotify Connect.

#### Эндпоинты REST API:

##### Общие:
| Метод | Эндпоинт | Описание |
|---|---|---|
| `GET` | `/health` | Статус готовности всех трех движков (`soundcloud`, `spotify`, `ytdlp`) |

##### 🟠 SoundCloud:
| Метод | Эндпоинт | Описание |
|---|---|---|
| `GET` | `/api/v1/resolve?url=<sc_url>` | Резолвит любую ссылку SoundCloud |
| `GET` | `/api/v1/tracks/{id}` | Метаданные трека |
| `GET` | `/api/v1/tracks/{id}/stream` | Прямая ссылка на воспроизведение (MP3/HLS) |
| `GET` | `/api/v1/tracks/{id}/lyrics` | Текст песни (LRC караоке с таймкодами) |
| `GET` | `/api/v1/tracks/{id}/waveform` | Данные звуковой волны для визуализатора |
| `GET` | `/api/v1/charts/trending?genre=...` | Тренды и чарты |
| `GET` | `/api/v1/search?q=<query>` | Поиск треков |

##### 🟢 Spotify & Librespot Connect:
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

##### 🔴 Universal Extractor (YouTube, VK, Bandcamp via yt-dlp):
| Метод | Эндпоинт | Описание |
|---|---|---|
| `GET` | `/api/v1/extract?url=<url>` | **Универсальный экстрактор**: отдает название, автора, обложку и прямой CDN Audio URL для любого сайта |
| `GET` | `/api/v1/youtube/search?q=<query>&limit=5` | Поиск треков на YouTube / YouTube Music |
| `GET` | `/api/v1/youtube/stream?url=<yt_url_or_id>` | Извлечение прямого аудио-стрима YouTube (Opus 160k / AAC 128k) |
| `GET` | `/api/v1/bandcamp/resolve?url=<bc_url>` | Резолв трека или альбома Bandcamp с прямыми MP3-потоками |
| `GET` | `/api/v1/vk/resolve?url=<vk_url>` | Резолв аудиозаписей ВКонтакте (VK) |

---

### 2. Использование CLI

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

* **Go 1.25+**
* **yt-dlp**: `brew install yt-dlp` (macOS) или `sudo apt install yt-dlp` (Linux)
* **ffmpeg**: `brew install ffmpeg` (macOS) или `sudo apt install ffmpeg` (Linux)
* **go-librespot** *(опционально для Spotify Connect)*: `brew install go-librespot`

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
