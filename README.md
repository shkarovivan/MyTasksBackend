# MyTasksBackend

Go-бэкенд приложения MyTasks. Три роли в одном сервисе:

1. **LLM-прокси** — хранит ключ провайдера (ProxyAPI, OpenAI-совместимый API),
   строит промпты, обращается к нейросети и возвращает приложению готовый
   структурированный JSON. Ключ провайдера никогда не покидает бэкенд.
2. **Хранилище задач** — PostgreSQL: синхронизация устройств одного
   Google-аккаунта, восстановление данных после удаления приложения.
3. **Авторизация** — проверка Google ID-токенов (`sub` — ключ пользователя).

```
                    https (Let's Encrypt)
[ приложение MyTasks ] ─────────────────────► [ caddy :443→8443 ]
        │                                            │
        │ X-Api-Key (LLM-эндпоинты)                  ▼
        │ Bearer Google ID-token (sync)      [ mytasks-backend :8080 ]
        │                                     │                │
        │                                     ▼                ▼
        └────────────────────────────► [ PostgreSQL ]     [ ProxyAPI ]
```

## Состав (docker compose)

| Сервис | Образ | Роль |
|---|---|---|
| `db` | postgres:17-alpine | данные задач, volume `pgdata`, healthcheck |
| `backend` | сборка из Dockerfile | Go 1.25 + pgx (единственная внешняя зависимость) |
| `caddy` | caddy:2-alpine | TLS-терминация, сертификат Let's Encrypt по TLS-ALPN, автопродление |

## Конфигурация (env)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `APP_API_KEY` | — (обязателен) | Общий секрет с приложением (`X-Api-Key`) |
| `PROVIDER_API_KEY` | — (обязателен) | Ключ ProxyAPI — только на бэкенде |
| `DATABASE_URL` | — (обязателен) | `postgres://user:pass@db:5432/mytasks?sslmode=disable` |
| `GOOGLE_CLIENT_ID` | — (обязателен) | Web OAuth client ID; проверяется как `aud` Google ID-токена |
| `DOMAIN` | — | Имя для Caddy (сертификат + виртуальный хост) |
| `PORT` | `8080` | Порт слушания (внутри контейнера) |
| `BACKEND_PORT` | `8080` | Хост-порт бэкенда |
| `LLM_BASE_URL` | `https://api.proxyapi.ru/.../chat/completions` | Endpoint chat/completions |
| `DEFAULT_MODEL` | `gpt-5.4-mini` | Модель, если приложение не указало свою |
| `LLM_TIMEOUT_SECS` | `120` | Таймаут запроса к LLM |
| `ACCEPT_DEV_TOKENS` | — | `1` = принимать заглушки `dev:<user_id>` (только локальные тесты) |

## Запуск

```bash
cp .env.example .env   # заполнить ключи
docker compose up -d --build
curl -s https://<домен>/health      # {"status":"ok"}
```

Локально без Docker (потребуется Postgres):

```bash
export $(grep -v '^#' .env | xargs)
go run .
go test .                             # юнит-тесты без БД
```

## API

### `GET /health` — без авторизации
→ `{"status":"ok"}`

### `POST /v1/task` — заголовок `X-Api-Key`
```json
{"text": "купить молоко завтра в 18:00", "model": ""}
```
→ `{"title": "...", "description": "...", "type": "DAILY", "date": "..."}` —
задача, собранная нейросетью.

### `POST /v1/search` — `X-Api-Key`
```json
{"request": "срочные домашние дела", "tasks": [ ... ], "model": ""}
```
→ `{"answer": "...", "ids": ["123", "456"]}`. Массив `tasks` вставляется в
промпт байт-в-байт (кейсинг енумов Gson: `"STARTED"`, `"WORK"`).

### `GET /v1/auth/whoami` — `Authorization: Bearer <Google ID-token>`
→ `{"user_id": "<Google sub>", "email": "..."}`

### `POST /v1/sync/push` — Bearer
```json
{"tasks": [ {"id","created","title","description","type","deadLine","deadLineMs",
             "status","work","updatedAt","deleted"}, ... ]}
```
→ `{"applied": N}`. До 500 задач за запрос. Конфликты решаются на сервере:
строка перезаписывается только при строго большем `updatedAt` (last-write-wins).

### `GET /v1/sync/pull?since=<ms>&limit=<n>` — Bearer
→ `{"tasks": [...], "cursor": <ms>}` — изменения по возрастанию `updatedAt`,
включая tombstones (`deleted:true`); `cursor` передаётся следующим запросом
как `since`.

### Ошибки
`400` — битый JSON/параметры; `401` — нет/неверный `X-Api-Key` или Google-токен;
`405` — не POST; `502` — LLM недоступна/не-JSON (тело ошибки провайдера как есть).

## Деплой за домашним роутером (проверено на практике)

- Домен: бесплатный DDNS (например DuckDNS) со статическим публичным IP.
- Сертификат — только по **TLS-ALPN** через порт 443: если провайдер блокирует
  входящий 80 (частая практика на домашних тарифах), HTTP-01 невозможен,
  Caddy решает это из коробки.
- Caddy публикуется на хост-порту **8443**, роутер пробрасывает
  «внешний 443 → внутренний 8443». На части бытовых роутеров симметричный
  проброс 443→443 не работает, а **отредактированные правила могут молча не
  применяться** — после каждой правки проверяйте порт снаружи
  (check-host.net) и пересоздавайте правило с нуля при проблемах.
- Единственный открытый наружу порт — 443. Обновление: `git pull` +
  `docker compose up -d --build` (compose сам пересоберёт и заменит контейнеры).

## Связанные проекты

- Приложение: `../MyTasks` (режимы AI: direct ↔ backend; синхронизация задач)
- Архитектурный образец: `../go-python-agent` (не зависит от него)
