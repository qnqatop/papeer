## Purpose

Интеграция CyberLeninka (крупнейшей русскоязычной базы статей открытого
доступа) в Papeer: поиск статей через API `/api/search`, резолв и скачивание
PDF с обязательным Referer и вежливым режимом запросов.

## ADDED Requirements

### Requirement: Поиск статей через провайдер cyberleninka

The `cyberleninka` provider SHALL participate in search only for axes whose
language scope is `ru` and SHALL search Russian-language articles via POST
`https://cyberleninka.ru/api/search` с телом
`{"mode":"articles","q":<query>,"size":<n>,"from":<offset>}`.

Маппинг записи ответа в `RawPaper`: `Title` ← `name` (HTML-теги сняты),
`Abstract` ← `annotation`, `Venue` ← `journal`, `Year` ← `year`,
`Authors` ← `authors`, `DOI` ← `""` (у CyberLeninka DOI нет),
`PdfURL` ← `"https://cyberleninka.ru" + link + "/pdf"`, `Source` ←
`"cyberleninka"`.

#### Scenario: поиск возвращает статьи
- **WHEN** провайдер `cyberleninka` получает поисковый запрос и API возвращает
  статьи
- **THEN** провайдер возвращает `RawPaper` с полями, отмапленными по таблице
  маппинга, HTML-теги (`<b>`-подсветка и прочие) сняты с `name`, `annotation`
  и `journal`

#### Scenario: ограничение лимита и фильтр по году
- **WHEN** запрошенный `limit` превышает 100 или задан `yearMin`
- **THEN** в `size` уходит значение не больше 100, а статьи со `year` меньше
  `yearMin` отбрасываются на стороне клиента (API фильтра по году не даёт)

#### Scenario: ошибка API
- **WHEN** API отвечает ошибкой 4xx/5xx или транспортом
- **THEN** провайдер возвращает ошибку, а поиск продолжает работу с остальными
  провайдерами (circuit breaker движка срабатывает как для прочих провайдеров)

### Requirement: Источник скачивания cyberleninka

The `cyberleninka` source SHALL resolve a PDF URL from the pre-found
`info.PdfURL` only when it points to the `cyberleninka.ru` host, and SHALL return
it unchanged (it is the final PDF endpoint).

#### Scenario: PDF-ссылка с CyberLeninka
- **WHEN** `info.PdfURL` содержит `cyberleninka.ru`
- **THEN** источник возвращает её как результат резолва

#### Scenario: ссылка другого провайдера
- **WHEN** `info.PdfURL` пустая или ведёт на другой хост
- **THEN** источник возвращает пустой URL с причиной `not a cyberleninka paper`,
  и цепочка источников переходит к следующему

### Requirement: Referer при скачивании PDF с cyberleninka.ru

When downloading a PDF from `https://cyberleninka.ru<link>/pdf`, every GET
MUST carry the `Referer: https://cyberleninka.ru<link>` header (article page)
and `Accept: application/pdf,*/*`. Without the Referer the server answers with a
captcha/HTML instead of a PDF.

#### Scenario: скачивание PDF
- **WHEN** Papeer выполняет GET на `https://cyberleninka.ru/.../pdf`
- **THEN** запрос содержит Referer на страницу статьи (URL без суффикса
  `/pdf`) и Accept, и полученные байты проходят валидацию `%PDF-` +
  минимальный размер существующим валидатором

### Requirement: Вежливый режим к API CyberLeninka

Requests to `cyberleninka.ru` MUST be limited by a per-host rate-limit (~1
request per 1.2–2 s) via the existing `SetHostRateLimit` mechanism, and search
POSTs MUST be retried on 429/502/503/504 with 3/8/20 s backoff (up to 3
attempts).

#### Scenario: rate-limit
- **WHEN** поиск и скачивание идут через `cyberleninka.ru`
- **THEN** все запросы к хосту проходят через per-host rate-limit с
  интервалом не чаще ~1 запроса в 1.2 c

#### Scenario: временные ошибки API
- **WHEN** API отвечает 429/502/503/504
- **THEN** провайдер повторяет запрос с бэкоффом 3/8/20 c и не более 3
  попыток, а при исчерпании возвращает ошибку

### Requirement: Проверка доступности провайдера

The `cyberleninka` provider MUST be included in `CheckSearchProviders` and
shown in settings alongside the other providers (name displayed as-is, no
localization).

#### Scenario: статус доступности
- **WHEN** пользователь запускает проверку доступности поисковых провайдеров
  в настройках
- **THEN** в списке статусов присутствует `cyberleninka` с результатом
  проверки и замером задержки

### Requirement: Языковой скоуп поиска на оси

Каждая ось MUST иметь атрибут `lang_scope` со значением `en` (по умолчанию)
или `ru`. Поиск по оси MUST использовать только те провайдеры, чей
объявленный язык (`Language()`) совпадает со скоупом оси: скоуп `en` —
англоязычные провайдеры, скоуп `ru` — русскоязычные (набор RU-источников
расширяемый; сейчас — `cyberleninka`). Отсутствующее значение трактуется
как `en`. Другие оси флагом не затрагиваются.

#### Scenario: ось со скоупом ru
- **WHEN** у оси установлен `lang_scope = ru` и запускается её поиск
- **THEN** запросы уходят только провайдерам с языком `ru` (сейчас —
  `cyberleninka`), англоязычные провайдеры не вызываются

#### Scenario: ось со скоупом en или без значения
- **WHEN** у оси установлен `lang_scope = en` или атрибут отсутствует
  (старая база данных)
- **THEN** запросы уходят только англоязычным провайдерам (semantic_scholar,
  openalex, crossref, arxiv), а `cyberleninka` не вызывается

#### Scenario: RU-источники недоступны при ru-скоупе
- **WHEN** ось со скоупом `ru` ищет, но все RU-провайдеры отвечают ошибкой
- **THEN** поиск по оси завершается с пустым результатом и событием
  ожидаемой ошибки провайдера, а остальные оси продолжают работать без
  изменений