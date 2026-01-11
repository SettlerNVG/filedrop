# FileDrop

P2P файловый обмен через relay-сервер. Работает между любыми ОС (Linux, macOS, Windows).

## Установка

```bash
curl -fsSL https://raw.githubusercontent.com/SettlerNVG/filedrop/main/install.sh | bash
```

Или скачай бинарник из [Releases](https://github.com/SettlerNVG/filedrop/releases).

## Возможности

- ✅ Передача файлов любого размера (потоковая, не грузит RAM)
- ✅ Передача папок с сохранением структуры
- ✅ Шифрование AES-256-GCM (end-to-end)
- ✅ Защита паролем или автогенерация ключа
- ✅ Сжатие данных (gzip)
- ✅ Докачка при обрыве соединения
- ✅ Rate limiting
- ✅ Прогресс-бар со скоростью
- ✅ **TUI интерфейс** — видеть онлайн пользователей и передавать файлы напрямую

## Быстрый старт

По умолчанию клиент подключается к публичному relay-серверу `5.18.196.229:9000`.

### TUI режим (рекомендуется)

```bash
filedrop-tui
```

В TUI вы можете:
- Видеть всех онлайн пользователей
- Выбрать пользователя и отправить ему файл
- Принять входящие передачи

### CLI режим — Отправка файла

```bash
filedrop send myfile.zip
```

Получишь код, например: `A1B2C3`

### CLI режим — Получение файла

```bash
filedrop receive A1B2C3
```

## Использование своего relay

Можно указать свой relay-сервер:

```bash
# Через флаг
filedrop -relay myserver:9000 send file.zip
filedrop-tui -relay myserver:9000

# Через переменную окружения
export FILEDROP_RELAY=myserver:9000
filedrop send file.zip

# Через конфиг файл
echo "myserver:9000" > ~/.filedrop/relay
```

## Расширенное использование

### Передача с шифрованием по паролю

```bash
# Отправитель
filedrop -simple=false -password=mysecret send ./folder

# Получатель (тот же пароль)
filedrop -simple=false -password=mysecret receive ABC123
```

### Передача папки со сжатием

```bash
filedrop -simple=false -compress send ./my-folder
```

## Флаги клиента

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `-relay` | `5.18.196.229:9000` | Адрес relay-сервера |
| `-output` | `.` | Папка для сохранения |
| `-password` | - | Пароль шифрования |
| `-compress` | `false` | Включить сжатие |
| `-simple` | `true` | Простой режим (один файл, без шифрования) |

## Self-hosted relay

### Запуск

```bash
git clone https://github.com/SettlerNVG/filedrop.git
cd filedrop
make run-relay
```

При запуске relay покажет ваш публичный IP для подключения.

### Docker

```bash
docker-compose up -d
```

### Флаги relay-сервера

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `-port` | `9000` | Порт сервера |
| `-auth` | `false` | Требовать аутентификацию |

## Архитектура

```
┌──────────────┐                ┌──────────────┐                ┌──────────────┐
│    Sender    │ ──── TCP ────► │    Relay     │ ◄──── TCP ──── │   Receiver   │
│              │   encrypted    │  5.18.196.229│   encrypted    │              │
└──────────────┘                └──────────────┘                └──────────────┘
```

- Relay только пробрасывает байты, не видит содержимое (E2E шифрование)
- Chunk size: 64KB для оптимальной скорости

## TUI Интерфейс

```
📁 FileDrop TUI
User: Alice | Relay: 5.18.196.229:9000

┌─────────────────────────────────┐
│ Main Menu                       │
│                                 │
│ [1/u] Browse online users       │
│ [2/p] Check pending transfers   │
│ [r]   Refresh                   │
│ [q]   Quit                      │
│                                 │
│ Online users: 3                 │
└─────────────────────────────────┘
```

Клавиши:
- `1` или `u` — список онлайн пользователей
- `2` или `p` — входящие запросы на передачу
- `r` — обновить
- `Enter` — выбрать пользователя/файл
- `Esc` — назад
- `q` — выход

## License

MIT
