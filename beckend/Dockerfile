FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mixora-api ./cmd/server

FROM alpine:3.22
ARG YT_DLP_VERSION=2026.8.19
RUN apk add --no-cache ca-certificates nodejs py3-pip python3 tzdata \
    && python3 -m venv /opt/yt-dlp \
    && /opt/yt-dlp/bin/pip install --no-cache-dir "yt-dlp[default]==${YT_DLP_VERSION}" \
    && ln -s /opt/yt-dlp/bin/yt-dlp /usr/local/bin/yt-dlp
COPY --from=build /out/mixora-api /usr/local/bin/mixora-api
EXPOSE 8080
USER nobody:nobody
ENTRYPOINT ["/usr/local/bin/mixora-api"]
