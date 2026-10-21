package service

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"gowms/internal/modules/inbound/model"
	"gowms/internal/pkg/observability"
)

// SetMetrics is called once during application assembly, before workers start.
func (s *Service) SetMetrics(metrics *observability.Metrics) {
	s.metrics = metrics
	if metrics != nil {
		metrics.Registry().MustRegister(&importQueueCollector{s: s,
			count:   prometheus.NewDesc("wms_import_queue_tasks", "Current import tasks across all tenants, by active status.", []string{"status"}, nil),
			age:     prometheus.NewDesc("wms_import_oldest_pending_age_seconds", "Age since creation of the oldest currently pending import; zero when empty.", nil, nil),
			success: prometheus.NewDesc("wms_import_queue_scrape_success", "Whether the import queue query succeeded.", nil, nil),
		})
	}
}

type importQueueCollector struct {
	s                   *Service
	count, age, success *prometheus.Desc
}

func (c *importQueueCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.count
	ch <- c.age
	ch <- c.success
}

func (c *importQueueCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stats, err := c.s.repo.ImportQueueStats(ctx, c.s.tm.DB())
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 1)
	var oldest time.Time
	counts := make(map[model.ImportTaskStatus]int64, 2)
	for _, stat := range stats {
		counts[stat.Status] = stat.Count
		if stat.Status == model.ImportPending {
			oldest = stat.Oldest
		}
	}
	for _, status := range []model.ImportTaskStatus{model.ImportPending, model.ImportProcessing} {
		ch <- prometheus.MustNewConstMetric(c.count, prometheus.GaugeValue, float64(counts[status]), string(status))
	}
	age := 0.0
	if !oldest.IsZero() {
		age = max(0, time.Since(oldest).Seconds())
	}
	ch <- prometheus.MustNewConstMetric(c.age, prometheus.GaugeValue, age)
}
