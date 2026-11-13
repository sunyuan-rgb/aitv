# 题目04 短剧平台服务镜像（多阶段构建）
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /aitv-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache curl
WORKDIR /app
COPY --from=builder /aitv-server ./aitv-server
COPY web/templates ./web/templates
COPY web/static ./web/static
ENV AITV_ADDR=:8080 AITV_DB=/app/data/aitv.db
VOLUME ["/app/data"]
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD curl -sf http://localhost:8080/login > /dev/null || exit 1
ENTRYPOINT ["./aitv-server"]
CMD ["-addr", ":8080", "-db", "/app/data/aitv.db"]
