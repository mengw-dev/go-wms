package observability

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MySQL global InnoDB statistics describe the whole server, not this process or tenant.
// Query failure omits data series and emits scrape_success=0 instead of a false zero.
type rowLockCollector struct {
	db                               *sql.DB
	current, waits, seconds, success *prometheus.Desc
}

func newRowLockCollector(db *sql.DB) *rowLockCollector {
	return &rowLockCollector{db: db,
		current: prometheus.NewDesc("wms_mysql_row_lock_current_waits", "Current InnoDB row lock waits on the whole MySQL server.", nil, nil),
		waits:   prometheus.NewDesc("wms_mysql_row_lock_waits_total", "Cumulative InnoDB row lock waits on the whole MySQL server.", nil, nil),
		seconds: prometheus.NewDesc("wms_mysql_row_lock_wait_seconds_total", "Cumulative InnoDB row lock wait seconds on the whole MySQL server.", nil, nil),
		success: prometheus.NewDesc("wms_mysql_row_lock_scrape_success", "Whether all required MySQL row lock status values were read.", nil, nil),
	}
}

func (c *rowLockCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.current
	ch <- c.waits
	ch <- c.seconds
	ch <- c.success
}

func (c *rowLockCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	values, err := c.read(ctx)
	if err != nil || len(values) != 3 {
		ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.success, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.current, prometheus.GaugeValue, values["Innodb_row_lock_current_waits"])
	ch <- prometheus.MustNewConstMetric(c.waits, prometheus.CounterValue, values["Innodb_row_lock_waits"])
	ch <- prometheus.MustNewConstMetric(c.seconds, prometheus.CounterValue, values["Innodb_row_lock_time"]/1000)
}

func (c *rowLockCollector) read(ctx context.Context) (map[string]float64, error) {
	rows, err := c.db.QueryContext(ctx, "SHOW GLOBAL STATUS WHERE Variable_name IN ('Innodb_row_lock_current_waits', 'Innodb_row_lock_waits', 'Innodb_row_lock_time')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make(map[string]float64, 3)
	for rows.Next() {
		var name, text string
		if err := rows.Scan(&name, &text); err != nil {
			return nil, err
		}
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, err
		}
		values[name] = value
	}
	return values, rows.Err()
}
