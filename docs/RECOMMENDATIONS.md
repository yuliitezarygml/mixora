# Рекомендации Mixora

Этот документ описывает фактически реализованный контур «Моей волны» и
следующий этап локальных embeddings. Музыкальный движок остаётся источником
поиска, треков, текстов и аудио; рекомендательная система работает только с
непрозрачным `track_ref` и metadata snapshot.

## Рабочий поток V1

```text
Search / Player / Library
          |
          | search, impression, play, listen_30s, complete,
          | repeat, skip, like, dislike, add_to_playlist
          v
POST /api/v1/events  +  POST /api/v1/wave/{session_id}/feedback
          |
          v
PostgreSQL listening_events (источник истины, idempotency key)
          |
          | durable projection с retry
          v
Gorse: users + items + агрегированный implicit feedback
          |
          | ordered track_ref candidates
          v
Mixora ranker: hydrate из track_catalog -> filters -> artist diversity
          |
          v
POST /api/v1/wave -> tracks + session_id + model_version
          |
          v
recommendation_impressions для последующей оценки качества
```

Gorse не получает URL аудиопотока, cookie, email или пароль. Его user id —
внутренний UUID, item id — устойчивый `track_ref`. Полный объект трека хранится
в `track_catalog`, чтобы app-layer мог восстановить результат, не разбирая
внутренние идентификаторы музыкального движка.

### Надёжность

- PostgreSQL, а не Gorse, является журналом событий.
- Клиентская offline-очередь повторяет неподтверждённые события.
- У каждого события есть idempotency key, поэтому повторная отправка безопасна.
- Проектор передаёт полный агрегат `user + item + event_type` через идемпотентный
  `PUT`, а затем отмечает событие обработанным.
- Если Gorse выключен, Wave возвращает `rules-v0`; воспроизведение не зависит от
  рекомендателя.
- Каждая выдача получает UUID `session_id`, а показанные позиции сохраняются
  отдельно от feedback.

### Сигналы V1

| Событие | Роль |
|---|---|
| `like`, `add_to_playlist` | сильный положительный сигнал |
| `complete`, `repeat` | положительный сигнал удержания |
| `listen_30s`, `play` | слабый положительный/read сигнал |
| `impression` | факт показа, нужен для denominator метрик |
| `skip` | мягкий отрицательный сигнал |
| `dislike` | явный отрицательный сигнал и жёсткий фильтр клиента |
| `search` | сохраняет запрос; impressions результатов связывают запрос с items |

Вес задаётся в одном месте — `internal/gorse/projector.go`. Менять его следует
только вместе с offline-оценкой, а не под отдельного пользователя.

## Локальный запуск

```bash
make recommendations
docker compose --profile recommendations ps
curl http://127.0.0.1:8088/api/health/ready
```

Корневой `.env` является активной локальной конфигурацией и автоматически
читается Compose. Он исключён из Git. `.env.example` содержит тот же набор
переменных, но не заменяет реальный `.env`.

## V2: content embeddings

Следующий слой нужен для cold start и похожести треков, когда collaborative
истории ещё мало. Выбран локальный `EmbeddingGemma` через Ollama:

- модель небольшая и помещается на рабочем Apple M2/24 GB;
- поддерживает более 100 языков, поэтому подходит для русских и иностранных
  названий, артистов, альбомов, жанров и тегов;
- вектор сохраняется в уже включённый PostgreSQL/pgvector;
- генерация embedding идёт в фоне и не блокирует старт воспроизведения.

Текст для embedding строится только из metadata snapshot:

```text
title: ...
artist: ...
album: ...
genre: ...
tags: ...
```

Пользовательский taste-vector вычисляется из взвешенного среднего embeddings
прослушанных/понравившихся треков с вычитанием dislikes. Финальный score:

```text
0.55 collaborative + 0.25 content + 0.10 context + 0.10 exploration
```

Коэффициенты являются стартовой конфигурацией, а не обещанием качества. Перед
включением V2 нужны таблица версий embeddings, background worker, ограничение
batch/timeout, fallback при недоступном Ollama и offline-метрики. Аудио-модель
вроде CLAP добавляется только после text embeddings и проверки стоимости.

## Метрики перед production

- completion rate и early-skip rate;
- likes на 100 impressions;
- доля новых артистов и diversity;
- недоступные треки;
- доля ответов `rules-v0`;
- Wave latency p50/p95;
- projection backlog и возраст самого старого события.

Персональные данные не отправляются во внешнюю модель. Для удаления аккаунта
должны каскадно удаляться события и библиотека, а user в Gorse — удаляться
отдельной фоновой задачей.

## Первичные источники

- Gorse Docker: <https://gorse.io/docs/deploy/docker>
- Gorse REST API: <https://gorse.io/docs/api/restful-api>
- Gorse collaborative filtering: <https://gorse.io/docs/concepts/recommenders/collaborative>
- Ollama EmbeddingGemma: <https://ollama.com/library/embeddinggemma:latest>
- Google EmbeddingGemma: <https://ai.google.dev/gemma/docs/embeddinggemma>
