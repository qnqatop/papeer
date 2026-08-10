## 1. Бэкенд: пакет `internal/updater/`

- [x] 1.1 Создать `internal/updater/updater.go` — структура `Updater` с полем `currentVersion string`, хелперы определения платформы (`platformKey()`, `archiveExt()`) и конструктор `NewUpdater(version string) *Updater`
- [x] 1.2 Реализовать `CheckForUpdates()` — GET-запрос к `https://api.github.com/repos/qnqatop/papeer/releases/latest`, разбор JSON (`tag_name` и `assets[].browser_download_url`), сравнение с `currentVersion` через `golang.org/x/mod/semver`, возврат `UpdateInfo{HasUpdate, LatestVersion, AssetURL, ReleaseURL, ReleaseNotes}` или ошибки
- [x] 1.3 Реализовать выбор URL ассета — сопоставление паттерна `papeer-{darwin|windows|linux}-*.{zip|tar.gz}` с именами файлов релиза; ошибка если ничего не подошло
- [x] 1.4 Реализовать `DownloadUpdate(assetURL string, onProgress func(downloaded, total int64))` — потоковая загрузка в `os.TempDir()`, вызов `onProgress` каждые 1 МБ, возврат пути к скачанному архиву; обработка обрыва соединения и HTTP-ошибок
- [x] 1.5 Реализовать `ExtractArchive(archivePath, destDir string)` — распаковка `.zip` (через `archive/zip`) и `.tar.gz` (через `archive/tar` + `compress/gzip`) в стейджинг-поддиректорию с сохранением прав файлов (exec-бит) и симлинков; проверка, что извлечённый бинарник/бандл существует и не пустой
- [x] 1.6 Реализовать `InstallAndRestart(stagingDir string)` — диспетчер, вызывающий платформенную функцию (1.7, 1.8 или 1.9) в зависимости от `runtime.GOOS`

### 1.7 Платформа: macOS

- [x] 1.7.1 Реализовать `installMacOS(stagingDir string) error` — найти текущий `.app`-бандл (от `os.Executable()` подняться до `Papeer.app`), отодвинуть старый бандл в соседний бэкап `<app>.old-<pid>` (тот же том, без Trash — избегаем `EEXIST`/`EXDEV`), скопировать новый из стейджинга на исходное место (`copyDir`, с сохранением симлинков), проверить целостность установленного бинарника (при провале — откат из бэкапа), удалить бэкап, `chmod +x`, перезапустить через `open -n <installed.app>` (фоллбэк — `exec.Command`; при полном провале — возврат ошибки без `os.Exit`), затем `os.Exit(0)`

### 1.8 Платформа: Windows

- [x] 1.8.1 Реализовать `installWindows(stagingDir string) error` — получить путь к `.exe` через `os.Executable()`, записать временный `update.ps1` в `%TEMP%` со скриптом: ждать 3с, `Copy-Item` нового `.exe` поверх старого, `Start-Process` нового `.exe`, `Remove-Item` самого скрипта
- [x] 1.8.2 Запустить PowerShell-скрипт через `os.StartProcess("powershell", ["-ExecutionPolicy", "Bypass", "-File", scriptPath])`, затем `os.Exit(0)`

### 1.9 Платформа: Linux

- [x] 1.9.1 Реализовать `installLinux(stagingDir string) error` — получить путь текущего бинарника через `os.Executable()`, скопировать новый бинарник во временный файл рядом с целевым (`<exe>.new`, та же ФС — иначе `os.Rename` из `/tmp` даёт `EXDEV`), `os.Rename` поверх старого, проверить целостность, вызвать `syscall.Exec` с теми же `os.Args`

## 2. Бэкенд: Wails-биндинги в `internal/app/app.go`

- [x] 2.1 Добавить поле `updater *updater.Updater` в структуру `App` и инициализировать в `NewApp()` с вшитой строкой `Version`
- [x] 2.2 Реализовать экспортируемый метод `CheckForUpdates() (updater.UpdateInfo, error)` — тонкая обёртка над `app.updater.CheckForUpdates()`
- [x] 2.3 Реализовать экспортируемый метод `DownloadUpdate(ctx context.Context, assetURL string) error` — вызывает `app.updater.DownloadUpdate()` с колбэком прогресса, который шлёт `update:download-progress` события через `runtime.EventsEmit(ctx, ...)` с полями `{percentage, downloaded, total}`; после загрузки распаковывает архив; сохраняет путь стейджинга в поле `App` для следующего шага
- [x] 2.4 Реализовать экспортируемый метод `InstallAndRestart() error` — вызывает `app.updater.InstallAndRestart()` с сохранённым путём стейджинга; возвращает ошибку только при провале предполётных проверок (при успехе процесс завершается)
- [x] 2.5 Реализовать публичный метод `AppVersion() string` (если ещё нет) для показа текущей версии во фронтенде

## 3. Фронтенд: UI в настройках

- [x] 3.1 Добавить ключи локализации EN/RU в `frontend/src/i18n/` для всех строк обновлений (кнопка проверки, «всё актуально», «доступна новая версия», кнопка скачивания, прогресс загрузки, кнопка перезапуска, тексты ошибок)
- [x] 3.2 Добавить секцию «Обновления» во вкладку «О программе» в SettingsView: отображение текущей версии, кнопка «Проверить обновления», текст статуса, прогресс-бар загрузки (Naive UI `n-progress`), кнопка «Перезапустить для обновления»
- [x] 3.3 Вынести логику обновлений в композабл `composables/useUpdater.ts` (`updateStatus`, `latestVersion`, `downloadPercentage`, `isDownloading`, `isUpdateReady`, `updateError` + методы `checkUpdates`/`downloadUpdate`/`installUpdate`); подписка на `update:download-progress` через `EventsOn` с отпиской в `onUnmounted`. SettingsView — тонкая обёртка с тостами и локализацией ошибок
- [x] 3.4 Привязать кнопку «Проверить обновления» → вызов `CheckForUpdates()`, обновление состояния по результату
- [x] 3.5 Привязать кнопку «Скачать обновление» → вызов `DownloadUpdate()`, установка `isDownloading` и подписка на события `update:download-progress` через `EventsOn`
- [x] 3.6 Привязать кнопку «Перезапустить для обновления» → вызов `InstallAndRestart()`; показать короткое сообщение «Перезапуск...» перед завершением приложения

## 3a. Пассивная проверка при запуске

- [x] 3a.1 Backend: добавить поле `Severity` в `UpdateInfo` и функцию `versionBump(current, latest)` (major/minor/patch/none через `semver.Major`/`MajorMinor`); заполнять в `CheckForUpdates`
- [x] 3a.2 Backend: `checkForUpdatesPassive()` в `Startup` (в горутине, с задержкой) — уважает настройку `auto_update_check`, показывает только major/minor, пропускает `update_dismissed_version`, эмитит `update:available`; поле `pendingUpdate` под мьютексом + метод `PendingUpdate()` для backfill
- [x] 3a.3 Frontend: компонент `components/layout/UpdateBanner.vue` — подписка на `update:available` + `PendingUpdate()` на mount, баннер сверху с «Обновить» (→ `settings#updates`) и «✕» (→ `SaveSetting('update_dismissed_version')`); подключить в `AppShell.vue`
- [x] 3a.4 Frontend: тумблер «Проверять обновления при запуске» в панели (setting `auto_update_check`, вкл по умолчанию); авто-запуск проверки при переходе с хэшем `#updates`
- [x] 3a.5 Frontend: ключи i18n `updates.section`, `updates.autoCheck`, `updates.banner.{available,action}` для EN/RU
- [x] 3a.6 Перегенерировать Wails-биндинги (`wails generate module`) — метод `PendingUpdate`, поле `severity`

## 4. Тестирование

### 4.A Unit-тесты (запускаются в CI на любой платформе)

#### Логика версий и парсинг API

- [x] 4.1 Табличные тесты сравнения версий через semver: `v1.2.3 > v1.2.2 = true`, `v2.0.0 > v1.9.9 = true`, `v1.0.0 == v1.0.0`, `v1.0.0-beta < v1.0.0`, пустая строка → ошибка, битая строка `abc` → ошибка, отсутствие `v`-префикса в одном из тегов
- [x] 4.1a Табличные тесты `versionBump`: major/minor/patch/none, без `v`-префикса, неразбираемые версии (`TestVersionBump`)
- [x] 4.2 Тест парсинга ответа GitHub API: `httptest.NewServer` возвращает валидный JSON с `tag_name`, `html_url`, `assets[]`, `body` — проверяем, что `UpdateInfo` заполнен корректно (все поля совпадают)
- [x] 4.3 Тест парсинга ответа с отсутствующими полями: нет `assets` → ошибка, `tag_name` пустой → ошибка, `assets` пустой массив → ошибка «ассеты не найдены»

#### Выбор ассета под платформу

- [x] 4.4 Тест выбора ассета: для `darwin` выбирается `papeer-macos-universal.zip`, для `windows` → `papeer-windows-amd64.zip`, для `linux` → `papeer-linux-amd64.tar.gz`, игнорируются чужие ассеты
- [x] 4.5 Тест: два ассета под одну платформу (например, `amd64` и `arm64`) — выбирается первый подходящий
- [x] 4.6 Тест: нет ассета под текущую платформу → ошибка «платформа не поддерживается»

#### HTTP и сетевые сценарии

- [x] 4.7 Тест: GitHub API возвращает HTTP 200, но парсинг JSON ломается (битый JSON) → ошибка парсинга
- [x] 4.8 Тест: GitHub API возвращает HTTP 403 (rate limit) → ошибка с упоминанием лимита и предложением повторить позже
- [x] 4.9 Тест: GitHub API возвращает HTTP 404 (релизов нет) → ошибка
- [x] 4.10 Тест: GitHub API возвращает HTTP 500/502/503 → ошибка «сервер недоступен»
- [x] 4.11 Тест: таймаут соединения (мок-сервер спит >N сек) → ошибка таймаута, а не паника

#### Загрузка и прогресс

- [x] 4.12 Тест потоковой загрузки: `httptest.NewServer` отдаёт N байт тела, проверяем что `DownloadUpdate` скачал ровно N байт, колбэк `onProgress` вызвался ожидаемое число раз
- [x] 4.13 Тест: сервер обрывает соединение на середине передачи → ошибка загрузки, временный файл удалён
- [x] 4.14 Тест: сервер возвращает Content-Length, не совпадающий с реальным телом → ошибка целостности

#### Распаковка архива

- [x] 4.15 Тест распаковки `.zip`: фикстура — валидный zip с одним файлом `papeer` внутри. После `ExtractArchive` файл существует, размер > 0
- [x] 4.16 Тест распаковки `.tar.gz`: фикстура — валидный tar.gz. После `ExtractArchive` файлы на месте
- [x] 4.17 Тест: битый zip (обрезан наполовину) → ошибка распаковки, стейджинг-директория зачищена
- [x] 4.18 Тест: битый tar.gz → ошибка распаковки
- [x] 4.19 Тест: архив без ожидаемого бинарника внутри (пустой архив или левые файлы) → ошибка «бинарник не найден»

#### Генерация скриптов установки

- [x] 4.20 Тест `installWindows`: проверяем что сгенерированный `update.ps1` содержит `Copy-Item`, `Start-Process`, `Remove-Item $MyInvocation.MyCommand.Path` (самоудаление), в путях подставлены конкретные значения (без вложенных переменных)
- [x] 4.21 Тест `installWindows`: проверяем что PS-скрипт записан в `%TEMP%` и команда запуска содержит `-ExecutionPolicy Bypass -File`
- [x] 4.22 Тест `installMacOS`: проверяем что путь к `.app` корректно вычисляется из `os.Executable()` (например, `/Applications/Papeer.app/Contents/MacOS/papeer` → `/Applications/Papeer.app`)
- [x] 4.23 Тест `installLinux`: проверяем что путь бинарника совпадает с `os.Executable()`

#### Интеграция: end-to-end логики (без реальной замены)

- [x] 4.24 Тест полного цикла `Updater` на моках: `CheckForUpdates` → фейковый релиз новее → `DownloadUpdate` (мок-сервер отдаёт фикстурный архив) → `ExtractArchive` → проверяем что стейджинг содержит валидный бинарник, `HasUpdate == true`

### 4.B Frontend-тесты (Vitest)

Тесты покрывают композабл `useUpdater` (моки Wails-биндингов и `EventsOn`) — файл `frontend/src/__tests__/updater.test.ts`. Видимость/`disabled` кнопок в шаблоне детерминированно выводится из этого состояния.

- [x] 4.25 Исходное состояние: `updateStatus === 'idle'`, `isDownloading/isUpdateReady === false` (кнопка «Проверить» видна, прогресс-бар и «Перезапустить» скрыты)
- [x] 4.26 После успешной загрузки `isUpdateReady === true` (кнопка «Перезапустить для обновления» видна)
- [x] 4.27 Во время загрузки `isDownloading === true` (кнопки заблокированы), сбрасывается после завершения

### 4.C Ручные smoke-тесты (под каждой платформой)

- [x] 4.28 macOS: dev-сборка с `Version=v0.0.1`, полный цикл — проверка, загрузка, замена `.app`, перезапуск, приложение запускается с новой версией
- [x] 4.29 Windows: то же самое, проверка что PS-скрипт отрабатывает, `.exe` заменён, новое приложение стартует
- [x] 4.30 Linux: то же самое, проверка `syscall.Exec` подмены процесса

### 4.D Проверка качества кода

- [x] 4.31 `go test -race ./...` — без новых гонок
- [x] 4.32 `go vet ./...` — без предупреждений
- [x] 4.33 `npm run typecheck` (или `vue-tsc --noEmit`) — фронтенд без ошибок типов
