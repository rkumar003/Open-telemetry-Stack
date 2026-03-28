FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o otel-app .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/otel-app /otel-app
EXPOSE 8080
ENTRYPOINT ["/otel-app"]
