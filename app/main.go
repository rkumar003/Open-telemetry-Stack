package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const serviceName = "otel-demo-app"

func getOtelEndpoint() string {
	ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if ep == "" {
		ep = "localhost:4317"
	}
	return ep
}

// initResource builds the OTel resource with service metadata.
func initResource() (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
			attribute.String("environment", os.Getenv("APP_ENV")),
		),
	)
}

// initMetrics sets up the OTLP gRPC metric exporter and MeterProvider.
func initMetrics(ctx context.Context, res *resource.Resource, conn *grpc.ClientConn) (*sdkmetric.MeterProvider, error) {
	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("creating OTLP metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(10*time.Second)),
		),
	)
	otel.SetMeterProvider(mp)
	return mp, nil
}

// initLogs sets up the OTLP gRPC log exporter and LoggerProvider.
func initLogs(ctx context.Context, res *resource.Resource, conn *grpc.ClientConn) (*sdklog.LoggerProvider, error) {
	exporter, err := otlploggrpc.New(ctx, otlploggrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("creating OTLP log exporter: %w", err)
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)
	global.SetLoggerProvider(lp)
	return lp, nil
}

// appLogger wraps the OTel logger for structured logging.
type appLogger struct {
	logger otellog.Logger
}

func newAppLogger() *appLogger {
	return &appLogger{
		logger: global.GetLoggerProvider().Logger(serviceName),
	}
}

func (l *appLogger) Info(ctx context.Context, msg string, attrs ...otellog.KeyValue) {
	var r otellog.Record
	r.SetTimestamp(time.Now())
	r.SetSeverity(otellog.SeverityInfo)
	r.SetBody(otellog.StringValue(msg))
	r.AddAttributes(attrs...)
	l.logger.Emit(ctx, r)
	log.Printf("[INFO] %s", msg)
}

func (l *appLogger) Error(ctx context.Context, msg string, err error, attrs ...otellog.KeyValue) {
	var r otellog.Record
	r.SetTimestamp(time.Now())
	r.SetSeverity(otellog.SeverityError)
	r.SetBody(otellog.StringValue(msg))
	r.AddAttributes(append(attrs, otellog.String("error", err.Error()))...)
	l.logger.Emit(ctx, r)
	log.Printf("[ERROR] %s: %v", msg, err)
}

// httpHandler simulates realistic traffic and emits metrics + logs.
type httpHandler struct {
	logger          *appLogger
	requestCounter  metric.Int64Counter
	requestDuration metric.Float64Histogram
	activeRequests  metric.Int64UpDownCounter
	errorCounter    metric.Int64Counter
}

func newHTTPHandler(meter metric.Meter, logger *appLogger) (*httpHandler, error) {
	reqCounter, err := meter.Int64Counter("http.server.request.total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	reqDuration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithDescription("HTTP request duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	activeReqs, err := meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithDescription("Number of in-flight HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	errCounter, err := meter.Int64Counter("http.server.errors.total",
		metric.WithDescription("Total number of HTTP errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}

	return &httpHandler{
		logger:          logger,
		requestCounter:  reqCounter,
		requestDuration: reqDuration,
		activeRequests:  activeReqs,
		errorCounter:    errCounter,
	}, nil
}

var routes = []string{"/api/video/stream", "/api/video/manifest", "/api/user/profile", "/api/health", "/api/metrics/ingest"}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	// Simulate a random route for load generation
	route := routes[rand.Intn(len(routes))]
	statusCode := 200
	isError := rand.Float64() < 0.05 // 5% error rate

	attrs := []attribute.KeyValue{
		attribute.String("http.method", r.Method),
		attribute.String("http.route", route),
	}

	h.activeRequests.Add(ctx, 1, metric.WithAttributes(attrs...))
	defer h.activeRequests.Add(ctx, -1, metric.WithAttributes(attrs...))

	// Simulate processing latency (10–400ms)
	latency := 10 + rand.Float64()*390
	time.Sleep(time.Duration(latency) * time.Millisecond)

	if isError {
		statusCode = 500
		h.errorCounter.Add(ctx, 1, metric.WithAttributes(
			append(attrs, attribute.Int("http.status_code", statusCode))...,
		))
		h.logger.Error(ctx, "request failed",
			fmt.Errorf("internal server error"),
			otellog.String("http.route", route),
			otellog.Int("http.status_code", statusCode),
		)
	} else {
		h.logger.Info(ctx, "request completed",
			otellog.String("http.route", route),
			otellog.Int("http.status_code", statusCode),
			otellog.Float64("duration_ms", latency),
		)
	}

	finalAttrs := append(attrs, attribute.Int("http.status_code", statusCode))
	h.requestCounter.Add(ctx, 1, metric.WithAttributes(finalAttrs...))
	h.requestDuration.Record(ctx, time.Since(start).Seconds()*1000, metric.WithAttributes(finalAttrs...))

	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `{"status":%d,"route":"%s","latency_ms":%.2f}`, statusCode, route, latency)
}

// loadGenerator simulates background traffic.
func loadGenerator(ctx context.Context, addr string, logger *appLogger) {
	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			go func() {
				resp, err := client.Get("http://" + addr + "/")
				if err != nil {
					logger.Error(ctx, "load generator request failed", err)
					return
				}
				resp.Body.Close()
			}()
		}
	}
}

// systemMetrics emits simulated system-level gauges.
func systemMetrics(ctx context.Context, meter metric.Meter, logger *appLogger) {
	cpuGauge, _ := meter.Float64ObservableGauge("system.cpu.utilization",
		metric.WithDescription("Simulated CPU utilization (0–1)"),
		metric.WithUnit("1"),
	)
	memGauge, _ := meter.Float64ObservableGauge("system.memory.utilization",
		metric.WithDescription("Simulated memory utilization (0–1)"),
		metric.WithUnit("1"),
	)

	meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		cpu := 0.2 + rand.Float64()*0.6
		mem := 0.3 + rand.Float64()*0.4
		o.ObserveFloat64(cpuGauge, cpu, metric.WithAttributes(attribute.String("cpu", "total")))
		o.ObserveFloat64(memGauge, mem, metric.WithAttributes(attribute.String("mem", "heap")))
		logger.Info(ctx, "system metrics sampled",
			otellog.Float64("cpu_utilization", cpu),
			otellog.Float64("mem_utilization", mem),
		)
		return nil
	}, cpuGauge, memGauge)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	endpoint := getOtelEndpoint()
	log.Printf("Connecting to OTel Collector at %s", endpoint)

	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("Failed to connect to OTel Collector: %v", err)
	}
	defer conn.Close()

	res, err := initResource()
	if err != nil {
		log.Fatalf("Failed to create OTel resource: %v", err)
	}

	mp, err := initMetrics(ctx, res, conn)
	if err != nil {
		log.Fatalf("Failed to init metrics: %v", err)
	}
	defer func() {
		if err := mp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down MeterProvider: %v", err)
		}
	}()

	lp, err := initLogs(ctx, res, conn)
	if err != nil {
		log.Fatalf("Failed to init logs: %v", err)
	}
	defer func() {
		if err := lp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down LoggerProvider: %v", err)
		}
	}()

	logger := newAppLogger()
	meter := otel.Meter(serviceName)

	handler, err := newHTTPHandler(meter, logger)
	if err != nil {
		log.Fatalf("Failed to init metrics instruments: %v", err)
	}

	systemMetrics(ctx, meter, logger)

	addr := "0.0.0.0:8080"
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go loadGenerator(ctx, "localhost:8080", logger)

	go func() {
		logger.Info(ctx, "HTTP server starting", otellog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Info(context.Background(), "shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
}
