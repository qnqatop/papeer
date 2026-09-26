## 1. Провайдер поиска

- [x] 1.1 Реализовать `internal/search/cyberleninka.go`: `NewCyberLeninka(client, email)` (поля `client`, `email`, `baseURL` test hook), `Name() = "cyberleninka"`, `Language() = "ru"`, DTO ответа (`found`, `articles` с `name`, `annotation`, `journal`, `year`, `authors`, `link`), `stripTags()` через regexp `<[^>]+>`, `Search()`: POST `{"mode":"articles","q","size","from":0}` через `client.PostJSON` с `extraHeaders {"User-Agent": PoliteUA(email)}`, ретраи 429/502/503/504 с бэкоффом 3/8/20 c (до 3 попыток), маппинг в `RawPaper` по таблице (DOI пустой, `PdfURL = "https://cyberleninka.ru" + link + "/pdf"`, `Source = "cyberleninka"`), `size = min(limit, 100)`, фильтр `yearMin` на клиенте
- [x] 1.2 Добавить тесты провайдера в `internal/search/providers_http_test.go` (образец — S2-тесты): маппинг полей, снятие `<b>`-тегов, пустой DOI, yearMin-фильтр, cap 100, проверка что запрос POST с корректным JSON-телом

## 2. Источник скачивания

- [x] 2.1 Реализовать `internal/download/sources/cyberleninka.go`: `NewCyberLeninka()`, `Name() = "cyberleninka"`, `Resolve()` без сетевых вызовов — если `info.PdfURL` содержит `cyberleninka.ru`, вернуть её; иначе `Reason: "not a cyberleninka paper"` (образец — `search_report.go`)
- [x] 2.2 Добавить тесты в `internal/download/sources/sources_http_test.go`: резолв cyberleninka-URL, отказ для чужого/пустого `PdfURL`

## 3. Referer при скачивании

- [x] 3.1 Добавить в `internal/httpclient/client.go` метод `DownloadFileWithReferer(ctx, rawURL, referer)` — общая приватная реализация существующего `DownloadFile` (те же UA-стратегии), referer подставляется на все попытки; старый `DownloadFile` остаётся обёрткой без изменений
- [x] 3.2 В `internal/download/fetcher.go` (в `FetchPDF`) вычислять referer по хосту: для `cyberleninka.ru` — URL с отрезанным суффиксом `/pdf`; заменить все внутренние вызовы `client.DownloadFile` на замыкание с referer (шаги 1, Akamai-рефетч, 3a, 3b)
- [x] 3.3 Тест на `DownloadFileWithReferer` в `internal/httpclient/client_test.go`: referer попал в заголовки запроса; старый путь без referer не регрессировал

## 4. Регистрация и конфигурация

- [x] 4.1 Зарегистрировать `search.NewCyberLeninka(client, profile.Email)` в оба списка провайдеров `SearchAxis` и `SearchAllAxes` (`internal/app/app.go`, ~663–667 и ~972–976)
- [x] 4.2 Добавить пробу `{"cyberleninka", "https://cyberleninka.ru", false}` (DoText на главную) в `CheckSearchProviders` (~414)
- [x] 4.3 Добавить `"cyberleninka": dlsources.NewCyberLeninka()` в `buildDownloadSources` (~870–876) и `"cyberleninka"` вторым элементом в `DefaultSourceOrder` после `search_report` (`internal/download/resolver.go:37`)
- [x] 4.4 Выставить `client.SetHostRateLimit("cyberleninka.ru", 0.5)` рядом с прочими per-host настройками в `makeHTTPClient` (`app.go`, ~910–915)

## 5. Языковой скоуп оси (lang_scope)

- [x] 5.1 Миграция и модель: `ALTER TABLE axes ADD COLUMN lang_scope TEXT NOT NULL DEFAULT 'en'` в `migrate()` (`internal/db/sqlite.go`, паттерн ~стр. 71); поле `db.Axis.LangScope string` (json `lang_scope`), включить в SELECT/INSERT/UPDATE в `internal/db/axis.go`
- [x] 5.2 Интерфейс: добавить `Language() string` в `search.Provider`; реализовать `"en"` у semantic_scholar/openalex/crossref/arxiv, `"ru"` у cyberleninka (обновить моки в `engine_test.go`, `providers_test.go`)
- [x] 5.3 Движок: скоуп читать прямо из `input.Axis.LangScope` (нормализация пустое = `"en"`), **без** отдельного поля в `SearchAxisInput`; пробросить скоуп в `searchQueryParallel` и фильтровать провайдеров по `p.Language() == scope`; в `app.go` ничего не менять — `Axis` уже передаётся целиком (`app.go:690`)
- [x] 5.4 Тест движка: ось `lang_scope = "ru"` — результаты только от RU-провайдера; `"en"`/пусто — только от EN-провайдеров (добавить RU-мок с кириллическими названиями)
- [x] 5.5 UI: контрол «Язык источников» (radio Англ./Рус.) в форме оси в `AxesView.vue` с биндингом `axis.lang_scope`; i18n-ключи `axes.langScope` + варианты в `frontend/src/i18n/{ru,en}.json`
- [x] 5.6 Проверка миграции: тест на старую БД без колонки — `lang_scope` читается как `"en"` (пример: существующий `sqlite_test.go`)

## 6. Верификация

- [x] 6.1 `go build ./...` и `go test ./internal/search/... ./internal/download/... ./internal/httpclient/... ./internal/db/...` — всё зелёное
- [ ] 6.2 Ручная проверка: ось `en` ищет только по англоязычным провайдерам; ось с `lang_scope = ru` по запросу «рекомендательная система абитуриент» возвращает русскоязычные статьи; approved-статья с CyberLeninka скачивается как валидный PDF (не капча); провайдер виден в настройках в `CheckSearchProviders`
- [ ] 6.3 Локализация: если заведены строки имён провайдеров для настроек — добавить ключ `cyberleninka` в `frontend/src/i18n/{ru,en}.json` (сейчас имена выводятся как есть — ключ не нужен)