FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy both api and processor modules
COPY api/ ./api/
COPY processor/ ./processor/

WORKDIR /build/processor

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o payments-processor ./cmd/payments-processor

# Final image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /build/processor/payments-processor .

EXPOSE 8081

CMD ["./payments-processor"]