# Integration tests

This directory is reserved for tests that require running infrastructure such as PostgreSQL or RabbitMQ.

Guidelines:

- Keep integration tests behind a `//go:build integration` build tag so `go test ./...` remains independent of external services.
- Read connection details from environment variables such as `DATABASE_URL` and `RABBITMQ_URL`.
- Use unique database schemas, queues, or exchanges per test run and clean them up.
- Do not make unit tests depend on this directory.
- Document any required containers or setup in the test file or this README.

No integration tests are implemented during initialization.
