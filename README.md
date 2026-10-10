# legitpayz-transaction-service (legitPayz)

Purchases, payments, history. Owns used_limit updates. (port `8003`). Part of the legitPayz credit-card microservices system.

## Run locally

Prereqs: Go 1.26+, Postgres (`cc/creditcard` migrated with `migrations/001_init.sql`), Redis.

```bash
PORT=8003 go run .
curl localhost:8003/health
```

## Docker

```bash
docker build -t legitpayz-transaction-service .
docker run -p 8003:8003 legitpayz-transaction-service
```

## Env

| Var | Purpose |
|-----|---------|
`DATABASE_URL` Postgres DSN · `REDIS_ADDR` Redis address
