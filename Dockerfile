# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26.10

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sdkhandler .

# The handler polls HRPAuth for SDK sources, then compiles the frontend
# template at runtime, so node/npm/git have to exist in the runtime image.
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata gettext nodejs npm git \
 && adduser -D -H -u 10001 -h /app app \
 && mkdir -p /app/state /app/artifacts /app/current /app/frontend-build \
 && chown -R app:app /app
WORKDIR /app
COPY --from=builder /out/sdkhandler /app/sdkhandler
COPY docker-entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh
EXPOSE 2704 2705
USER app
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/app/sdkhandler"]
