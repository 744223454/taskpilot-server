FROM golang:1.26.5-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/taskpilot ./cmd/api

FROM alpine:3.22

WORKDIR /app

RUN addgroup -S taskpilot && adduser -S -G taskpilot taskpilot \
	&& apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/taskpilot /app/taskpilot
COPY etc/taskpilot-api.prod.example.yaml /app/etc/taskpilot-api.prod.example.yaml

RUN mkdir -p /app/uploads && chown -R taskpilot:taskpilot /app

USER taskpilot

EXPOSE 8888

CMD ["./taskpilot", "-f", "etc/taskpilot-api.prod.yaml"]
