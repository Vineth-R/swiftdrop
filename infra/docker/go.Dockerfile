FROM golang:1.27-alpine AS build

ARG SERVICE
WORKDIR /src
ENV GOFLAGS="-p=1"
ENV GOMEMLIMIT="1500MiB"
ENV GOGC="50"

COPY go.mod go.sum ./
RUN go mod download

COPY packages ./packages
COPY apps/services/${SERVICE} ./apps/services/${SERVICE}

RUN test -n "$SERVICE" \
    && mkdir -p /out \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service ./apps/services/${SERVICE}

FROM alpine:3.24

RUN addgroup -S app && adduser -S -G app app \
    && apk add --no-cache ca-certificates tzdata

COPY --from=build /out/service /usr/local/bin/service

USER app
ENTRYPOINT ["/usr/local/bin/service"]
