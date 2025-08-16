FROM golang:1.23-alpine AS backend-builder
ENV GO111MODULE=on
WORKDIR /app
RUN apk add --no-cache git gcc musl-dev vips-dev libheif-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o imageflow

FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install --frozen-lockfile
COPY frontend/. .
RUN npm run build

FROM alpine:latest AS release
WORKDIR /app
RUN apk add --no-cache \
    ca-certificates \
    vips \
    libheif

RUN mkdir -p /app/static/images/metadata \
    /app/static/images/original/landscape \
    /app/static/images/original/portrait \
    /app/static/images/landscape/webp \
    /app/static/images/landscape/avif \
    /app/static/images/portrait/webp \
    /app/static/images/portrait/avif


COPY --from=backend-builder /app/imageflow /app/
COPY --from=backend-builder /app/config /app/config
COPY --from=frontend-builder /app/frontend/out /app/static
COPY --from=frontend-builder /app/frontend/public/favicon* /app/static/

ENV API_KEY=""
ENV STORAGE_TYPE="local"
ENV LOCAL_STORAGE_PATH="/app/static/images"
ENV S3_ENDPOINT=""
ENV S3_REGION=""
ENV S3_ACCESS_KEY=""
ENV S3_SECRET_KEY=""
ENV S3_BUCKET=""
ENV CUSTOM_DOMAIN=""
ENV MAX_UPLOAD_COUNT="20"
ENV IMAGE_QUALITY="80"
ENV WORKER_THREADS="4"
ENV SPEED="5"
ENV METADATA_STORE_TYPE="redis"
ENV REDIS_HOST="localhost"
ENV REDIS_PORT="6379"
ENV REDIS_PASSWORD=""
ENV REDIS_DB="0"
ENV REDIS_TLS_ENABLED="false"
ENV DEBUG_MODE="false"

EXPOSE 8686
CMD ["./imageflow"]
