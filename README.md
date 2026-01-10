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
- ✅ Аутентификация на relay (опционально)
- ✅ Rate limiting
- ✅ Прогресс-бар со скоростью

## Быстрый старт

### 1. Запуск relay-сервера

```bash
# Docker
docker-compose up -d

# Или локально
make run-relay
```

### 2. Отправка файла

```bash
filedrop -relay your-server:9000 send myfile.zip
```

Получишь код, например: `A1B2C3`

### 3. Получение файла

```bash
filedrop -relay your-server:9000 receive A1B2C3
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
| `-relay` | `localhost:9000` | Адрес relay-сервера |
| `-output` | `.` | Папка для сохранения |
| `-password` | - | Пароль шифрования |
| `-key` | - | Ключ шифрования (при получении) |
| `-compress` | `false` | Включить сжатие |
| `-simple` | `true` | Простой режим (один файл, без шифрования) |

## Self-hosted relay

### Docker

```bash
git clone https://github.com/SettlerNVG/filedrop.git
cd filedrop
docker-compose up -d
```

### Флаги relay-сервера

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `-port` | `9000` | Порт сервера |
| `-auth` | `false` | Требовать аутентификацию |
| `-genkey` | - | Сгенерировать API ключ |

## Архитектура

```
┌──────────────┐                ┌──────────────┐                ┌──────────────┐
│    Sender    │ ──── TCP ────► │    Relay     │ ◄──── TCP ──── │   Receiver   │
│              │   encrypted    │   (Docker)   │   encrypted    │              │
└──────────────┘                └──────────────┘                └──────────────┘
```

- Relay только пробрасывает байты, не видит содержимое (E2E шифрование)
- Chunk size: 64KB для оптимальной скорости

## License

MIT
