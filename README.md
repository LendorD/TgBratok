<div align="center">

# 🤖 Bratok

### Telegram-бот с ролями и памятью диалога на Go

Бот общается с нейросетью через **OpenRouter** в выбранной вами роли
и **помнит контекст беседы**. Написан по принципам **чистой архитектуры**.

<p>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white">
  <img alt="Telegram" src="https://img.shields.io/badge/Telegram-Bot%20API-26A5E4?logo=telegram&logoColor=white">
  <img alt="OpenRouter" src="https://img.shields.io/badge/AI-OpenRouter-8A2BE2">
  <img alt="Architecture" src="https://img.shields.io/badge/architecture-clean-2ea44f">
  <img alt="Docker" src="https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white">
</p>

</div>

---

## 📑 Содержание

- [Что умеет](#-что-умеет)
- [Как это выглядит](#-как-это-выглядит)
- [Архитектура](#-архитектура)
- [Структура проекта](#-структура-проекта)
- [Конфигурация](#-конфигурация)
- [Запуск локально](#-запуск-локально)
- [Запуск в Docker](#-запуск-в-docker)
- [Прокси (если Telegram недоступен)](#-прокси-если-telegram-недоступен)
- [Как сменить модель или роли](#-как-сменить-модель-или-роли)
- [Тесты](#-тесты)
- [Технологии](#-технологии)

---

## ✨ Что умеет

- 🚀 **`/start`** — приветствие и предложение выбрать роль.
- 🎭 **`/role`** — список готовых ролей (**Психолог**, **Мотиватор**, **Программист**, **Друг**) и подсказка, как задать свою.
- 📝 **Ввод роли текстом** → она становится системным промптом нейросети.
- 🧠 **Память диалога** — каждое сообщение уходит в OpenRouter вместе с историей беседы (системный промпт + последние `HISTORY_LIMIT` сообщений). Ответ сохраняется в историю.
- 🔄 **Смена роли очищает историю** — начинается новый, чистый контекст.
- ⚡ **Потокобезопасное in-memory хранилище** — состояние (роль + история) живёт в памяти процесса.
- 🌐 **Поддержка прокси** (HTTP/HTTPS/SOCKS5) на случай блокировок.

> Команды `/start` и `/role` в историю диалога не попадают.

---

## 💬 Как это выглядит

```text
Пользователь: /start
Бот: Привет! Я бот, который общается с нейросетью в выбранной тобой роли.
     Сначала выбери роль командой /role ...

Пользователь: /role
Бот: Выбери роль — просто отправь её название следующим сообщением:
     • Психолог
     • Мотиватор
     • Программист
     • Друг
     Или придумай свою ...

Пользователь: Программист
Бот: Готово! Теперь я — «Программист». История диалога очищена ...

Пользователь: Как развернуть слайс в Go?
Бот: Можно пройти его с двух концов и менять местами элементы ... (пример кода)

Пользователь: А без изменения исходного?
Бот: (помнит, что речь о слайсах в Go) Тогда скопируй слайс и разверни копию ...
```

---

## 🏛 Архитектура

Зависимости направлены строго **внутрь**: внешние слои зависят от внутренних, но не наоборот.
Бизнес-логика не знает ни про Telegram, ни про HTTP, ни про хранилище — всё подключено через интерфейсы.

```
        ┌───────────────────────────────────────────────────┐
        │                  cmd/bot + app                      │  точка входа и сборка (DI)
        └───────────────────────────────────────────────────┘
                          │ создаёт и связывает
        ┌─────────────────┴───────────────────────────────────┐
        │                    adapters                           │  внешний мир
        │   handlers  ·  integrations/openrouter  ·            │
        │              repositories/memory                     │
        └─────────────────┬───────────────────────────────────┘
                          │ реализуют интерфейсы
        ┌─────────────────┴───────────────────────────────────┐
        │                   interfaces                          │  интерфейсы ядра
        │  UserRepository · ChatHistoryRepository · AIClient    │
        │                · ChatUsecase                          │
        └─────────────────┬───────────────────────────────────┘
                          │ использует
        ┌─────────────────┴───────────────────────────────────┐
        │                   usecases                            │  вся бизнес-логика
        │                   ChatUseCase                         │
        └─────────────────┬───────────────────────────────────┘
                          │ оперирует
        ┌─────────────────┴───────────────────────────────────┐
        │                domain/entities                        │  сущности, без зависимостей
        │               User · Role · Message                   │
        └───────────────────────────────────────────────────────┘
```

**Поток обработки сообщения:**

1. `handlers` (Telegram) получает обновление и вызывает метод через интерфейс `ChatUsecase`.
2. Для текста `HandleMessage` решает: это ввод роли (если роль ещё не задана или только что вызван `/role`) или обычное сообщение.
3. Для обычного сообщения собирается массив `[system: роль] + [последние HISTORY_LIMIT сообщений] + [текущее]`.
4. Массив уходит в `openrouter`, ответ возвращается пользователю и вместе с его сообщением сохраняется в `memory`.

---

## 📂 Структура проекта

```
.
├── cmd/
│   └── bot/
│       └── main.go                      # тонкая точка входа: вызывает app.Run()
├── internal/
│   ├── app/
│   │   └── app.go                       # composition root: конфиг, DI, graceful shutdown
│   ├── config/
│   │   ├── config.go                    # чтение и валидация конфигурации из env
│   │   └── dotenv.go                    # подгрузка .env для локального запуска
│   ├── domain/
│   │   └── entities/                    # сущности без зависимостей
│   │       ├── entities.go              #   User, Message
│   │       └── roles.go                 #   Role, готовые роли, ResolveRolePrompt
│   ├── interfaces/                      # интерфейсы (порты) ядра
│   │   ├── repository.go                #   UserRepository, ChatHistoryRepository
│   │   ├── integration.go               #   AIClient
│   │   └── usecase.go                   #   ChatUsecase
│   ├── usecases/
│   │   ├── chat.go                      # ChatUseCase: Start, RequestRole, HandleMessage, ...
│   │   └── chat_test.go                 # unit-тесты на моках (testify/mock)
│   └── adapters/
│       ├── handlers/
│       │   └── telegram.go              # приём обновлений Telegram
│       ├── integrations/
│       │   └── openrouter/
│       │       └── openrouter.go        # реализация AIClient (OpenRouter)
│       └── repositories/
│           └── memory/
│               └── memory.go            # in-memory репозитории (sync.RWMutex)
├── Dockerfile                           # многоэтапная сборка
├── docker-compose.yml
├── .env.example
├── go.mod
└── README.md
```

---

## ⚙️ Конфигурация

Все настройки читаются из переменных окружения (см. [`.env.example`](.env.example)):

| Переменная             | Обяз. | По умолчанию                                       | Описание                                          |
|------------------------|:-----:|---------------------------------------------------|---------------------------------------------------|
| `TELEGRAM_BOT_TOKEN`   |  ✅   | —                                                 | Токен бота от [@BotFather](https://t.me/BotFather) |
| `OPENROUTER_API_KEY`   |  ✅   | —                                                 | Ключ [OpenRouter](https://openrouter.ai/keys)     |
| `OPENROUTER_MODEL`     |  ❌   | `openai/gpt-4o-mini`                               | Модель (slug из OpenRouter)                        |
| `OPENROUTER_URL`       |  ❌   | `https://openrouter.ai/api/v1/chat/completions`   | Эндпоинт chat-completions                          |
| `OPENROUTER_REFERER`   |  ❌   | —                                                 | Заголовок `HTTP-Referer` (атрибуция)              |
| `OPENROUTER_TITLE`     |  ❌   | `Bratok Bot`                                       | Заголовок `X-Title` (атрибуция)                   |
| `PROXY_URL`            |  ❌   | —                                                 | Прокси `http`/`https`/`socks5` для Telegram и OpenRouter |
| `HISTORY_LIMIT`        |  ❌   | `20`                                              | Сколько последних сообщений держать в контексте   |
| `REQUEST_TIMEOUT`      |  ❌   | `60s`                                             | Таймаут запроса к OpenRouter (Go duration)        |
| `LOG_LEVEL`            |  ❌   | `info`                                            | `debug` / `info` / `warn` / `error`               |

---

## 🚀 Запуск локально

Требуется **Go 1.22+**.

```bash
# 1. Подготовить конфиг
cp .env.example .env
#   и вписать TELEGRAM_BOT_TOKEN и OPENROUTER_API_KEY

# 2. Скачать зависимости (создаст go.sum)
go mod tidy

# 3. Прогнать тесты (по желанию)
go test ./...

# 4. Запустить
go run ./cmd/bot
```

> 💡 При запуске бот **сам подхватывает файл `.env`** из текущей папки (если он есть).
> Реальные переменные окружения имеют приоритет над `.env`. Задать переменные вручную тоже можно:
>
> - **PowerShell:** `$env:TELEGRAM_BOT_TOKEN="..."; $env:OPENROUTER_API_KEY="..."`
> - **bash:** `export $(grep -v '^#' .env | xargs)`

---

## 🐳 Запуск в Docker

```bash
cp .env.example .env   # заполнить токены

docker compose up --build
```

Бот работает только на исходящих запросах (long polling), поэтому пробрасывать порты не нужно. Логи:

```bash
docker compose logs -f bot
```

---

## 🌐 Прокси (если Telegram недоступен)

Если при запуске видите ошибку вида
`dial tcp api.telegram.org ... connectex: A connection attempt failed` —
значит прямой доступ к Bot API (или OpenRouter) заблокирован. Решение — указать прокси:

```dotenv
# в .env
PROXY_URL=socks5://127.0.0.1:1080
# или
PROXY_URL=http://user:pass@host:port
```

Через этот прокси пойдут запросы и к Telegram, и к OpenRouter. Поддерживаются схемы
`http`, `https`, `socks5`. Альтернативно можно задать системные `HTTPS_PROXY`/`HTTP_PROXY` —
бот их тоже учитывает.

---

## 🔧 Как сменить модель или роли

**Модель** — поменяйте `OPENROUTER_MODEL` в `.env` (например, `anthropic/claude-3.5-sonnet`
или `google/gemini-flash-1.5`) и перезапустите бота. Список моделей:
<https://openrouter.ai/models>.

**Готовые роли** — отредактируйте срез `PredefinedRoles` в
[`internal/domain/entities/roles.go`](internal/domain/entities/roles.go): добавьте/измените `Name` и `Prompt`.
Меню `/role` строится из этого среза автоматически.

**Своя роль на лету** — вызовите `/role` и вместо названия отправьте произвольное описание,
например: `Ты — строгий редактор, который сокращает тексты`.

**Глубина истории** — переменная `HISTORY_LIMIT`.

---

## 🧪 Тесты

Unit-тесты покрывают слой use case с моками всех интерфейсов (`testify/mock`):

```bash
go test ./...
```

Проверяется: сборка контекста запроса (system + история + текущее сообщение),
сохранение обоих сообщений, очистка истории при смене роли, маршрутизация ввода
(роль vs сообщение) и обработка ошибок AI-клиента.

---

## 🛠 Технологии

- **Go 1.22+**, стандартный `log/slog` (структурированное логирование).
- **Telegram:** [`go-telegram-bot-api/v5`](https://github.com/go-telegram-bot-api/telegram-bot-api) — зрелая библиотека без внешних зависимостей.
- **AI:** [OpenRouter](https://openrouter.ai) chat-completions API.
- **Тесты:** [`testify`](https://github.com/stretchr/testify).
- **Graceful shutdown** по `SIGINT` / `SIGTERM`.

<div align="center">

---

Сделано с ❤️ на Go

</div>
