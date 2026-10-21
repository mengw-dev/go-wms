package config

import (
	"strings"
	"testing"
)

func TestShipmentScaleConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, detailsEnv, rowsEnv string
		details, rows                   int
	}{
		{"omitted", "server:\n  mode: debug\n", "", "", 200, 1000},
		{"nonpositive", "limits:\n  max_shipment_details: -1\n  max_allocation_rows: 0\n", "", "", 200, 1000},
		{"configured", "limits:\n  max_shipment_details: 50\n  max_allocation_rows: 250\n", "", "", 50, 250},
		{"env_without_yaml_keys", "server:\n  mode: debug\n", "40", "80", 40, 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WMS_LIMITS_MAX_SHIPMENT_DETAILS", tc.detailsEnv)
			t.Setenv("WMS_LIMITS_MAX_ALLOCATION_ROWS", tc.rowsEnv)
			content := tc.yaml
			if !strings.Contains(content, "server:") {
				content = "server:\n  port: 8080\n  mode: debug\n" + content
			} else {
				content = strings.Replace(content, "server:", "server:\n  port: 8080", 1)
			}
			cfg, err := Load(writeConfig(t, content))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Limits.MaxShipmentDetails != tc.details || cfg.Limits.MaxAllocationRows != tc.rows {
				t.Fatalf("limits=%+v", cfg.Limits)
			}
		})
	}
}
