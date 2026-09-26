## Why

Papeer работает с четырьмя англоязычными OA-базами (Semantic Scholar, OpenAlex,
Crossref, arXiv) и не находит/не скачивает русскоязычную литературу вообще. Для
русскоязычных обзоров это структурная дыра: пользователь держит отдельные
Python-скрипты вне приложения. Подключение CyberLeninka — крупнейшей RU-базы
статей открытого доступа — закрывает весь русскоязычный поток (и поиск, и
скачивание PDF) внутри Papeer.

## What Changes

- Новый провайдер поиска `cyberleninka` в `internal/search/`: реализует
  `search.Provider` через недокументированный, но публичный POST-эндпоинт
  `https://cyberleninka.ru/api/search` (формат запроса/ответа подтверждён в боевых
  Python-скриптах диссертационного репозитория).
- Языковой скоуп оси `lang_scope` (`en` по умолчанию / `ru`): оси без флага
  ищут **только** в англоязычных источниках (semantic_scholar, openalex,
  crossref, arxiv), оси с флагом — **только** в русскоязычных (сейчас
  `cyberleninka`, набор расширяемый). Провайдеры объявляют язык сами через
  метод `Language()` интерфейса `search.Provider`.
- Новый источник скачивания `cyberleninka` в `internal/download/sources/`:
  резолвит PDF по пред-найденному `info.PdfURL` хоста `cyberleninka.ru`.
- Обязательный заголовок `Referer` (страница статьи) на GET PDF — без него
  CyberLeninka отдаёт капчу/HTML вместо PDF.
- Вежливый per-host rate-limit для `cyberleninka.ru`.
- Регистрация провайдера и источника в `internal/app/app.go` (оба axis-списка,
  `CheckSearchProviders`, `buildDownloadSources`, `DefaultSourceOrder`).
- Ретраи на 429/502/503/504 (бэкофф 3/8/20 c) при поиске.
- Локализация: имена провайдеров в UI показываются как есть, поэтому i18n-строки
  не требуются; при появлении локализованного показа имён — завести ключи
  `cyberleninka` в `frontend/src/i18n/{ru,en}.json`.

## Capabilities

### New Capabilities
- `cyberleninka`: интеграция CyberLeninka в Papeer как единого русскоязычного
  источника: поиск через `/api/search`, резолв PDF-ссылок, обязательный Referer,
  вежливый режим (rate-limit + ретраи), проверка доступности провайдера.

### Modified Capabilities
<!-- нет: `openspec/specs/` пуст, существующих спек нет -->

## Impact

- `internal/search/cyberleninka.go` — новый провайдер (+ тест в
  `providers_http_test.go`).
- `internal/search/provider.go` — интерфейс `Provider` дополняется методом
  `Language() string`.
- `internal/search/engine.go` — фильтр провайдеров по `lang_scope` оси
  (скоуп читается из `input.Axis.LangScope`, без нового поля в
  `SearchAxisInput`).
- `internal/db/` — колонка `axes.lang_scope` (миграция), поле `Axis.LangScope`.
- `frontend/src/views/AxesView.vue` + `frontend/src/i18n/{ru,en}.json` —
  контрол «Язык источников» в форме оси.
- `internal/download/sources/cyberleninka.go` — новый источник (+ тест).
- `internal/httpclient/client.go` — опционально новый метод для скачивания с
  per-request `Referer` (овариант: обработка по хосту в `fetcher.go`).
- `internal/download/fetcher.go` — простановка `Referer` для
  `cyberleninka.ru`.
- `internal/app/app.go` — регистрация в: списки провайдеров `SearchAxis` /
  `SearchAllAxes`, пробы `CheckSearchProviders`, `buildDownloadSources`,
  `DefaultSourceOrder`, `SetHostRateLimit` для `cyberleninka.ru`.
- Внешние зависимости: только HTTP API CyberLeninka (не задокументирован, но
  стабилен); никаких новых Go-зависимостей.