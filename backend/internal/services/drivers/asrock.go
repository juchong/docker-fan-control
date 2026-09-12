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

// ASRockDriver implements IPMI support for ASRock Rack motherboards
type ASRockDriver struct {
	*services.BaseDriver
	ipmi IPMIExecutor
	// Track current speeds for each fan to avoid applying defaults
	currentSpeeds [16]int
}

// NewASRockDriver creates a new ASRock driver
func NewASRockDriver(ipmi IPMIExecutor) *ASRockDriver {
	// Define zone layout for ASRock ROMED8-2T
	// Zone 0: FAN1 (CPU fan)
	// Zone 1: FAN2-FAN7 (System fans)
	layout := services.ZoneLayout{
		Zones: []services.ZoneDefinition{
			{
				ID:           0,
				Name:         "CPU Zone",
				FanIndices:   []int{0}, // FAN1
				Description:  "CPU cooling fan",
				IsDefault:    false,
			},
			{
				ID:           1,
				Name:         "System Zone",
				FanIndices:   []int{1, 2, 3, 4, 5, 6}, // FAN2-FAN7
				Description:  "System cooling fans",
				IsDefault:    true,
			},
		},
	}

	caps := services.DriverCapabilities{
		SupportsManualMode:       true,
		SupportsDutyCycleReading: true,
		SupportsPerZoneControl:   true,
		MaxZones:                 2,
		MaxFans:                  16,
		HasStaticRPMValues:       true, // ASRock reports placeholder RPM values
	}

	driver := &ASRockDriver{
		BaseDriver: services.NewBaseDriver("ASRock Rack", "ROMED8-2T", caps, layout),
		ipmi:       ipmi,
	}

	// Initialize current speeds to a safe default
	for i := range driver.currentSpeeds {
		driver.currentSpeeds[i] = 30
	}

	return driver
}

// DetectFans detects fans using standard IPMI SDR
func (d *ASRockDriver) DetectFans(ctx context.Context) ([]models.DetectedFan, error) {
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
func (d *ASRockDriver) GetFanSpeeds(ctx context.Context) (map[string]int, error) {
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

// GetFanDutyCycles reads duty cycle percentages for all fans using ASRock raw command
func (d *ASRockDriver) GetFanDutyCycles(ctx context.Context) (map[int]int, error) {
	output, err := d.ipmi.RunCommand(ctx, "raw", "0x3a", "0xd7")
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get fan duty cycles via raw command")
		return nil, err
	}

	// Parse hex output: "32 32 32 32 32 32 32 1e 1e 1e 1e 1e 1e 1e 1e 1e"
	result := make(map[int]int)
	fields := strings.Fields(strings.TrimSpace(string(output)))
	for i, field := range fields {
		if i >= 16 {
			break
		}
		var duty int
		if _, err := fmt.Sscanf(field, "%x", &duty); err == nil {
			if duty > 100 {
				duty = 100
			}
			result[i] = duty
		}
	}

	return result, nil
}

// SetFanSpeed sets fan speed for a specific zone
// For ASRock, this updates the internal zone speed tracking and builds the 16-byte command
func (d *ASRockDriver) SetFanSpeed(ctx context.Context, zone int, percent int) error {
	// Clamp percentage
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	// Update zone speeds based on zone
	fansInZone := d.GetFansInZone(zone)
	if fansInZone == nil {
		// If zone not found, use all fans
		fansInZone = []int{0, 1, 2, 3, 4, 5, 6}
	}

	// Build the 16-byte command
	// Update only the fans in the target zone, preserve current speeds for others
	args := make([]string, 16)
	for i := 0; i < 16; i++ {
		// Check if this fan is in the target zone
		for _, fanIdx := range fansInZone {
			if i == fanIdx {
				args[i] = fmt.Sprintf("0x%02x", percent)
				// Update our tracking
				d.currentSpeeds[i] = percent
				break
			}
		}
		// For fans not in the target zone, use their current speed
		if args[i] == "" {
			args[i] = fmt.Sprintf("0x%02x", d.currentSpeeds[i])
		}
	}

	output, err := d.ipmi.RunCommand(ctx, "raw", "0x3a", "0xd6",
		args[0], args[1], args[2], args[3], args[4], args[5], args[6],
		args[7], args[8], args[9], args[10], args[11], args[12], args[13], args[14], args[15])
	if err != nil {
		return fmt.Errorf("ASRock ROMED8 command failed: %w, output: %s", err, string(output))
	}

	log.Debug().Int("zone", zone).Int("percent", percent).Msg("Set fan speed (ASRock ROMED8)")
	return nil
}

// SetAllFanSpeeds sets all fans to the same speed
func (d *ASRockDriver) SetAllFanSpeeds(ctx context.Context, percent int) error {
	// Clamp percentage
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	// Build 16-byte command with same speed for all fans
	args := make([]string, 16)
	for i := 0; i < 16; i++ {
		args[i] = fmt.Sprintf("0x%02x", percent)
		// Update our tracking
		d.currentSpeeds[i] = percent
	}

	output, err := d.ipmi.RunCommand(ctx, "raw", "0x3a", "0xd6",
		args[0], args[1], args[2], args[3], args[4], args[5], args[6],
		args[7], args[8], args[9], args[10], args[11], args[12], args[13], args[14], args[15])
	if err != nil {
		return fmt.Errorf("ASRock ROMED8 command failed: %w, output: %s", err, string(output))
	}

	log.Debug().Int("percent", percent).Msg("Set all fan speeds (ASRock ROMED8)")
	return nil
}

// SetManualMode enables or disables manual fan control
func (d *ASRockDriver) SetManualMode(ctx context.Context, enabled bool) error {
	modeVal := "0x01"
	if !enabled {
		modeVal = "0x00"
	}

	// Build 16-byte command for manual mode
	// Enable manual mode on FAN1-7, leave FAN8-16 as auto
	args := make([]string, 16)
	for i := 0; i < 7; i++ {
		args[i] = modeVal
	}
	for i := 7; i < 16; i++ {
		args[i] = "0x00"
	}

	output, err := d.ipmi.RunCommand(ctx, "raw", "0x3a", "0xd8",
		args[0], args[1], args[2], args[3], args[4], args[5], args[6],
		args[7], args[8], args[9], args[10], args[11], args[12], args[13], args[14], args[15])
	if err != nil {
		return fmt.Errorf("ASRock ROMED8 mode command failed: %w, output: %s", err, string(output))
	}

	log.Debug().Bool("enabled", enabled).Msg("Set manual mode (ASRock ROMED8)")
	return nil
}

// IsManualMode returns whether manual mode is enabled
// Note: This is a best-effort check since we don't track state
func (d *ASRockDriver) IsManualMode() bool {
	return true // Assume manual mode is enabled after we set it
}

// CanDetect attempts to detect if this is an ASRock motherboard
func (d *ASRockDriver) CanDetect(ctx context.Context) bool {
	// Try to read board name
	output, err := d.ipmi.RunCommand(ctx, "raw", "0x3a", "0xa7")
	if err != nil {
		return false
	}

	boardName := strings.TrimSpace(string(output))
	if strings.Contains(strings.ToLower(boardName), "asrock") ||
		strings.Contains(strings.ToLower(boardName), "romed8") {
		return true
	}

	// Try the ROMED8-specific command
	_, err = d.ipmi.RunCommand(ctx, "raw", "0x3a", "0xd7")
	return err == nil
}

// Discover performs additional detection and configuration
func (d *ASRockDriver) Discover(ctx context.Context) error {
	// Try to read board name
	output, err := d.ipmi.RunCommand(ctx, "raw", "0x3a", "0xa7")
	if err == nil {
		boardName := strings.TrimSpace(string(output))
		log.Info().Str("board", boardName).Msg("Detected ASRock motherboard")
	}

	return nil
}
