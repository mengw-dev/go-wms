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
	registry       *prometheus.Registry
	requestsTotal  *prometheus.CounterVec
	requestSeconds *prometheus.HistogramVec
	inFlight       prometheus.Gauge
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
	m.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m.registry.MustRegister(m.requestsTotal, m.requestSeconds, m.inFlight)
	if db != nil {
		if sqlDB, err := db.DB(); err == nil {
			m.registry.MustRegister(newDBCollector(sqlDB))
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
