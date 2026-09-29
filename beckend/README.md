# Mixora backend

Go API музыкальной платформы. Папка называется `beckend` по названию в задаче.

## Реализовано

- PostgreSQL, пул соединений и транзакционные SQL-миграции при запуске.
- Регистрация, вход, выход, профиль; bcrypt и сессии в HttpOnly cookie.
- Собственный каталог: поиск по названию/исполнителю, пагинация, отметка Explicit.
- Личные плейлисты, добавление/удаление треков, проверка владельца.
- Выдача локального аудио с HTTP Range (перемотка), без выхода за каталог хранения.
- CLI импорта аудио, Docker Compose, health checks, корректное завершение сервера.
- Адаптер официального SoundCloud API: поиск публичных треков и получение сведений о потоках, авторизация приложения, шифрование и обновление токенов.

Это рабочая первая версия backend. Интерфейс, подключение личного аккаунта SoundCloud через PKCE, смешанные плейлисты с внешними треками, избранное, история прослушивания, отдельные сущности альбомов/исполнителей и остальные музыкальные сервисы пока не реализованы. Поле `album` сейчас — строка в треке. Внешний поиск отделён от собственного каталога.

## Запуск

Нужен Docker с Compose. Локальный `.env` уже подготовлен со случайным паролем базы и ключом шифрования; не публикуйте его.

```sh
cd /Users/iulian/Documents/mixora/beckend
docker compose up -d --build
curl http://localhost:8080/health/ready
```

API: `http://localhost:8080`; PostgreSQL на хосте: `127.0.0.1:5433`. Оба порта опубликованы только локально. Параметры — в `.env`. Остановка: `docker compose down` (данные останутся в томе). Не добавляйте `-v`, если нужно сохранить данные.

На новой машине скопируйте `.env.example` в `.env`, задайте случайный шестнадцатеричный `POSTGRES_PASSWORD` и тот же пароль в `DATABASE_URL`. Для ключа шифрования используйте 32 случайных байта в hex (64 символа). Не меняйте пароль в `.env` у уже инициализированного тома без смены пароля роли PostgreSQL.

Для разработки с Go 1.26+:

```sh
docker compose up -d db
# Если API из Compose уже запущен, сначала освободите его порт:
docker compose stop api
set -a
source .env
set +a
go run ./cmd/api
```

Go читает переменные окружения, а не `.env` автоматически. Compose читает `.env` сам. Origin интерфейса должен совпадать с `ALLOWED_ORIGIN`; используйте один hostname (`localhost`) для API и сайта. Для браузерных запросов требуется `credentials: "include"`.

## Импорт музыки

Сначала запустите базу. Загружайте файлы, которые вы вправе использовать. CLI проверяет расширение и размер, но не декодирует аудио и не делает перекодирование; формат должен поддерживаться будущим плеером.

```sh
set -a
source .env
set +a
go run ./cmd/import-track \
  -file /absolute/path/song.mp3 \
  -title 'Название' -artist 'Исполнитель' -album 'Альбом' -explicit
```

Ответ содержит UUID трека. Файл копируется в `storage` под случайным именем; метаданные сохраняются в PostgreSQL. Ограничение: 500 MiB; MP3, WAV, OGG, FLAC, M4A. Тесты создают временный аудиофайл; демонстрационная чужая музыка в проект не включена.

## API

Ошибки прикладных обработчиков: `{"error":"..."}`. Все тела POST, кроме выхода, — JSON с `Content-Type: application/json`; неизвестные поля отклоняются. После регистрации нужен отдельный вход.

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/health/live` | Сервер работает |
| GET | `/health/ready` | База доступна |
| POST | `/api/v1/auth/register` | `email`, `password`, `display_name` |
| POST | `/api/v1/auth/login` | `email`, `password`; устанавливает cookie |
| POST | `/api/v1/auth/logout` | Удаляет текущую сессию |
| GET | `/api/v1/me` | Профиль текущего пользователя |
| GET | `/api/v1/tracks?q=...&limit=50&offset=0` | Публичный каталог; максимум 100 |
| GET | `/api/v1/tracks/{id}` | Данные трека |
| GET / HEAD | `/api/v1/tracks/{id}/stream` | Аудио, требует входа; поддерживает Range |
| GET / POST | `/api/v1/playlists` | Список / создание (`name`) |
| GET | `/api/v1/playlists/{id}/tracks` | Состав своего плейлиста |
| POST | `/api/v1/playlists/{id}/tracks` | Добавление (`track_id`), повтор безопасен |
| DELETE | `/api/v1/playlists/{id}/tracks/{trackID}` | Удаление трека из плейлиста |
| DELETE | `/api/v1/playlists/{id}` | Удаление своего плейлиста |
| GET | `/api/v1/providers/soundcloud/tracks?q=...&limit=20&offset=0` | Поиск SoundCloud, требует входа |
| GET | `/api/v1/providers/soundcloud/tracks/{id}/streams` | Метаданные потоков SoundCloud, требует входа |
| GET | `/api/v1/providers/soundcloud/tracks/{id}/playback` | Временный URL HLS для плеера, требует входа |

Пароль: 12–72 байта. Сессия: 7 дней. Вход и регистрация ограничены суммарно 20 запросами в минуту с одного IP на экземпляр API. Поиск каталога возвращает `items`, `limit`, `offset`; списки плейлистов — `items`. Плейлисты приватные.

Пример:

```sh
curl -sS http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"listener@example.com","password":"a-long-example-password","display_name":"Listener"}'
curl -sS -c /tmp/mixora-cookies.txt http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"listener@example.com","password":"a-long-example-password"}'
curl -sS -b /tmp/mixora-cookies.txt http://localhost:8080/api/v1/me
curl -sS -b /tmp/mixora-cookies.txt http://localhost:8080/api/v1/playlists \
  -H 'Content-Type: application/json' -d '{"name":"Любимые треки"}'
```

## SoundCloud

Источники: [официальный guide](https://developers.soundcloud.com/docs/api/guide), [OpenAPI](https://github.com/soundcloud/api/blob/master/openapi/api.yaml). Проверены 27.09.2026.

В `.env` заполните `SOUNDCLOUD_CLIENT_ID`, `SOUNDCLOUD_CLIENT_SECRET` данными зарегистрированного приложения. `TOKEN_ENCRYPTION_KEY` уже сгенерирован локально. Пересоздайте API: `docker compose up -d api`. Не отправляйте секреты в чат и не кладите их в frontend.

Адаптер использует Client Credentials для публичных ресурсов, Basic authentication при получении токена и `Authorization: OAuth ...` при вызовах API. Refresh-токены зашифрованы AES-256-GCM в PostgreSQL; блокировка базы сериализует обновление между экземплярами. При потере ответа во время одноразового refresh может потребоваться ручное восстановление авторизации. Ошибка не вызывает бесконечную выдачу новых токенов.

Без ключей маршруты возвращают 503. С локальными ключами проверены авторизация, поиск, получение потоков и загрузка HLS-манифеста и фрагмента аудио с CDN SoundCloud. Контрактные тесты используют локальный HTTP-сервер. Возвращается JSON SoundCloud с исходными полями `collection`, `next_href`, `access`, данными автора и ссылками атрибуции. Следующую страницу запрашивайте через Mixora с `offset`; `next_href` относится к внешнему API.

`/streams` возвращает метаданные внешних потоков. Для плеера используйте `/playback`: backend авторизует запрос к адресу потока SoundCloud и возвращает временную подписанную ссылку CDN, `format: "hls"` и `source: "soundcloud"`. OAuth-токен остаётся на сервере. После истечения ссылки запросите новую. Для браузеров без встроенной поддержки HLS потребуется HLS-плеер на frontend; интерфейс ещё не реализован. Маршрут поддерживает HLS AAC/MP3 и возвращает 403, если такой поток отсутствует. Ограничения `playable`, `preview`, `blocked` сохраняются; источник и автор должны быть показаны в интерфейсе.

## Архитектура

```text
cmd/api                 сборка зависимостей и жизненный цикл HTTP-сервера
cmd/import-track        локальный административный импорт
internal/config         переменные окружения
internal/database       пул PostgreSQL и исполнитель миграций
internal/auth           регистрация, пароли, сессии
internal/catalog        каталог и работа с треками
internal/playlist       плейлисты с проверкой владельца в SQL
internal/soundcloud     клиент API и защищённое хранение токенов
internal/httpapi        маршруты, JSON, cookie, CORS, обработчики
migrations              версионированная SQL-схема, включённая в бинарник
storage                 аудио; содержимое исключено из Git и Docker image
```

Модульный монолит: один процесс и одна база. HTTP-слой вызывает модули предметной области; SQL находится в соответствующих модулях. Параметризованные запросы, внешние ключи и ограничения базы защищают целостность. Для токенов, cookie и аудио разные механизмы хранения и доступа.

## Проверки

```sh
go test -race ./...
go vet ./...
# Полный сценарий с настоящей базой:
set -a
source .env
set +a
TEST_DATABASE_URL="$DATABASE_URL" go test -race ./...
```

Без `TEST_DATABASE_URL` интеграционный тест пропускается. С ним создаётся отдельная временная схема, после теста удаляется только она. Проверяются миграции, регистрация, конфликт email, вход/выход, истечение сессии, защита чужих плейлистов, Range, запрет выхода за storage, CORS, JSON, ограничение попыток входа и контракты SoundCloud.

Перед публичным размещением: HTTPS и `COOKIE_SECURE=true`, точный origin, резервное копирование базы и storage; настройка доверенных прокси и общего rate limit при нескольких экземплярах. Текущий лимитер использует RemoteAddr, не доверяет X-Forwarded-For и за reverse proxy будет учитывать адрес прокси. Восстановление пароля и подтверждение email пока отсутствуют. Автоматические миграции удобны для локальной версии; для дальнейших изменений схемы добавляйте новые SQL-файлы.
