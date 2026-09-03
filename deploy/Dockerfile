FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/tunnelbox ./cmd/tunnelbox

FROM cloudflare/cloudflared:latest AS cloudflared

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -S tunnelbox \
    && adduser -S -G tunnelbox tunnelbox
COPY --from=build /out/tunnelbox /usr/local/bin/tunnelbox
COPY --from=build /src/web/dist /app/web/dist
COPY --from=cloudflared /usr/local/bin/cloudflared /usr/local/bin/cloudflared
RUN mkdir -p /app/data/cloudflared /app/data \
    && chown -R tunnelbox:tunnelbox /app
WORKDIR /app
USER tunnelbox
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tunnelbox"]
