FROM golang:1.27-alpine AS build

WORKDIR /src
ENV GOFLAGS="-p=1"
ENV GOMEMLIMIT="1500MiB"
ENV GOGC="50"

COPY go.mod go.sum ./
RUN --mount=type=cache,id=swiftdrop-go-modules,target=/go/pkg/mod go mod download

COPY . .
ARG SERVICE
RUN --mount=type=cache,id=swiftdrop-go-modules,target=/go/pkg/mod \
    --mount=type=cache,id=swiftdrop-go-build,target=/root/.cache/go-build \
    test -n "$SERVICE" \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service ./apps/services/${SERVICE}

FROM alpine:3.22

RUN addgroup -S app && adduser -S -G app app \
    && apk add --no-cache ca-certificates tzdata

COPY --from=build /out/service /usr/local/bin/service

USER app
ENTRYPOINT ["/usr/local/bin/service"]
