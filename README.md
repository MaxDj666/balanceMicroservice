# Balance Microservice

Простой микросервис на Go для управления балансом пользователей с использованием REST API и PostgreSQL.

![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)

## Быстрый старт

### 1. Клонируйте репозиторий

```bash
git clone https://github.com/MaxDj666/balanceMicroservice
```

### 2. Инициализируйте Go проект

```bash
go mod init balanceMicroservice
go get github.com/lib/pq
go get github.com/joho/godotenv
```

### 3. Создайте файл .env

Скопируйте `.env.example` в `.env` и заполните ваши параметры:

```bash
cp .env.example .env
```

Отредактируйте `.env`:

```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password_here
DB_NAME=balance_service
SERVER_PORT=8080
```

### 4. Создайте базу данных и таблицу

**Способ 1: Через GoLand Database Tools**

1. Откройте GoLand → View → Tool Windows → Database
2. Добавьте новое подключение к PostgreSQL
3. Создайте новую БД `balance_service`
4. Откройте SQL консоль и выполните:

```sql
CREATE TABLE IF NOT EXISTS transactions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    operation_type VARCHAR(50) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**Способ 2: Через psql (командная строка)**

```bash
psql -U postgres -c "CREATE DATABASE balance_service;"

psql -U postgres -d balance_service -c "
CREATE TABLE IF NOT EXISTS transactions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    operation_type VARCHAR(50) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);"
```

### 5. Запустите микросервис

```bash
go run main.go
```

Вы должны увидеть:

```
✓ Database connection established to localhost:5432/balance_service
🚀 Server running on http://localhost:8080
```

## API Endpoints

### 1. Пополнение баланса (Deposit)

**Запрос:**
```http
POST /api/deposit
Content-Type: application/json

{
  "user_id": 1,
  "amount": 100.00
}
```

**Успешный ответ (200 OK):**
```json
{
  "status": "success",
  "message": "Deposited 100.00 to user 1"
}
```

### 2. Списание со счёта (Withdraw)

**Запрос:**
```http
POST /api/withdraw
Content-Type: application/json

{
  "user_id": 1,
  "amount": 50.00
}
```

**Успешный ответ (200 OK):**
```json
{
  "status": "success",
  "message": "Withdrawn 50.00 from user 1"
}
```

## Конфигурация

### Переменные окружения (.env)

| Переменная | Описание | Значение по умолчанию |
|-----------|---------|------------|
| `DB_HOST` | Адрес хоста PostgreSQL | `localhost` |
| `DB_PORT` | Порт PostgreSQL | `5432` |
| `DB_USER` | Пользователь БД | `postgres` |
| `DB_PASSWORD` | Пароль БД | — (обязательна) |
| `DB_NAME` | Имя базы данных | — (обязательна) |
| `SERVER_PORT` | Порт микросервиса | `8080` |

### .env.example

Пример конфигурационного файла для документации:

```env
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password_here
DB_NAME=balance_service

# Server Configuration
SERVER_PORT=8080
```
