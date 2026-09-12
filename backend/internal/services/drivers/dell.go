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

// DellDriver implements IPMI support for Dell PowerEdge servers
type DellDriver struct {
	*services.BaseDriver
	ipmi IPMIExecutor
}

// NewDellDriver creates a new Dell driver
func NewDellDriver(ipmi IPMIExecutor) *DellDriver {
	// Define zone layout for Dell PowerEdge
	// Dell typically has multiple zones (0-7)
	layout := services.ZoneLayout{
		Zones: []services.ZoneDefinition{
			{
				ID:           0,
				Name:         "Zone 0",
				FanIndices:   []int{0, 1, 2, 3}, // Example mapping
				Description:  "System fans - Zone 0",
				IsDefault:    true,
			},
			{
				ID:           1,
				Name:         "Zone 1",
				FanIndices:   []int{4, 5, 6, 7},
				Description:  "System fans - Zone 1",
				IsDefault:    false,
			},
			{
				ID:           2,
				Name:         "Zone 2",
				FanIndices:   []int{8, 9, 10, 11},
				Description:  "System fans - Zone 2",
				IsDefault:    false,
			},
			{
				ID:           3,
				Name:         "Zone 3",
				FanIndices:   []int{12, 13, 14, 15},
				Description:  "System fans - Zone 3",
				IsDefault:    false,
			},
		},
	}

	caps := services.DriverCapabilities{
		SupportsManualMode:       true,
		SupportsDutyCycleReading: false,
		SupportsPerZoneControl:   true,
		MaxZones:                 8,
		MaxFans:                  32,
		HasStaticRPMValues:       false,
	}

	driver := &DellDriver{
		BaseDriver: services.NewBaseDriver("Dell", "PowerEdge", caps, layout),
		ipmi:       ipmi,
	}

	return driver
}

// DetectFans detects fans using standard IPMI SDR
func (d *DellDriver) DetectFans(ctx context.Context) ([]models.DetectedFan, error) {
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
			idx := len(fans) // ordinal fan index → control zone via the layout
			zone, _ := d.GetZoneForFan(idx)
			fans = append(fans, models.DetectedFan{
				SensorID: strings.TrimSpace(matches[1]),
				Name:     strings.TrimSpace(matches[1]),
				RPM:      rpm,
				Channel:  idx + 1,
				ZoneID:   zone,
				Unit:     matches[3],
				Status:   strings.ToLower(matches[4]),
			})
		}
	}

	return fans, nil
}

// GetFanSpeeds gets current RPM readings from IPMI SDR
func (d *DellDriver) GetFanSpeeds(ctx context.Context) (map[string]int, error) {
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

// GetFanDutyCycles is not supported by Dell
func (d *DellDriver) GetFanDutyCycles(ctx context.Context) (map[int]int, error) {
	return make(map[int]int), nil
}

// SetFanSpeed sets fan speed for a specific zone
func (d *DellDriver) SetFanSpeed(ctx context.Context, zone int, percent int) error {
	// Clamp percentage
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	// Convert percent to hex (0-FF)
	hexValue := fmt.Sprintf("0x%02x", percent*255/100)
	zoneHex := fmt.Sprintf("0x%02x", zone)

	output, err := d.ipmi.RunCommand(ctx, "raw", "0x30", "0x30", "0x02", zoneHex, hexValue)
	if err != nil {
		return fmt.Errorf("Dell command failed: %w, output: %s", err, string(output))
	}

	log.Debug().Int("zone", zone).Int("percent", percent).Msg("Set fan speed (Dell)")
	return nil
}

// SetAllFanSpeeds sets all fans to the same speed using zone 0xFF
func (d *DellDriver) SetAllFanSpeeds(ctx context.Context, percent int) error {
	// Clamp percentage
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	// Convert percent to hex (0-FF)
	hexValue := fmt.Sprintf("0x%02x", percent*255/100)

	output, err := d.ipmi.RunCommand(ctx, "raw", "0x30", "0x30", "0x02", "0xff", hexValue)
	if err != nil {
		return fmt.Errorf("Dell command failed: %w, output: %s", err, string(output))
	}

	log.Debug().Int("percent", percent).Msg("Set all fan speeds (Dell)")
	return nil
}

// SetManualMode enables or disables manual fan control
func (d *DellDriver) SetManualMode(ctx context.Context, enabled bool) error {
	mode := "0x00"
	if !enabled {
		mode = "0x01"
	}

	output, err := d.ipmi.RunCommand(ctx, "raw", "0x30", "0x30", "0x01", mode)
	if err != nil {
		return fmt.Errorf("Dell mode command failed: %w, output: %s", err, string(output))
	}

	log.Debug().Bool("enabled", enabled).Msg("Set manual mode (Dell)")
	return nil
}

// IsManualMode returns whether manual mode is enabled
func (d *DellDriver) IsManualMode() bool {
	return true // Assume manual mode is enabled after we set it
}

// CanDetect attempts to detect if this is a Dell server
func (d *DellDriver) CanDetect(ctx context.Context) bool {
	// Try the Dell-specific command
	_, err := d.ipmi.RunCommand(ctx, "raw", "0x30", "0x30", "0x01", "0x00")
	if err == nil {
		return true
	}

	// Also try the fan speed command
	_, err = d.ipmi.RunCommand(ctx, "raw", "0x30", "0x30", "0x02", "0x00", "0x32")
	return err == nil
}

// Discover performs additional detection
func (d *DellDriver) Discover(ctx context.Context) error {
	log.Info().Msg("Detected Dell PowerEdge server")
	return nil
}
