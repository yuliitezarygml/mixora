# QA плеера и коллекции Mixora — 2026-10-08

Scope первого прохода: нижний плеер и его действия. Ниже отдельный проход
коллекции и рекомендуемых плейлистов; это не аудит всего интерфейса проекта.

## Источник и результат

- Выбран второй пользовательский скриншот, не первый.
- Источник: `/var/folders/6_/w9_z4_r13rg3dtmypxxnr25m0000gn/T/TemporaryItems/NSIRD_screencaptureui_nCWKe7/Снимок экрана — 2026-10-08 в 09.19.38.png`.
- Размер исходного изображения: 2116 × 198 px. Для сравнения принят масштаб 2×
  (вывод по размерам обложки и кнопок), эквивалент ширины 1058 CSS px.
- Реализация: `http://127.0.0.1:5174/`, Codex in-app browser.
- Desktop: 1058 × 900 CSS px, DPR 1 при viewport override. Плеер 1038 × 80 px,
  обложка 64 px, play 40 px, seek 1020 px; боковые поля сохранены из оболочки.
- Mobile: 390 × 844 CSS px, DPR 1; плеер 374 × 66 px, seek 356 px,
  горизонтального overflow нет. Временный viewport override сброшен.
- Состояние сравнения: выбран реальный трек, пауза, прогресс около 23%.
  У референса Take It/Ambassador, у реализации Утро/Дайте танк (!): metadata
  и обложка намеренно берутся из сервиса, а не подменяются для скриншота.
- Снимки: `mixora-client/release/player-qa/desktop.jpg`, `player.jpg`, `mobile.jpg`.
  Это локальные QA-артефакты в ignored release, не production assets.

## Визуальное сравнение

Reference и итоговый desktop screenshot открыты и сравнены.

- Постоянная жёлтая полоса по верхней кромке и круглый thumb вместо маленькой
  центральной hover-плашки. Track 4 px, thumb 12 px; время осталось в ARIA.
- Нейтральный серо-оливковый фон, лёгкое выделение пройденной части,
  скругление и рамка сохранены. Цвет не зависит от CORS canvas обложки.
- Обложка и metadata слева, многоточие рядом с названием, ellipsis длинных строк.
- В центре dislike → shuffle → previous → play/pause → next → repeat → like.
- Справа text → queue → sound settings → volume. Использованы оригинальные
  sprite icons и шрифты локального референса, без нарисованных замен.
- Play центрирован относительно всего плеера, а не оставшегося после metadata
  пространства. Отступы адаптируются на 960/840 px; на мобильном остаются
  metadata, play и полная seek-полоса.

## Findings и исправления

- P1: прежняя seek-плашка не соответствовала референсу — исправлено.
- P2: кнопки смещались вправо в узком desktop-окне — исправлено симметричной grid.
- P2: многоточие было нерабочим, sound settings отсутствовали — подключены.
- P2: очередь не давала переставлять/удалять треки, current-row click не ставил
  паузу — исправлено и покрыто React integration-тестом.
- Открытых P0/P1/P2 в проверенном scope нет. P3: реальные metadata и боковые
  поля оболочки отличаются от вырезанного референса; это намеренно сохранено.

## Поведение и проверки

- SoundCloud «Утро»: воспроизведение подтверждено ростом позиции; pause и click
  seek до 33.8 s работают, без playback error.
- Queue открывается; Awake перемещён вниз и обратно, current трек сохранён.
- Меню текущего трека открывает реальные действия библиотеки/плейлистов.
- Настройки звука открываются; equalizer checkbox включён и возвращён обратно.
  Это проверка UI/state переключателя, не акустическая оценка фильтров.
- Browser console: нет ошибок в финальном проходе.
- 105/105 client tests; format check и SHA-256 43 reference stylesheets прошли.
- Build и macOS ARM64 packaging прошли. Существующее предупреждение о размере
  HLS chunk и отсутствие подписи/default Electron icon не являются visual gate.
- Полный desktop journey пяти музыкальных источников этим отчётом не закрыт.

final result: passed

## Коллекция и рекомендуемые плейлисты

- Desktop: существующее окно 934 × 857 CSS px; mobile breakpoint проверен
  при 390 × 844 px, временный viewport override возвращён в исходное состояние.
- Red repro: первая artist card растягивалась до 1574 px, несколько других
  превышали 200 px; intrinsic width имени увеличивал и квадратную обложку.
  После фикса все artist cards 180 px desktop / 148 px mobile. Карточки
  рекомендаций на mobile 148 px, горизонтального overflow страницы нет.
- Добавлены «Для вас», «Открытия», «Для работы», preview треков и их источников,
  play/save/refresh, состояния гостя/загрузки/ошибки/пустой выдачи.
- Browser smoke: «Самая (feat. Amigo)» играет, позиция достигла 83.1 s при
  duration 291.3 s и без player error. Snapshot «Для вас» из 13 треков появился
  в коллекции и сохранился после reload; воспроизведение оставлено на паузе.
- CSS regression, запросы/фильтрация/поиски/отмена, React account fencing,
  preview до загрузки account и feedback до начала playback покрыты тестами.
  122/122 client tests; формат, 43 original stylesheet hashes, Go tests
  recommendation/embedding/httpapi и build/macOS ARM64 packaging пройдены.
- Рабочий `.env` подключает ранее установленную EmbeddingGemma. API healthy,
  local embeddings enabled, индекс на момент проверки 236/236. В текущем
  языковом профиле ответы rule-only; UI не заявляет, что они нейросетевые,
  и сообщает о значительном пересечении focus/daily при малом каталоге.
- Proof: `mixora-client/release/collection-qa/collection-desktop.jpg`,
  `collection-mobile.jpg`, `saved-playlist.jpg`. Снимки находятся в ignored
  release, а не в исходных assets или репозитории.
- Остаток: измеренная релевантность/акустический mood не проверены; полный
  live journey всех пяти источников в packaged Electron не закрыт.

Collection functional/layout result: passed; recommendation quality evaluation: pending.

## Последний дизайн-срез коллекции — образец от 11:12

Это scoped image-to-code проход коллекции, не пиксельная копия чужой библиотеки.
Использованы существующие шрифты/токены/иконки, оригинальное изображение сердца
и локальные Wave-assets. Оригинальные 43 таблицы стилей не изменялись.

### Источник, состояние и evidence

- Актуальная visual truth: `/var/folders/6_/w9_z4_r13rg3dtmypxxnr25m0000gn/T/TemporaryItems/NSIRD_screencaptureui_H015Gb/Снимок экрана — 2026-10-08 в 11.12.14.png`.
  Более ранний скрин от 11:00 показывает проблему, не желаемый дизайн.
- Source: 2076 × 1362 px, cropped content без навигации/плеера. По размеру
  сердца и типографике принят масштаб 2×, около 1038 × 681 CSS px; это вывод,
  исходный viewport не приложен.
- Browser: 1260 × 900 CSS px, DPR 1, screenshot 1260 × 900 px; content около
  1024 px шириной. Дополнительно проверены 1600 × 1000 и 390 × 844, DPR 1;
  временные viewport overrides сброшены. Desktop/mobile page overflow отсутствует.
- Full-view source и desktop screenshot открыты вместе в одном tool input.
  Сопоставлялись размеры компонентов в CSS px с учётом 2× source, а не raw
  размеры изображений; surrounding shell в референсе отсутствует и сохранён.
  Фокусный crop не нужен: сердце, колонки, шрифты и круги читаемы в full-view.
- Content/state различаются намеренно: у source 33 лайка / preview из 8 треков,
  у browser-аккаунта 2 настоящих лайка, у native `niag` 0. Названия, картинки,
  счётчик и вертикальная высота списка не подменялись ради совпадения.
- Evidence в ignored `mixora-client/release/collection-qa/`:
  `collection-redesign-desktop.jpg`, `collection-redesign-mobile.jpg`,
  `collection-redesign-mixes.jpg`, `collection-redesign-native.png`.
  Native screenshot 2760 × 1800 px, scale 2×; приложение 1380 × 900 CSS px.
- Итоговая локальная страница: `http://127.0.0.1:5174/collection`, отдаётся
  обновлённой packaged Mac-сборкой. Временный preview на 4174 остановлен.

### Findings, итерации и пять fidelity surfaces

1. [P1, исправлено] Огромные чёрные области рекомендаций на wide desktop:
   `1fr` растягивал cover, а image error делал его невидимым. После фикса
   три карточки 220 px, все локальные cover complete/naturalWidth > 0;
   mobile ширина 160 px и горизонтальный carousel. Cover error regression
   подтверждает видимую fallback-иконку.
2. [P1, исправлено] Структура не соответствовала выбранному скрину:
   отсутствовали subtitle, сердце и счётчик, рекомендации шли до исполнителей.
   После переноса заголовок → избранное → исполнители → рекомендации.
3. [P1, исправлено после browser capture] Общий старый `.collection-likes`
   задавал purple background и max-width 500 px, не позволяя получить колонки.
   Избранное переведено на отдельный `.collection-favorite-tracks`.
   Post-fix capture: прозрачный фон, desktop columns 475/475 px при 1260 px;
   на 1600 px две колонки 645/645. Mobile: одна 358 px колонка.
4. Типографика: сохранены YS Text / original heading font, h1 32 px,
   подпись и metadata 14 px, оригинальные веса и line heights; ellipsis
   длинных названий работает. Не вводились новые внешние шрифты.
5. Ритм/layout: сердце 56 px, track cover 40 px, bounded artists 180/148 px,
   две колонки с чтением сверху вниз, в узком контейнере — одна. Sidebar и
   bottom player сохранены; более раннее появление исполнителей у аккаунта
   с двумя лайками — ожидаемая разница данных, не искусственный spacer.
6. Цвета/tokens: тёмный фон, muted metadata и жёлтый акцент из существующего
   design system. Фиолетовый фон внутри списка удалён; hover ряда и focus
   действий работают. `цвет` — подпись, не неработающая ссылка на новую функцию.
7. Images: heart.602389ae.png из оригинального экспорта без нарисованной замены;
   track/artist artwork реальные. Недоступная картинка даёт note из имеющегося
   sprite, не чёрную дыру. Рекомендации используют bundled 1000 × 1000 JPEG,
   не размытый remote thumbnail на всю ширину.
8. Copy: текст заголовка соответствует source, число треков и русская форма
   берутся из библиотеки. Пустое состояние имеет работающий CTA поиска.
   Model explanation соответствует реальному ответу backend; snapshot явно
   не обещает автообновление сохранённого плейлиста.

### Поведение и границы

- Preview «Для вас» открывается и показывает реальные треки/источники;
  Listen/Save доступны, close работает. В этом дизайн-проходе ещё один плейлист
  не создавался; ранее подтверждённый save/reload сохранён, integration tests
  продолжают проверять play/save/feedback и account fencing.
- Mac ARM64 `.app` пересобрана и открыта на `/collection`. Аккаунт/очередь
  сохранены, paused position 176.2 s восстановлена после metadata.
- 126/126 client tests, format check, SHA-256 43 stylesheets и production
  build/macOS packaging пройдены. Browser errors в последнем проходе: нет.
- В checked visual scope открытых P0/P1/P2 нет. Отдельная data-quality задача:
  некоторые старые SoundCloud artist references (URN/numeric) отображаются
  повторно, а источники не всегда отдают портрет вместо album artwork.
  Этот проход не мигрировал artist identity и не подменял картинки портретами.
- Full live journey всех пяти источников в packaged Electron и измеренная
  релевантность рекомендаций остаются незакрытыми пунктами генерального плана.

Implementation checklist: image/spacing/column fixes применены, desktop/mobile
capture и native reopen выполнены, отчёт и план обновлены.

final result: passed
