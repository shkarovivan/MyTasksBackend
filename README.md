# MyTasksBackend

Go-бэкенд приложения MyTasks: принимает запросы приложения, хранит ключ
LLM-провайдера, строит промпты, обращается к нейросети (ProxyAPI,
OpenAI-совместимый API) и возвращает приложению готовый структурированный JSON.
Ключ провайдера никогда не покидает бэкенд.

```
[ приложение MyTasks ] --X-Api-Key--> [ mytasks-backend :8080 ] --Bearer--> [ ProxyAPI ]
```

## Конфигурация (env)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `PORT` | `8080` | Порт слушания |
| `APP_API_KEY` | — (обязателен) | Общий секрет с приложением (заголовок `X-Api-Key`) |
| `PROVIDER_API_KEY` | — (обязателен) | Ключ ProxyAPI, только на бэкенде |
| `LLM_BASE_URL` | `https://api.proxyapi.ru/openai/v1/chat/completions` | Endpoint chat/completions |
| `DEFAULT_MODEL` | `gpt-5.4-mini` | Модель, если приложение не указало свою |
| `LLM_TIMEOUT_SECS` | `120` | Таймаут запроса к LLM |

## Запуск локально

```bash
cp .env.example .env   # заполнить ключи
export $(grep -v '^#' .env | xargs)
go run .
curl -s localhost:8080/health          # {"status":"ok"}
```

## Docker

```bash
docker build -t mytasks-backend .
docker run -d -p 8080:8080 --env-file .env mytasks-backend
```

## API

Все эндпоинты, кроме `/health`, требуют заголовок `X-Api-Key: <APP_API_KEY>`.

### `GET /health`
→ `{"status":"ok"}`

### `POST /v1/task`
Запрос:
```json
{"text": "купить молоко завтра в 18:00", "model": "gpt-5.4-mini"}
```
`model` необязателен (пустой → `DEFAULT_MODEL`). Ответ — задача:
```json
{"title": "...", "description": "...", "type": "DAILY", "date": "2026-08-29 18:00:00"}
```

### `POST /v1/search`
Запрос (массив задач передаётся как есть из приложения):
```json
{"request": "срочные домашние дела", "tasks": [ ... ], "model": ""}
```
Ответ:
```json
{"answer": "...", "ids": ["123", "456"]}
```

### Ошибки
`401` — нет/неверный `X-Api-Key`; `405` — не POST; `400` — битый JSON или пустой
`text`/`request`; `502` — LLM недоступна / вернула не-JSON (тело ошибки провайдера
передаётся как есть).

## Продакшен

Только за TLS-реверс-прокси (Caddy/nginx): секрет `X-Api-Key` и ключ провайдера
не должны ходить по открытому HTTP.

## Связанные проекты

- Приложение: `../MyTasks` (переключатель «AI Connection Type»: direct ↔ backend)
- Архитектурный образец: `../go-python-agent` (не зависит от него)
