# Webhook test receiver

A deterministic HTTPS/HTTP receiver for the real-stack webhook retry tests. It
is started only through the `webhook-test` Compose profile, never by the normal
stack.

- HTTPS `:8098` receives webhook deliveries (the test origin).
- HTTP `:8099` exposes test control.

Control endpoints:

- `POST /control` `{ "statuses": [500, 200], "reset": true }` sets the response
  sequence and optionally clears recorded requests.
- `GET /control` returns the current sequence and all recorded requests,
  including raw bodies and signature headers.
- `POST /reset` restores a 200 response and clears recordings.
- `GET /health` is the container health check.

Every other HTTPS path records the request and answers with the next configured
status. The receiver is reachable from the webhook worker as
`https://webhook-sink:8098` and from tests at `http://localhost:8099`.
