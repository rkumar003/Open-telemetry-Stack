FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod tidy
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o otel-app .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/otel-app /otel-app
EXPOSE 8080
ENTRYPOINT ["/otel-app"]
