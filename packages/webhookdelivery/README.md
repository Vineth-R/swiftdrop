# webhookdelivery

Shared domain package for SwiftDrop merchant webhook delivery.

## Why this package is outside `go-common`

`packages/go-common` is reserved for infrastructure. This package contains
SwiftDrop's merchant webhook contract and delivery rules:

- the public `parcel.status_changed` event body (`Event`, `EventData`);
- the API v1.1 test event (`SampleEvent`);
- the HMAC-SHA256 signature and timestamp/replay contract;
- the six-attempt retry schedule; and
- the hardened HTTP sender (SSRF checks, TLS, redirect refusal and timeout).

It therefore lives in its own shared domain package instead of
`packages/go-common`. It imports only `packages/go-common/webhookurl` for the
SSRF address checks; it has no database, RabbitMQ or service-runtime
dependencies.

The webhook worker and the merchant test-send endpoint can import this package
without importing another service's `internal/` code.

## Compatibility

This is a mechanical move of the existing worker delivery code. Signing,
verification, retry timing, serialization, SSRF protection, TLS, redirect and
timeout behavior are unchanged. The package name is `webhookdelivery`.
