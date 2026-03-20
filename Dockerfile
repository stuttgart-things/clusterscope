# Build stage
FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /clusterscope ./cmd/clusterscope

# Runtime stage: debian-slim with Chromium for headless PDF rendering
FROM debian:bookworm-slim

# Install Chromium + dependencies for headless PDF generation
RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    chromium-sandbox \
    fonts-liberation \
    fonts-dejavu-core \
    libglib2.0-0 \
    libnss3 \
    libatk-bridge2.0-0 \
    libx11-6 \
    libxcomposite1 \
    libxdamage1 \
    libxext6 \
    libxfixes3 \
    libxrandr2 \
    libgbm1 \
    libasound2 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# chromedp looks for the browser via CHROME_PATH or well-known locations
ENV CHROME_PATH=/usr/bin/chromium

COPY --from=builder /clusterscope /usr/local/bin/clusterscope

USER nobody
EXPOSE 8080
ENTRYPOINT ["clusterscope"]
