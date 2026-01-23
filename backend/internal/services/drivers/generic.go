package drivers

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/rs/zerolog/log"
)

// GenericDriver provides basic IPMI support for unsupported motherboards
// It uses standard IPMI commands and tries to work with any BMC

type GenericDriver struct {
	*services.BaseDriver
	ipmi IPMIExecutor
}

// NewGenericDriver creates a new generic driver
func NewGenericDriver(ipmi IPMIExecutor) *GenericDriver {
	// Generic zone layout - single zone with all fans
	layout := services.ZoneLayout{
		Zones: []services.ZoneDefinition{
			{
				ID:           0,
				Name:         "All Fans",
				FanIndices:   []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
				Description:  "All detected fans (generic mode)",
				IsDefault:    true,
			},
		},
	}

	caps := services.DriverCapabilities{
		SupportsManualMode:       false,
		SupportsDutyCycleReading: false,
		SupportsPerZoneControl:   false,
		MaxZones:                 1,
		MaxFans:                  32,
		HasStaticRPMValues:       false,
	}

	driver := &GenericDriver{
		BaseDriver: services.NewBaseDriver("Generic", "Unknown", caps, layout),
		ipmi:       ipmi,
	}

	return driver
}

// DetectFans detects fans using standard IPMI SDR
func (d *GenericDriver) DetectFans(ctx context.Context) ([]models.DetectedFan, error) {
	output, err := d.ipmi.RunCommand(ctx, "sdr", "list", "full")
	if err != nil {
		return nil, fmt.Errorf("failed to run ipmitool: %w, output: %s", err, string(output))
	}

	var fans []models.DetectedFan
	fanRegex := regexp.MustCompile(`(?i)^([^\|]+)\s*\|\s*(\d+)\s*(RPM)\s*\|\s*(\w+)`)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(strings.ToLower(line), "fan") {
			continue
		}

		matches := fanRegex.FindStringSubmatch(line)
		if matches != nil {
			rpm, _ := strconv.Atoi(matches[2])
			fans = append(fans, models.DetectedFan{
				SensorID: strings.TrimSpace(matches[1]),
				Name:     strings.TrimSpace(matches[1]),
				RPM:      rpm,
				Unit:     matches[3],
				Status:   strings.ToLower(matches[4]),
			})
		}
	}

	return fans, nil
}

// GetFanSpeeds gets current RPM readings from IPMI SDR
func (d *GenericDriver) GetFanSpeeds(ctx context.Context) (map[string]int, error) {
	output, err := d.ipmi.RunCommand(ctx, "sdr", "list", "full")
	if err != nil {
		return nil, fmt.Errorf("failed to get fan speeds: %w", err)
	}

	result := make(map[string]int)
	fanRegex := regexp.MustCompile(`(?i)^([^\|]+)\s*\|\s*(\d+)\s*RPM`)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(strings.ToLower(line), "fan") {
			continue
		}

		matches := fanRegex.FindStringSubmatch(line)
		if matches != nil {
			sensorID := strings.TrimSpace(matches[1])
			rpm, _ := strconv.Atoi(matches[2])
			result[sensorID] = rpm
		}
	}

	return result, nil
}

// GetFanDutyCycles is not supported in generic mode
func (d *GenericDriver) GetFanDutyCycles(ctx context.Context) (map[int]int, error) {
	return make(map[int]int), nil
}

// SetFanSpeed sets fan speed for all fans (no per-zone support)
func (d *GenericDriver) SetFanSpeed(ctx context.Context, zone int, percent int) error {
	// In generic mode, ignore zone and set all fans
	return d.SetAllFanSpeeds(ctx, percent)
}

// SetAllFanSpeeds sets all fans to the same speed
// Uses standard IPMI fan control if available
func (d *GenericDriver) SetAllFanSpeeds(ctx context.Context, percent int) error {
	// Clamp percentage
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	// Try standard IPMI fan control
	// This may not work on all BMCs, but provides a fallback
	_, err := d.ipmi.RunCommand(ctx, "fan", "set", "1", fmt.Sprintf("%d", percent))
	if err == nil {
		log.Debug().Int("percent", percent).Msg("Set all fan speeds (Generic - standard IPMI)")
		return nil
	}

	// Fallback: Try to set each fan individually via raw commands
	// This is a best-effort approach
	log.Warn().Err(err).Msg("Standard IPMI fan control not available, trying fallback methods")
	
	// Try common vendor commands as fallback
	return fmt.Errorf("generic fan control not available: %w", err)
}

// SetManualMode is not supported in generic mode
func (d *GenericDriver) SetManualMode(ctx context.Context, enabled bool) error {
	// In generic mode, we assume manual mode is always enabled
	// since we can't control it
	log.Warn().Msg("Manual mode control not supported in generic mode")
	return nil
}

// IsManualMode returns whether manual mode is enabled
func (d *GenericDriver) IsManualMode() bool {
	return true // Assume manual mode for generic driver
}

// CanDetect always returns true for generic driver
// It's the fallback when no other driver works
func (d *GenericDriver) CanDetect(ctx context.Context) bool {
	return true
}

// Discover performs basic detection
func (d *GenericDriver) Discover(ctx context.Context) error {
	log.Info().Msg("Using generic IPMI driver (vendor-specific driver not detected)")
	
	// Try to get some information about the system
	output, err := d.ipmi.RunCommand(ctx, "mc", "info")
	if err == nil {
		log.Info().Str("info", string(output)).Msg("BMC Information")
	}

	return nil
}
