FROM golang:1.26.5-alpine AS builder

WORKDIR /src

ENV CGO_ENABLED=0 \
	GOOS=linux \
	GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.google.cn

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /out/taskpilot-api ./cmd/api \
	&& go build -o /out/taskpilot-worker ./cmd/worker

FROM alpine:3.22

WORKDIR /app

RUN addgroup -S taskpilot && adduser -S -G taskpilot taskpilot \
	&& apk add --no-cache ca-certificates tzdata poppler-utils

COPY --from=builder /out/taskpilot-api /app/taskpilot-api
COPY --from=builder /out/taskpilot-worker /app/taskpilot-worker
COPY etc/taskpilot-api.prod.example.yaml /app/etc/taskpilot-api.prod.example.yaml

RUN mkdir -p /app/uploads && chown -R taskpilot:taskpilot /app

USER taskpilot

EXPOSE 8888

CMD ["./taskpilot-api", "-f", "etc/taskpilot-api.prod.yaml"]
