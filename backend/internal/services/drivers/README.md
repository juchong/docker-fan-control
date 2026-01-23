# IPMI Drivers

This directory contains vendor-specific IPMI driver implementations for the docker-fan-control project.

## Overview

The driver system provides a plugin-based architecture for supporting different motherboard vendors and models. Each driver implements the `IPMIDriver` interface and handles the specific IPMI command formats required by that vendor.

## Driver Interface

All drivers must implement the `IPMIDriver` interface defined in `ipmi_driver.go`:

```go
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
```

## Available Drivers

### 1. ASRock Driver (`asrock.go`)

**Vendor**: ASRock Rack
**Model**: ROMED8-2T and similar
**Features**:
- Supports manual mode control
- Supports duty cycle reading
- Supports per-zone control
- 16 fan positions
- 2 zones (CPU and System)
- **Note**: ASRock BMCs report static RPM values (typically 800 RPM) regardless of actual fan speed

**Zone Layout**:
- Zone 0: FAN1 (CPU fan)
- Zone 1: FAN2-FAN7 (System fans)

**IPMI Commands**:
- Read duty cycles: `raw 0x3a 0xd7`
- Set duty cycles: `raw 0x3a 0xd6 <16 bytes>`
- Set manual mode: `raw 0x3a 0xd8 <16 bytes>`

### 2. Dell Driver (`dell.go`)

**Vendor**: Dell
**Model**: PowerEdge servers
**Features**:
- Supports manual mode control
- Supports per-zone control
- Up to 8 zones
- Up to 32 fans

**IPMI Commands**:
- Set manual mode: `raw 0x30 0x30 0x01 <mode>`
- Set fan speed: `raw 0x30 0x30 0x02 <zone> <percent>`

### 3. Supermicro Driver (`supermicro.go`)

**Vendor**: Supermicro
**Model**: X9/X10/X11 series
**Features**:
- Supports manual mode control
- Supports per-zone control
- Up to 8 zones
- Up to 32 fans

**IPMI Commands**:
- Set manual mode: `raw 0x30 0x45 0x01 <mode>`
- Set fan speed: `raw 0x30 0x70 0x66 0x01 <zone> <percent>`

### 4. Generic Driver (`generic.go`)

**Vendor**: Generic/Unknown
**Model**: Fallback for unsupported motherboards
**Features**:
- No manual mode control
- No per-zone control
- Single zone with all fans
- Uses standard IPMI commands as fallback

## Creating a New Driver

To add support for a new motherboard vendor, follow these steps:

### 1. Create a new driver file

Create a new file in this directory following the pattern of existing drivers:

```go
package drivers

import (
	"context"
	"fmt"

	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/rs/zerolog/log"
)

// NewVendorDriver implements IPMI support for Vendor motherboards
type NewVendorDriver struct {
	*services.BaseDriver
	ipmi *services.IPMIService
}

// NewNewVendorDriver creates a new Vendor driver
func NewNewVendorDriver(ipmi *services.IPMIService) *NewVendorDriver {
	// Define zone layout
	layout := services.ZoneLayout{
		Zones: []services.ZoneDefinition{
			{
				ID:           0,
				Name:         "Zone 0",
				FanIndices:   []int{0, 1, 2, 3},
				Description:  "System fans - Zone 0",
				IsDefault:    true,
			},
			// Add more zones as needed
		},
	}

	caps := services.DriverCapabilities{
		SupportsManualMode:       true,
		SupportsDutyCycleReading: false,
		SupportsPerZoneControl:   true,
		MaxZones:                 4,
		MaxFans:                  16,
		HasStaticRPMValues:       false,
	}

	driver := &NewVendorDriver{
		BaseDriver: services.NewBaseDriver("Vendor", "Model", caps, layout),
		ipmi:       ipmi,
	}

	return driver
}
```

### 2. Implement required methods

Implement all methods from the `IPMIDriver` interface:

```go
// DetectFans detects fans using standard IPMI SDR
func (d *NewVendorDriver) DetectFans(ctx context.Context) ([]models.DetectedFan, error) {
	return d.ipmi.DetectFans(ctx)
}

// GetFanSpeeds gets current RPM readings
func (d *NewVendorDriver) GetFanSpeeds(ctx context.Context) (map[string]int, error) {
	return d.ipmi.GetFanSpeeds(ctx)
}

// SetFanSpeed sets fan speed for a specific zone
func (d *NewVendorDriver) SetFanSpeed(ctx context.Context, zone int, percent int) error {
	// Implement vendor-specific command
	// ...
}

// SetAllFanSpeeds sets all fans to the same speed
func (d *NewVendorDriver) SetAllFanSpeeds(ctx context.Context, percent int) error {
	// Implement vendor-specific command
	// ...
}

// SetManualMode enables or disables manual fan control
func (d *NewVendorDriver) SetManualMode(ctx context.Context, enabled bool) error {
	// Implement vendor-specific command
	// ...
}

// IsManualMode returns whether manual mode is enabled
func (d *NewVendorDriver) IsManualMode() bool {
	return true
}

// CanDetect attempts to detect if this is the correct vendor
func (d *NewVendorDriver) CanDetect(ctx context.Context) bool {
	// Try vendor-specific commands to detect
	_, err := d.ipmi.RunCommand(ctx, "raw", "0xXX", "0xYY")
	return err == nil
}

// Discover performs additional detection and configuration
func (d *NewVendorDriver) Discover(ctx context.Context) error {
	log.Info().Msg("Detected Vendor server")
	return nil
}
```

### 3. Register the driver

The driver will be automatically registered in the main application. The driver registry will attempt to detect the best driver for the current system.

### 4. Test the driver

Test your driver with the motherboard:

```bash
# Test detection
docker-compose exec fan-control-backend ipmitool raw 0xXX 0xYY

# Test fan control
docker-compose exec fan-control-backend ipmitool raw 0xXX 0xZZ <data>
```

## Driver Discovery

The driver discovery process works as follows:

1. **Initialization**: All drivers are registered with the `DriverRegistry`
2. **Detection**: The registry tries each driver's `CanDetect()` method in order
3. **Selection**: The first driver that successfully detects is selected
4. **Fallback**: If no driver detects, the generic driver is used

## Driver Capabilities

Each driver reports its capabilities through the `DriverCapabilities` struct:

```go
type DriverCapabilities struct {
    SupportsManualMode       bool  // Can enable/disable manual mode
    SupportsDutyCycleReading bool  // Can read actual PWM duty cycles
    SupportsPerZoneControl   bool  // Can control zones independently
    MaxZones                 int   // Maximum number of zones supported
    MaxFans                  int   // Maximum number of fans supported
    HasStaticRPMValues       bool  // BMC reports static RPM values
}
```

## Zone Layout

Each driver defines its zone layout through the `ZoneLayout` struct:

```go
type ZoneLayout struct {
    Zones []ZoneDefinition
}

type ZoneDefinition struct {
    ID          int      // Zone identifier
    Name        string   // Human-readable name
    FanIndices  []int    // Which fan positions belong to this zone
    Description string   // Description of the zone
    IsDefault   bool     // Whether this is the default zone
}
```

## Best Practices

1. **Error Handling**: Always handle IPMI command errors gracefully
2. **Logging**: Use appropriate log levels (Debug for normal operations, Info for detection, Warn/Error for issues)
3. **Zone Management**: Define clear zone layouts that match typical motherboard configurations
4. **Backward Compatibility**: Ensure new drivers don't break existing functionality
5. **Documentation**: Document all vendor-specific commands and quirks

## Troubleshooting

### Driver not detected

1. Check that the vendor-specific commands work with `ipmitool`
2. Verify the BMC firmware version
3. Try manual driver selection in the UI

### Fan speeds not changing

1. Check that manual mode is enabled
2. Verify the zone assignments
3. Check for error messages in the logs

### Static RPM values

Some BMCs (notably ASRock) report placeholder RPM values instead of actual tachometer readings. The duty cycle percentage shown is the accurate measure of fan speed. This is a firmware limitation, not a bug.

## Contributing

When contributing a new driver:

1. Include the motherboard model and manufacturer
2. Document the BMC/IPMI firmware version
3. Provide working raw commands
4. Note any quirks or limitations
5. Include test results if possible

Please submit a pull request with your driver implementation and update this README with information about your driver.
