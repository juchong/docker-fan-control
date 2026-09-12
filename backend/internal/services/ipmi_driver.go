package services

import (
	"context"
	"docker-fan-control/internal/models"
)

// IPMIDriver defines the interface for vendor-specific IPMI implementations
type IPMIDriver interface {
	// Basic operations
	DetectFans(ctx context.Context) ([]models.DetectedFan, error)
	GetFanSpeeds(ctx context.Context) (map[string]int, error)
	GetFanDutyCycles(ctx context.Context) (map[int]int, error)
	SetFanSpeed(ctx context.Context, zone int, percent int) error
	SetAllFanSpeeds(ctx context.Context, percent int) error
	SetManualMode(ctx context.Context, enabled bool) error
	IsManualMode() bool

	// Metadata
	GetVendor() string
	GetModel() string
	GetCapabilities() DriverCapabilities
	GetZoneLayout() ZoneLayout

	// Discovery
	Discover(ctx context.Context) error
	CanDetect(ctx context.Context) bool
}

// FanReadingProvider is an optional driver capability. Drivers that can report
// RPM and duty together, keyed by the same SensorID they emit from DetectFans,
// implement it so IPMIService.GetFanReadings surfaces accurate duty without
// guessing the id format. Legacy IPMI drivers don't implement it (they fall
// back to the SDR-based path).
type FanReadingProvider interface {
	GetFanReadings(ctx context.Context) (map[string]FanReading, error)
}

// DriverCapabilities describes what features a driver supports
type DriverCapabilities struct {
	SupportsManualMode       bool
	SupportsDutyCycleReading bool
	SupportsPerZoneControl   bool
	MaxZones                 int
	MaxFans                  int
	HasStaticRPMValues       bool // Some BMCs report placeholder RPM values
}

// ZoneLayout defines the fan zone configuration for a motherboard
type ZoneLayout = models.ZoneLayout

// ZoneDefinition describes a fan zone and which fans belong to it
type ZoneDefinition = models.ZoneDefinition

// BaseDriver provides common functionality for all IPMI drivers
type BaseDriver struct {
	vendor      string
	model       string
	capabilities DriverCapabilities
	zoneLayout  ZoneLayout
}

// NewBaseDriver creates a new base driver with common functionality
func NewBaseDriver(vendor, model string, caps DriverCapabilities, layout ZoneLayout) *BaseDriver {
	return &BaseDriver{
		vendor:      vendor,
		model:       model,
		capabilities: caps,
		zoneLayout:  layout,
	}
}

// GetVendor returns the vendor name
func (d *BaseDriver) GetVendor() string {
	return d.vendor
}

// GetModel returns the model name
func (d *BaseDriver) GetModel() string {
	return d.model
}

// GetCapabilities returns the driver's capabilities
func (d *BaseDriver) GetCapabilities() DriverCapabilities {
	return d.capabilities
}

// GetZoneLayout returns the zone layout for this driver
func (d *BaseDriver) GetZoneLayout() ZoneLayout {
	return d.zoneLayout
}

// GetZoneForFan returns the zone ID for a given fan index
func (d *BaseDriver) GetZoneForFan(fanIndex int) (int, error) {
	for _, zone := range d.zoneLayout.Zones {
		for _, idx := range zone.FanIndices {
			if idx == fanIndex {
				return zone.ID, nil
			}
		}
	}
	
	// Return default zone if not found
	for _, zone := range d.zoneLayout.Zones {
		if zone.IsDefault {
			return zone.ID, nil
		}
	}
	
	return 0, nil
}

// GetFansInZone returns all fan indices that belong to a zone
func (d *BaseDriver) GetFansInZone(zoneID int) []int {
	for _, zone := range d.zoneLayout.Zones {
		if zone.ID == zoneID {
			return zone.FanIndices
		}
	}
	return nil
}

// CanDetect is a default implementation that can be overridden
func (d *BaseDriver) CanDetect(ctx context.Context) bool {
	return true
}

// Discover is a default implementation that can be overridden
func (d *BaseDriver) Discover(ctx context.Context) error {
	return nil
}

// GetFanDutyCycles provides a default empty implementation
func (d *BaseDriver) GetFanDutyCycles(ctx context.Context) (map[int]int, error) {
	return make(map[int]int), nil
}
