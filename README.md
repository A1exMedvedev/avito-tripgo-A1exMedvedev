# TripGo — Trip Service

Сервис поездок учебного проекта TripGo (курс «Разработка микросервисов на Go»).
Задания и контракты — в [`course-go-autumn-2026/course`](https://github.com/course-go-autumn-2026/course).

Сейчас сделаны:

- **лабораторная работа 1**: HTTP API по OpenAPI-контракту, PostgreSQL, миграции и
  менеджер транзакций, плюс оба задания со звёздочкой: идемпотентность по
  `Idempotency-Key` и многостадийный `Dockerfile`;
- **лабораторная работа 2**: код разложен по слоям, есть тесты, линтер, логи, метрики
  и трейсы. Снаружи сервис ведёт себя так же, новых ручек и миграций нет.

## Требования

- Go 1.27+ (версия из `go.mod`; код использует `errors.AsType` и `new(expr)` из Go 1.26)
- [`tripgoctl`](https://github.com/course-go-autumn-2026/course-infra) и Docker для
  локального окружения
- `oapi-codegen`, `goose` и `mockgen` ставить отдельно не нужно: они подключены в
  `go.mod` как `tool` и запускаются через `go tool`
- `golangci-lint` тоже: `make lint` запускает закреплённую версию через `go run`
- Docker для `make test-integration`

## Запуск

```bash
tripgoctl cluster start        # один раз
tripgoctl environment start    # поднимает PostgreSQL и коллектор, пишет адреса в .env
make migrate                   # накатить миграции
make run                       # сервис на $HTTP_ADDR (по умолчанию :8080)
```

`Makefile` сам подхватывает переменные из `.env`. Другой файл можно передать через
`make run ENV_FILE=...`.

| Цель | Что делает |
|---|---|
| `make generate` | генерирует `api/api.gen.go` из контракта и моки через `go generate` |
| `make migrate` | накатывает все миграции |
| `make migrate-down` | откатывает последнюю миграцию |
| `make migrate-reset` | откатывает все миграции |
| `make migrate-status` | показывает статус миграций |
| `make run` | запускает сервис |
| `make build` | собирает бинарь в `bin/trip-service` |
| `make test` | все unit-тесты с `-race`, печатает итоговое покрытие `internal/domain` и `internal/usecase` |
| `make test-integration` | интеграционные тесты репозитория на настоящем PostgreSQL (`testcontainers-go`, нужен Docker) |
| `make lint` | `golangci-lint` с конфигом курса |
| `make fmt` | форматтеры `gofumpt` и `gci` |
| `make docker-build` | собирает образ `trip-service:local` из `deploy/Dockerfile` |
| `make docker-run` | запускает образ рядом с PostgreSQL из `tripgoctl` (сеть хоста, переменные из `.env`) |

## Переменные окружения

Все обязательные переменные проверяются на старте. Если какой-то нет, сервис
не запускается и перечисляет, чего не хватает. Пример — [`.env.example`](.env.example).

| Переменная | Обязательна | Пример | Что задаёт |
|---|---|---|---|
| `HTTP_ADDR` | да | `:8080` | адрес HTTP-сервера |
| `LOG_LEVEL` | да | `info` | `debug`, `info`, `warn`, `error` |
| `SHUTDOWN_TIMEOUT` | да | `10s` | общий бюджет на graceful shutdown |
| `DATABASE_URL` | да | `postgres://…` | строка подключения к PostgreSQL |
| `DATABASE_MAX_CONNS` | да | `10` | максимум соединений в пуле |
| `DATABASE_MIN_CONNS` | да | `2` | минимум соединений в пуле |
| `DATABASE_MAX_CONN_LIFETIME` | да | `30m` | время жизни соединения |
| `DATABASE_CONNECT_TIMEOUT` | да | `5s` | таймаут подключения и стартового `Ping` |
| `DATABASE_QUERY_TIMEOUT` | да | `3s` | таймаут одного запроса к базе и проверки `/ready` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | да | `http://localhost:4318` | адрес коллектора телеметрии |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | нет | `http/protobuf` | поддержан только `http/protobuf`, с другим значением сервис не стартует |
| `OTEL_SERVICE_NAME` | нет | `trip-service` | `service.name` и поле `service` в логах |
| `OTEL_RESOURCE_ATTRIBUTES` | нет | `service.namespace=tripgo,deployment.environment=local` | атрибуты ресурса, читает SDK |
| `OTEL_TRACES_SAMPLER` | нет | `parentbased_always_on` | сэмплирование, читает SDK |
| `OTEL_METRIC_EXPORT_INTERVAL` | нет | `60000` | период отправки метрик в мс, читает SDK |
| `HTTP_READ_TIMEOUT` | нет | `10s` | `http.Server.ReadTimeout` |
| `HTTP_READ_HEADER_TIMEOUT` | нет | `5s` | `http.Server.ReadHeaderTimeout` |
| `HTTP_WRITE_TIMEOUT` | нет | `15s` | `http.Server.WriteTimeout` |
| `HTTP_IDLE_TIMEOUT` | нет | `60s` | `http.Server.IdleTimeout` |
| `IDEMPOTENCY_KEY_TTL` | нет | `24h` | сколько живёт ключ `Idempotency-Key` |

У необязательных переменных есть значения по умолчанию, и сервис запускается с `.env`,
который сгенерировал `tripgoctl`.

## API

Источник истины — [`contracts/openapi/trip-service.openapi.yaml`](contracts/openapi/trip-service.openapi.yaml).

| Метод | Путь | Ответы |
|---|---|---|
| `POST` | `/api/v1/trips` | `201` + `Location`, `200` на повтор с тем же `Idempotency-Key`, `400`, `409 driver_busy` / `idempotency_conflict`, `500` |
| `GET` | `/api/v1/trips/{tripId}` | `200`, `400`, `404`, `500` |
| `POST` | `/api/v1/trips/{tripId}/finish` | `200`, `400`, `404`, `409 trip_completed`, `500` |
| `GET` | `/health` | `200` |
| `GET` | `/ready` | `200`, `503` |

Ошибки отдаются по RFC 9457 в `application/problem+json` с полем `code`. Текст
ошибок базы и SQL в ответ не попадает, только в лог.

```bash
curl -si -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' \
  -d '{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
       "driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
       "start_point":{"latitude":59.9398,"longitude":30.3146},
       "end_point":{"latitude":59.9290,"longitude":30.3626},
       "price":1450}'
```

## Архитектура

Зависимости направлены внутрь: транспорт и репозитории знают про usecase и домен,
usecase знает про домен, домен не знает ни про кого. Правила проверяет `depguard`
из `.golangci.yml`, поэтому нарушение ломает `make lint`.

```
cmd/trip-service ──► internal/config
        │
        ▼
internal/container          единственное место, где создаются реализации
   │         │        │
   ▼         ▼        ▼
http/*   usecase/*   repository/*  ──► infra/postgres
   │         │             │           infra/telemetry
   │         ▼             │
   └──► domain, model ◄────┘
```

| Пакет | Что внутри | Что знает |
|---|---|---|
| `cmd/trip-service` | читает конфиг, зовёт `container.New`, ждёт сигнала | `config`, `container` |
| `internal/config` | разбор и проверка переменных окружения | ничего |
| `internal/container` | создаёт реализации и связывает их | все пакеты |
| `internal/domain/trip` | поездка, статусы и переход `Finish`, правила, ошибки | ничего |
| `internal/model` | структуры, которые ходят между слоями | домен |
| `internal/usecase/trip/{create,get,finish}` | сценарии: транзакции, бизнес-логи, бизнес-метрики | домен, `model` |
| `internal/http/trip/{create,get,finish}` | разбор запроса и ответ, по пакету на ручку | домен, `model`, `api` |
| `internal/http/health` | `/health` и `/ready` | `api` |
| `internal/http/domainerror` | единственный маппинг ошибки в `Problem` | домен, `api` |
| `internal/http/middleware` | `Recover → Tracing → Logging → Metrics` | OTel, chi |
| `internal/http/router` | роутер, валидация по OpenAPI, порядок middleware | `api`, `http/*` |
| `internal/http/response` | запись JSON и `problem+json`, поездка в DTO | домен, `api` |
| `internal/repository/trip` | SQL | домен, `model`, `infra/postgres/transaction` |
| `internal/infra/postgres` | пул pgx и трейсер запросов | `config`, OTel |
| `internal/infra/postgres/transaction` | менеджер транзакций | pgx |
| `internal/infra/telemetry` | провайдеры OTel, логгер, бизнес-метрики | `config`, OTel |

### Где объявлены интерфейсы и почему там

Интерфейс лежит в `contract.go` того пакета, который его использует, и не
экспортируется. Usecase формулирует, что ему нужно, а реализаций может быть несколько.
Поэтому интерфейсы узкие: `usecase/trip/get` просит только `Fetch`, хотя `repository/trip.Repo`
умеет больше. Моки генерирует `mockgen` в `mocks_test.go` рядом с кодом.

Что изменится со вторым транспортом в ЛР4: появится `internal/grpc/trip/...` со своими
интерфейсами на те же usecase и свой маппинг ошибок в коды gRPC. Usecase, домен и
репозитории не меняются, в `container` добавляется сборка сервера.

### Отступления от раскладки задания

- `cmd/trip-service`, а не `cmd/service`: так записано в разделе 8 `conventions.md`.
- `api/`, а не `internal/generated/`: то же самое, и на `api/` рассчитаны исключения
  в конфиге линтера курса.
- Добавлены `http/router`, `http/response` и `http/health`. Роутеру и общей записи ответа
  нет места в пакете одной ручки, а `/health` и `/ready` не относятся к поездкам.
- `convert.go` есть только у `http/trip/create`. Перевод поездки в ответ один на три ручки,
  он лежит в `http/response/trip.go`, чтобы не копировать его трижды.
- Правила пакета на действие и инфраструктуры отдельно от бизнес-логики соблюдены.

## Тесты

| Что | Где | Чем |
|---|---|---|
| Домен: `Finish`, валидация | `internal/domain/trip` | table-driven, без моков |
| Usecase | `internal/usecase/trip/*` | `gomock` на репозиторий, `txManager`, метрики |
| Маппинг ошибок | `internal/http/domainerror` | table-driven |
| Хендлеры | `internal/http/trip/*`, `internal/http/health` | `httptest` + мок usecase |
| Репозиторий, транзакции | `internal/repository/trip` | настоящий PostgreSQL, тег `integration` |

Где искать обязательные сценарии:

| Сценарий | Тест |
|---|---|
| создание — успех | `usecase/trip/create: TestDo_Success`, `http/trip/create: TestHandle_Created` |
| занятый водитель → `409` | `usecase/trip/create: TestDo_DriverBusy`, `http/trip/create: TestHandle_UsecaseErrors`, `repository/trip: TestSave_DriverBusy` |
| завершение активной — успех | `usecase/trip/finish: TestDo_ActiveTrip`, `http/trip/finish: TestHandle_OK` |
| завершение завершённой → `409` | `domain/trip: TestFinish`, `usecase/trip/finish: TestDo_CompletedTrip_NothingIsWritten`, `http/trip/finish: TestHandle_Errors` |
| несуществующая → `404` | `usecase/trip/get: TestDo`, `http/trip/get: TestHandle_NotFound` |
| невалидное тело → `400 invalid_request` | `http/trip/create: TestHandle_InvalidBody` |
| ошибка репозитория → `500` без деталей | `http/trip/get: TestHandle_RepositoryFailure_HidesDetails`, `http/domainerror: TestWrite` |
| откат `INSERT` в двух таблицах | `repository/trip: TestTransaction_RollbackRevertsBothTables` |

Интеграционные тесты сами поднимают контейнер с PostgreSQL и накатывают миграции. Каждый
тест создаёт поездку со случайными идентификаторами и удаляет её в `t.Cleanup`, поэтому
тесты изолированы и проходят при повторном запуске.

Если Docker работает внутри Lima или Colima, контейнер-уборщик testcontainers не может
смонтировать сокет. Тесты и так удаляют свой контейнер сами, поэтому уборщик можно отключить:

```bash
DOCKER_HOST=unix://$HOME/.lima/<instance>/sock/docker.sock TESTCONTAINERS_RYUK_DISABLED=true make test-integration
```

## Observability

Метрики и трейсы уходят по OTLP в коллектор, логи — JSON в stdout. Как проверить:

- Jaeger (ссылка из `tripgoctl connect`): в трейсе запроса виден корневой спан
  `POST /api/v1/trips` и вложенные `BEGIN`, `INSERT trips`, `INSERT trip_status_history`, `COMMIT`;
- Grafana: `http_server_request_duration_seconds`, `tripgo_trips_created_total`;
- `trace_id` из записи `request handled` находит этот же трейс в Jaeger.

## Решения

### Кодогенерация и валидация

`oapi-codegen` генерирует типы, chi-сервер и встроенную спеку
(`api/oapi-codegen.yaml`), но только для пяти операций этой работы. Nullable-поля
генерируются как `nullable.Nullable`, поэтому `finished_at` и `last_position_at`
отдаются явным `null`, как в примерах контракта.

Тело запроса проверяется по той же спеке middleware `oapi-codegen/nethttp-middleware`
(kin-openapi): обязательные поля, диапазоны координат, `price >= 0`,
`additionalProperties: false`. Правила не дублируются руками и не расходятся с
контрактом. В домене (`trip.CreateParams.Validate`) те же инварианты проверяются
ещё раз, потому что сервис будут вызывать не только по HTTP (в ЛР4 появится Kafka).

### Уровень изоляции — `READ COMMITTED`

Это уровень PostgreSQL по умолчанию, и его достаточно: оба правила, чувствительные
к гонкам, держатся не на уровне изоляции, а на конкретных механизмах.

- **Две активные поездки у водителя** запрещает частичный уникальный индекс. Он
  работает на любом уровне изоляции.
- **Двойное завершение** запрещает условный `UPDATE ... WHERE status = 'active'`.
  В `READ COMMITTED` второй `UPDATE` ждёт блокировку строки, затем перепроверяет
  `WHERE` на уже закоммиченной версии и ничего не обновляет.

`REPEATABLE READ` и `SERIALIZABLE` здесь ничего бы не добавили. При этом
параллельные запросы стали бы получать ошибки сериализации (`40001`), и под них
пришлось бы писать ретраи. Уровень задаётся при создании менеджера в `internal/container`
(`transaction.NewManager(pool, pgx.ReadCommitted, …)`), так что при необходимости
его легко поменять.

### Менеджер транзакций

`internal/infra/postgres/transaction/manager.go`, интерфейс
`Do(ctx, fn func(ctx) error) error`.

- `Do` открывает транзакцию через `pool.BeginTx` с заданным уровнем изоляции,
  кладёт `pgx.Tx` в `context` под приватным ключом и вызывает `fn` с новым контекстом.
- `fn` вернула `nil` — `COMMIT`. Вернула ошибку — `ROLLBACK`, ошибка уходит
  наружу. Паника — `ROLLBACK` и паника пробрасывается дальше, в HTTP её ловит
  `middleware.Recover` и отвечает `500`. `ROLLBACK` выполняется на контексте без отмены
  (`context.WithoutCancel` + таймаут): откатиться надо и тогда, когда клиент уже
  отключился.
- `BEGIN` и `COMMIT` ограничены тем же таймаутом `DATABASE_QUERY_TIMEOUT`.
- Репозиторий получает исполнителя через `Manager.Querier(ctx)`. Если в
  контексте есть транзакция, запрос идёт через неё, если нет — через пул.
  Транзакция не передаётся аргументом, поэтому usecase не знает про `pgx` и зависит
  только от своего интерфейса `txManager`.
- Вложенный `Do` видит транзакцию в контексте и просто вызывает `fn` в ней, вторую
  не открывает. Коммитит или откатывает только внешний `Do`.
- Вызов репозитория вне `Do` работает через пул: каждый запрос выполняется в
  своей автокоммит-транзакции.

Создание поездки — это один `Do`: `INSERT` в `trips` и `INSERT` в
`trip_status_history`. Завершение — тоже один `Do`: чтение, условный `UPDATE` и
запись в журнал.

### Запрет двух активных поездок

```sql
CREATE UNIQUE INDEX trips_driver_id_active_uniq ON trips (driver_id) WHERE status = 'active';
```

Проверка `SELECT` перед `INSERT` не работает: оба параллельных запроса пройдут
её раньше, чем кто-то из них вставит строку. Уникальный индекс проверяет
вставку атомарно. Второй `INSERT` ждёт, чем закончится транзакция первого,
и получает ошибку `23505`. Репозиторий распознаёт её по коду и имени индекса и
превращает в `trip.ErrDriverBusy`, а `http/domainerror` отдаёт `409 driver_busy`. После
завершения поездки строка перестаёт попадать в индекс, и водитель может взять
новую.

### Завершение без гонок

Переход описан в домене, но от гонки защищает база. Usecase внутри транзакции читает
поездку, вызывает `Trip.Finish` и сохраняет результат запросом
`UPDATE trips SET status='completed', finished_at=… WHERE id=$1 AND status='active'`.

- Поездки нет — `Fetch` даёт `404 trip_not_found`.
- Поездка уже завершена — `Trip.Finish` даёт `409 trip_completed`, записи в базу нет.
- Два одновременных `finish` оба прочитали `active` и прошли проверку в домене. Второй
  `UPDATE` ждёт блокировку строки, перепроверяет `WHERE` на закоммиченной версии и
  обновляет ноль строк. Репозиторий превращает это в `trip.ErrTripCompleted`, поэтому
  ответы — один `200` и один `409`, а `finished_at` не перезаписывается.

Если убрать условие из `UPDATE`, второй запрос перезапишет `finished_at` и добавит лишнюю
запись в журнал.

### Прочее

- `id` поездки — UUIDv7: он растёт во времени, поэтому вставки в индекс
  первичного ключа идут в конец b-tree.
- Время обрезается до микросекунд (точность `timestamptz`), чтобы ответ на
  создание совпадал с последующим `GET`.
- `/health` в базу не ходит. Это liveness: если бы он зависел от базы, падение
  PostgreSQL привело бы к перезапуску всех подов сервиса, хотя сами они исправны.
  `/ready` делает `Ping` с таймаутом `DATABASE_QUERY_TIMEOUT` и при ошибке
  отвечает `503`, чтобы балансировщик перестал слать трафик.
- Graceful shutdown: по `SIGINT`/`SIGTERM` вызывается `http.Server.Shutdown`
  (новые соединения не принимаются, текущие запросы дорабатывают), затем
  закрывается пул. Если сервис не уложился в `SHUTDOWN_TIMEOUT`, соединения
  закрываются принудительно, и в лог пишется ошибка.

### Идемпотентность `POST /api/v1/trips`

Заголовок необязательный. Без него создание работает как обычно.

| Запрос | Ответ |
|---|---|
| первый с этим ключом | `201`, поездка создана |
| повтор с тем же ключом и тем же телом | `200` и та же поездка, вторая не создаётся |
| тот же ключ, другое тело | `409 idempotency_conflict` |

- **Хранение.** Таблица `idempotency_keys` (миграция `00003`): ключ, sha256-отпечаток
  запроса, `trip_id`, время создания. Отпечаток считается по разобранным
  параметрам, а не по сырому телу, поэтому пробелы и порядок полей в JSON на него
  не влияют.
- **Порядок в транзакции.** Внутри того же `Do`, что и создание поездки, ключ
  занимается **первым**, до вставки в `trips`. Иначе параллельный повтор упал бы
  на индексе активной поездки водителя и получил `409 driver_busy` вместо той же
  поездки. Внешний ключ `trip_id` объявлен `DEFERRABLE INITIALLY DEFERRED` и
  проверяется при `COMMIT`, когда поездка уже вставлена.
- **Конкурентность.** Ключ занимается запросом
  `INSERT ... ON CONFLICT (key) DO UPDATE ... WHERE created_at < срок RETURNING key`.
  Параллельный запрос с тем же ключом ждёт на первичном ключе, пока первая
  транзакция закончится. Потом он видит занятый ключ, сравнивает отпечаток и
  отдаёт ту же поездку (`200`) или `409`.
- **Время жизни — 24 часа** (`IDEMPOTENCY_KEY_TTL`), столько же хранит ключи Stripe.
  После этого тот же ключ можно занять заново: тот же запрос перезаписывает
  просроченную запись. Физически старые строки пока не удаляются, чистку можно
  будет сделать фоновой задачей в ЛР3.
- **Неудачи не запоминаются.** Если создание упало (`driver_busy`, `400`, `500`),
  транзакция откатывается вместе с ключом, и повтор с тем же ключом выполнится
  заново.
- **Повтор отдаёт текущее состояние поездки.** Если её уже завершили, в ответе
  будет `status: completed`.
- **Область видимости ключа.** Ключ глобальный, не привязан к пользователю. Это
  допустимо, потому что клиент генерирует его как UUID.

### Домен и ошибки

Доменные ошибки объявлены в `domain/trip/errors.go`. Ошибка валидации — отдельный тип
`ValidationError` с полями и причинами. HTTP-статусы домен не видит: `switch` по
`errors.Is` и `errors.As` написан один раз, в `http/domainerror.Problem`, а хендлер
передаёт ошибку туда, не разбирая. Неизвестная ошибка становится `500` с общим
текстом. Подробности пишутся в лог и в спан, клиент их не видит. Запись в лог делает
только `domainerror.Write` и только для 5xx, поэтому одна ошибка логируется один раз.

### Телеметрия

- **Провайдеры передаются явно**, глобальных `otel.SetTracerProvider` нет. Единственная
  глобальная настройка — `otel.SetErrorHandler`, у SDK нет другого способа направить
  ошибки экспорта в JSON-лог.
- **Порядок middleware** `Recover → Tracing → Logging → Metrics` задан в `http/router`.
  `Tracing` стоит снаружи `Logging`, поэтому в записи о запросе есть `trace_id`. Спан
  лежит в контексте запроса, а `trace_id` и `span_id` добавляет в запись обёртка над
  `slog.Handler` в `infra/telemetry/logger.go`.
- **`Recover` стоит снаружи спана**, поэтому в его записи о панике нет `trace_id`. Связать
  панику с трейсом можно через запись `request handled` от `Logging`: она сделана внутри спана.
- **Паника в `Logging`, `Metrics` и `Tracing`** учитывается как `500`: запись и метрика
  делаются в `defer`, а `active_requests` не остаётся завышенным.
- **`http.route` — только шаблон.** С `tripId` в лейбле каждая поездка создала бы свою
  временную серию, метрика перестала бы агрегироваться, а Prometheus съел бы память.
  Путь, которого нет в роутере, идёт с пустым `route`.
- **HTTP-метрики написаны руками.** `otelhttp` не знает шаблон маршрута chi и не кладёт
  `http.route` в метрики, поэтому он отвечает только за спан. `active_requests` растёт до
  маршрутизации, поэтому маршрут сопоставляется заранее через `chi.Routes.Match`.
- **Корзины гистограмм заданы явно.** Корзины SDK по умолчанию рассчитаны на миллисекунды,
  а значения у нас в секундах.
- **Запросы в БД инструментированы на уровне пула** через `otelpgx`, репозиторий про
  телеметрию не знает. `otelpgx` не ставит `db.collection.name`, а его
  `db.client.operation.duration` содержит только `pgx.operation.type`. Поэтому
  `infra/postgres/tracer.go` дополняет его спаны и сам пишет гистограмму с лейблами из
  каталога. Метрики самого `otelpgx` отключены, иначе инструменты конфликтовали бы.
- **`error.type` с низкой кардинальностью:** код SQLSTATE или тип ошибки, но не текст.
  `4xx` спан ошибкой не помечает: это ошибка клиента, а не поломка сервиса.
- **Недоступный коллектор** ничего не ломает. Экспортёры подключаются лениво и отправляют
  из фона, запрос их не ждёт. В лог уходит `WARN telemetry export failed`.
- **`tripgo.trips.active`** из каталога телеметрии читает `count(*)` в момент сбора.
  Если база недоступна, точка пропускается: разрыв на графике честнее ложного нуля.
- **Порядок остановки:** HTTP-сервер, затем телеметрия, затем пул. Телеметрия раньше пула,
  потому что финальный сбор метрик читает `tripgo.trips.active` из базы.

### Docker

`deploy/Dockerfile`, собирается командой `make docker-build`.

- Две стадии. `golang:1.27-alpine` собирает статический бинарь
  (`CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`). Итоговый образ —
  `gcr.io/distroless/static-debian12:nonroot`, в нём только бинарь: нет исходников,
  тулчейна, кеша сборки, shell и пакетного менеджера.
- Процесс запускается не от root: `USER nonroot`, uid 65532.
- `.dockerignore` построен по белому списку: в контекст сборки попадают только
  `go.mod`, `go.sum`, `api/`, `cmd/`, `internal/` без `*_test.go`. `.git`, `.env`,
  `.tripgo/`, `course/` и прочее не попадают.
- Кеш модулей и сборки подключается через `--mount=type=cache`: он ускоряет
  пересборку и не попадает в слои образа.
- **Размер итогового образа — 38,1 МБ.**
- `make docker-run` запускает контейнер в сети хоста с переменными из `.env`,
  поэтому он видит PostgreSQL по тому же адресу, что и `make run`.

