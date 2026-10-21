package config

const (
	DefaultMaxShipmentDetails = 200
	DefaultMaxAllocationRows  = 1000
)

// ApplyShipmentScaleDefaults keeps omitted/nonpositive limits bounded, including
// internal service constructors that do not load YAML. These are not tenant quotas.
func (c *LimitsConfig) ApplyShipmentScaleDefaults() {
	if c.MaxShipmentDetails <= 0 {
		c.MaxShipmentDetails = DefaultMaxShipmentDetails
	}
	if c.MaxAllocationRows <= 0 {
		c.MaxAllocationRows = DefaultMaxAllocationRows
	}
}
