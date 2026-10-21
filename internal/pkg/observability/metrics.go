// Package observability 提供 Prometheus 指标和 HTTP 观测中间件。
package observability

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"

	"gowms/internal/pkg/version"
)

type Metrics struct {
	registry           *prometheus.Registry
	requestsTotal      *prometheus.CounterVec
	requestSeconds     *prometheus.HistogramVec
	inFlight           prometheus.Gauge
	txSeconds          *prometheus.HistogramVec
	txRetries          *prometheus.CounterVec
	importSeconds      *prometheus.HistogramVec
	importRecoveries   prometheus.Counter
	cleanupSeconds     *prometheus.HistogramVec
	cleanupRows        prometheus.Counter
	cleanupBatches     prometheus.Counter
	cleanupLastSuccess prometheus.Gauge
}

func New(db *gorm.DB, serviceName string) *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "wms", Name: "http_requests_total", Help: "Total number of HTTP requests.",
		}, []string{"method", "route", "status"}),
		requestSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "wms", Name: "http_request_duration_seconds", Help: "HTTP request duration in seconds.", Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "wms", Name: "http_requests_in_flight", Help: "Current number of in-flight HTTP requests.",
		}),
	}
	m.txSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "wms_transaction_attempt_duration_seconds", Help: "Business transaction attempt duration, including database lock waits.",
		Buckets: prometheus.DefBuckets,
	}, []string{"outcome"})
	m.txRetries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "wms_transaction_retries_total", Help: "Additional business transaction attempts actually started.",
	}, []string{"reason"})
	m.importSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "wms_import_run_duration_seconds", Help: "Duration of claimed import executions, including resumed executions.",
		Buckets: []float64{.1, .5, 1, 5, 15, 30, 60, 120, 300, 600},
	}, []string{"outcome"})
	m.importRecoveries = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "wms_import_recoveries_total", Help: "Stale import executions successfully returned to pending.",
	})
	m.cleanupSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "wms_idempotency_cleanup_duration_seconds", Help: "Scheduled idempotency cleanup round duration.",
		Buckets: []float64{.01, .1, .5, 1, 2, 5, 10, 30},
	}, []string{"outcome"})
	m.cleanupRows = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "wms_idempotency_cleanup_deleted_rows_total", Help: "Acknowledged deleted rows in scheduled cleanup rounds, including partial progress before errors.",
	})
	m.cleanupBatches = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "wms_idempotency_cleanup_batches_total", Help: "Successfully completed bounded delete statements in scheduled cleanup rounds.",
	})
	m.cleanupLastSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "wms_idempotency_cleanup_last_success_timestamp_seconds", Help: "Last round that completed without reaching the batch cap; zero until first completion in this process.",
	})
	m.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m.registry.MustRegister(m.requestsTotal, m.requestSeconds, m.inFlight)
	m.registry.MustRegister(m.txSeconds, m.txRetries, m.importSeconds, m.importRecoveries)
	m.registry.MustRegister(m.cleanupSeconds, m.cleanupRows, m.cleanupBatches, m.cleanupLastSuccess)
	if db != nil {
		if sqlDB, err := db.DB(); err == nil {
			m.registry.MustRegister(newDBCollector(sqlDB))
			m.registry.MustRegister(newRowLockCollector(sqlDB))
		}
	}
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "wms", Name: "build_info", Help: "Build information for the running service.",
	}, []string{"service", "version", "commit"})
	buildInfo.WithLabelValues(serviceName, version.Version, version.Commit).Set(1)
	m.registry.MustRegister(buildInfo)
	return m
}

func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

// RecordTransaction records a fixed outcome chosen by the transaction manager.
func (m *Metrics) RecordTransaction(outcome string, elapsed time.Duration) {
	if m != nil {
		m.txSeconds.WithLabelValues(outcome).Observe(elapsed.Seconds())
	}
}

func (m *Metrics) RecordTransactionRetry(reason string) {
	if m != nil {
		m.txRetries.WithLabelValues(reason).Inc()
	}
}

func (m *Metrics) RecordImport(outcome string, elapsed time.Duration) {
	if m != nil {
		m.importSeconds.WithLabelValues(outcome).Observe(elapsed.Seconds())
	}
}

func (m *Metrics) RecordImportRecovery() {
	if m != nil {
		m.importRecoveries.Inc()
	}
}

func (m *Metrics) RecordIdempotencyCleanup(outcome string, rows int64, batches int, elapsed time.Duration) {
	if m == nil {
		return
	}
	m.cleanupSeconds.WithLabelValues(outcome).Observe(elapsed.Seconds())
	m.cleanupRows.Add(float64(rows))
	m.cleanupBatches.Add(float64(batches))
	if outcome == "completed" {
		m.cleanupLastSuccess.SetToCurrentTime()
	}
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		m.inFlight.Inc()
		c.Next()
		m.inFlight.Dec()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		labels := []string{c.Request.Method, route, strconv.Itoa(c.Writer.Status())}
		m.requestsTotal.WithLabelValues(labels...).Inc()
		m.requestSeconds.WithLabelValues(labels...).Observe(time.Since(start).Seconds())
	}
}

type dbCollector struct {
	db                                                 *sql.DB
	maxOpen, open, inUse, idle, waitCount, waitSeconds *prometheus.Desc
}

func newDBCollector(db *sql.DB) *dbCollector {
	return &dbCollector{
		db:          db,
		maxOpen:     prometheus.NewDesc("wms_db_max_open_connections", "Maximum number of open database connections.", nil, nil),
		open:        prometheus.NewDesc("wms_db_open_connections", "Number of established database connections.", nil, nil),
		inUse:       prometheus.NewDesc("wms_db_in_use_connections", "Number of database connections currently in use.", nil, nil),
		idle:        prometheus.NewDesc("wms_db_idle_connections", "Number of idle database connections.", nil, nil),
		waitCount:   prometheus.NewDesc("wms_db_wait_count_total", "Total number of database connection waits.", nil, nil),
		waitSeconds: prometheus.NewDesc("wms_db_wait_duration_seconds_total", "Total time blocked waiting for a database connection.", nil, nil),
	}
}

func (c *dbCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.maxOpen
	ch <- c.open
	ch <- c.inUse
	ch <- c.idle
	ch <- c.waitCount
	ch <- c.waitSeconds
}

func (c *dbCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.db.Stats()
	ch <- prometheus.MustNewConstMetric(c.maxOpen, prometheus.GaugeValue, float64(stats.MaxOpenConnections))
	ch <- prometheus.MustNewConstMetric(c.open, prometheus.GaugeValue, float64(stats.OpenConnections))
	ch <- prometheus.MustNewConstMetric(c.inUse, prometheus.GaugeValue, float64(stats.InUse))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(stats.Idle))
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(stats.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.waitSeconds, prometheus.CounterValue, stats.WaitDuration.Seconds())
}
