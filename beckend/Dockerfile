FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mixora-api ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata yt-dlp
COPY --from=build /out/mixora-api /usr/local/bin/mixora-api
EXPOSE 8080
USER nobody:nobody
ENTRYPOINT ["/usr/local/bin/mixora-api"]
