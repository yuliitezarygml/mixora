# Рекомендации Mixora

Этот документ описывает фактически работающий контур «Моей волны». Готовый
music engine остаётся источником поиска, треков, текстов и аудио. Mixora хранит
каноническую ссылку на трек и небольшой metadata snapshot, необходимый для
рекомендаций.

## Рабочий поток

```text
Search / Player / Wave
          |
          | impression, play, listen_30s, complete, repeat,
          | skip, seek, add_to_playlist и Wave feedback
          v
PostgreSQL listening_events (append-only journal, idempotency key)
          |
          +--> durable projection ------------------------> Gorse
          |
          +--> server-side taste -------------------------> pgvector

Like / dislike / «Вернуть»
          |
          v
GET/PUT /api/v1/me/track-preferences
          |
          v
user_track_preferences + idempotency receipt + durable outbox
          |                                      |
          |                                      +----------> Gorse: текущее состояние
          +--> Wave filters and server-side taste

Wave context --> EmbeddingGemma query --> pgvector cold-start candidates
          |
          v
content taste/query RRF --> Gorse/content blend 2:1 --> rules/filters
          |
          v
POST /api/v1/wave --> tracks + session_id + model_version
          |
          v
recommendation_impressions + impression events
```

Gorse и Ollama не получают cookie, email, пароль или URL аудиопотока. User ID
в Gorse — внутренний UUID, item ID — каноническая пара `source:id`. Полный
metadata snapshot хранится в `track_catalog`; legacy SoundCloud URN/path-like
ID нормализуются на границах клиента, API и миграции данных.

## Collaborative-контур

- PostgreSQL является долговечным журналом событий; Gorse — перестраиваемым
  индексом рекомендаций.
- У каждого события обязателен `idempotency_key`, поэтому повторная доставка из
  offline-очереди не увеличивает сигнал дважды.
- Для событий внутри Wave дополнительно хранится receipt на
  `user + session + track + type`: новый случайный key не позволит повторно
  усилить один и тот же сигнал в одной выдаче.
- Проектор пересчитывает полный агрегат `user + item + event_type`, отправляет
  его в Gorse через идемпотентный `PUT` и только потом отмечает события.
- PostgreSQL advisory lock не позволяет нескольким API-репликам перезаписать
  агрегат устаревшим значением.
- Каждая Wave получает UUID `session_id`; feedback разрешён только для трека,
  реально показанного этому пользователю в этой сессии. Хранилище показов
  обязательно; если оно не сохранилось транзакционно, Wave отвечает ошибкой,
  а не выдаёт невалидную сессию.
- Показ Wave сохраняется одновременно в `recommendation_impressions` и как
  `impression` в `listening_events`.

## Текущее состояние «нравится / не нравится»

Like, dislike и возврат в нейтральное состояние больше не интерпретируются как
неограниченная сумма одинаковых кликов. Для каждой пары `user + source:id`
сервер хранит одно значение `liked`, `disliked` или `neutral` в
`user_track_preferences`.

- `PUT /api/v1/me/track-preferences` принимает компактный track snapshot,
  желаемое состояние и `idempotency_key`; повтор того же запроса возвращает
  исходный результат, а повтор ключа с другим запросом даёт `409`.
- Одна транзакция обновляет текущее состояние, receipt и coalesced строку
  `user_track_preference_outbox`. Конкурентные записи для одного трека
  сериализуются, поэтому поздний выбор не может быть перезаписан устаревшим
  запросом другого устройства.
- Worker забирает outbox с lease/retry/backoff. Для `liked` он снимает
  противоположный dislike и записывает like в Gorse; для `disliked` делает
  обратное; `neutral` удаляет оба сигнала. Устаревшие pending-версии
  coalesced, поэтому наружу публикуется последнее состояние.
- Общий advisory lock разделяют event projector и worker предпочтений. Их
  публикации в Gorse выполняются последовательно; если сначала успел старый
  event-агрегат, следующая публикация текущего состояния его исправляет. Если Gorse
  выключен, запись остаётся в PostgreSQL до следующей попытки.
- Wave читает серверное состояние до ранжирования; оно имеет приоритет над
  временными seed клиента. Content taste также заменяет старые `like`/
  `dislike`-события текущим состоянием, а `neutral` снимает такой сигнал.
- Клиент сразу обновляет интерфейс, хранит отдельную offline-очередь на
  аккаунт и оставляет в ней только последний выбор для трека. После входа он
  сначала читает серверное состояние, затем накладывает ещё не доставленные
  локальные изменения.

Миграция `006_track_preferences.sql` переносит валидные likes/dislikes из
старого `user_libraries.payload`; при конфликте legacy dislike имеет приоритет.
Snapshot библиотеки остаётся переходным хранилищем других библиотечных полей,
но больше не является источником истины для likes/dislikes.

## Content embeddings V2a

Локальный embedding-контур уже включён и работает через Ollama:

- runtime model: `embeddinggemma:300m-qat-q4_0`;
- неизменяемая версия индекса: `embeddinggemma-q4-768-doc-v1`;
- размерность: 768;
- хранение: PostgreSQL/pgvector, один активный vector на канонический трек;
- nearest-neighbour lookup: HNSW cosine index.

Документ строится только из доверенного metadata snapshot:

```text
title: <title> | text: artist: <artist> | album: <album> | genre: <genre> | tags: <tags>
```

Cold-start запрос использует отдельный retrieval prompt:

```text
task: search result | query: <Wave query>
```

Фоновый worker выбирает задачи с `FOR UPDATE SKIP LOCKED`, обрабатывает их
батчами, проверяет количество, размерность и конечность значений, а при ошибке
освобождает claim с retry/backoff. Hash metadata предотвращает повторную
индексацию неизменившегося трека.

## Персонализация и fallback

Сервер строит taste-vector из `listening_events` и текущих предпочтений,
поэтому профиль работает между устройствами. Like, добавление, repeat,
complete, длительное прослушивание и play дают положительный вес; skip и
dislike не становятся положительными seed. Если для трека уже существует
текущее предпочтение, оно заменяет его старые `like`/`dislike`-события в этом
расчёте. Клиентские seed используются только для текущего ответа и не могут
изменять общий каталог.

Content-кандидаты вкуса и query объединяются weighted reciprocal-rank fusion.
Затем итоговый список смешивает два collaborative-кандидата Gorse и один
content-кандидат, после чего применяются существующие фильтры и правила
разнообразия. Это порядок слияния, а не обученный числовой score.

Каждый внешний слой допускает отказ:

- нет Ollama или pgvector-результата — используются Gorse + rules;
- нет Gorse — используются content + rules;
- оба персональных слоя недоступны — Wave остаётся на `rules-v0` и music engine;
- embedding никогда не находится в критическом пути воспроизведения.

Активный путь виден в `model_version`, например
`gorse-v1+embeddinggemma-q4-768-doc-v1+rules-v0`.

## Локальный запуск

Полный контур:

```bash
make embeddings
docker compose --profile recommendations --profile embeddings ps
curl http://127.0.0.1:8080/ready
curl http://127.0.0.1:11434/api/tags
```

Только collaborative-контур без Ollama:

```bash
make recommendations
```

Корневой `.env` является активной локальной конфигурацией и исключён из Git.
`.env.example` содержит синхронизированный безопасный шаблон, но не заменяет
реальный `.env`.

## Что ещё не реализовано или не подтверждено сквозным сценарием

- нормализованные server-side history и CRUD/ordering плейлистов;
- разрешение конфликтов нескольких устройств для history и плейлистов;
- полный сбор repeat/seek на всех путях плеера (сейчас есть repeat-one и
  осмысленные перемотки от пяти секунд);
- offline evaluation, A/B-ready assignment и продуктовые dashboards;
- управляемая exploration и объяснение причины для каждого трека;
- CLAP/audio embeddings — только после измеримого сравнения с text embeddings.

Контур предпочтений покрыт целевыми Go unit/HTTP contract tests и клиентскими
unit tests: сохранение и replay состояния, конфликт idempotency key, очередь,
retry outbox и перевод трёх состояний в операции Gorse. Полный ручной сценарий
в запущенном Docker-стеке — migration → offline click → повторный вход →
подтверждённая публикация в Gorse — остаётся отдельной проверкой перед тем, как
считать его production-ready.

Основные метрики перед production: completion rate, early-skip rate, likes на
100 impressions, diversity/novelty, недоступные треки, доля fallback-ответов,
Wave latency p50/p95 и возраст projection/embedding backlog.

## Первичные источники

- Gorse Docker: <https://gorse.io/docs/deploy/docker>
- Gorse REST API: <https://gorse.io/docs/api/restful-api>
- Gorse collaborative filtering: <https://gorse.io/docs/concepts/recommenders/collaborative>
- Ollama EmbeddingGemma tags: <https://ollama.com/library/embeddinggemma/tags>
- Google EmbeddingGemma model card: <https://ai.google.dev/gemma/docs/embeddinggemma/model_card>
