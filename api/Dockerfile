# ---------- Build stage ----------
FROM golang:1.25-alpine AS build
WORKDIR /src

# Faster, sane module caching
COPY go.mod go.sum ./
RUN go mod download

# Bring in the rest
COPY . .

# Allow overriding the main package path; default to ./cmd/api
ARG CMD_PATH=./cmd/payments-api

# Static linux/amd64 binary for Fargate
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/app ${CMD_PATH}

# ---------- Runtime (distroless) ----------
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/app /app/app

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/app"]
