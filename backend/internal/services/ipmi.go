package services

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"docker-fan-control/internal/models"

	"github.com/rs/zerolog/log"
)

// IPMICommandFormat represents the detected IPMI command format
type IPMICommandFormat int

const (
	FormatUnknown IPMICommandFormat = iota
	FormatAsrockROMED8              // 0x3a 0xd6 / 0xd8 with 16 bytes
	FormatAsrockLegacy              // 0x3a 0x01 with 8 bytes
	FormatDell                      // 0x30 0x30
	FormatSupermicro                // 0x30 0x70 0x66
)

// ParseIPMIFormat converts a string format name to IPMICommandFormat
func ParseIPMIFormat(s string) IPMICommandFormat {
	switch s {
	case models.IPMIFormatAsrockROMED8:
		return FormatAsrockROMED8
	case models.IPMIFormatAsrockLegacy:
		return FormatAsrockLegacy
	case models.IPMIFormatDell:
		return FormatDell
	case models.IPMIFormatSupermicro:
		return FormatSupermicro
	default:
		return FormatUnknown
	}
}

// FormatName returns the string name for an IPMICommandFormat
func (f IPMICommandFormat) String() string {
	switch f {
	case FormatAsrockROMED8:
		return models.IPMIFormatAsrockROMED8
	case FormatAsrockLegacy:
		return models.IPMIFormatAsrockLegacy
	case FormatDell:
		return models.IPMIFormatDell
	case FormatSupermicro:
		return models.IPMIFormatSupermicro
	default:
		return models.IPMIFormatAuto
	}
}

// IPMIService handles IPMI fan detection and control
type IPMIService struct {
	mode         string // "local" or "lan"
	host         string
	user         string
	password     string
	mu           sync.RWMutex
	manualMode   bool
	speedFormat  IPMICommandFormat // Cached working format for SetFanSpeed
	modeFormat   IPMICommandFormat // Cached working format for SetManualMode
	zoneSpeeds   [16]int           // Per-fan/zone speed tracking for ASRock ROMED8
}

// NewIPMIService creates a new IPMI service
func NewIPMIService(mode, host, user, password string) *IPMIService {
	svc := &IPMIService{
		mode:     mode,
		host:     host,
		user:     user,
		password: password,
	}
	// Initialize zone speeds to 30% (minimum safe default)
	for i := range svc.zoneSpeeds {
		svc.zoneSpeeds[i] = 30
	}
	return svc
}

// UpdateConfig updates the IPMI configuration
func (s *IPMIService) UpdateConfig(mode, host, user, password string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
	s.host = host
	s.user = user
	s.password = password
	// Reset cached formats when config changes
	s.speedFormat = FormatUnknown
	s.modeFormat = FormatUnknown
}

// SetCommandFormat manually sets the IPMI command format
// Pass FormatUnknown to enable auto-detection
func (s *IPMIService) SetCommandFormat(format IPMICommandFormat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.speedFormat = format
	s.modeFormat = format
	if format == FormatUnknown {
		log.Info().Msg("IPMI command format set to auto-detect")
	} else {
		log.Info().Str("format", format.String()).Msg("IPMI command format manually set")
	}
}

// GetCommandFormat returns the current IPMI command format (detected or manual)
func (s *IPMIService) GetCommandFormat() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.speedFormat.String()
}

// runCommand executes ipmitool with appropriate flags
func (s *IPMIService) runCommand(ctx context.Context, args ...string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var cmdArgs []string

	if s.mode == "lan" && s.host != "" {
		cmdArgs = append(cmdArgs, "-I", "lanplus", "-H", s.host)
		if s.user != "" {
			cmdArgs = append(cmdArgs, "-U", s.user)
		}
		if s.password != "" {
			cmdArgs = append(cmdArgs, "-P", s.password)
		}
	}

	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, "ipmitool", cmdArgs...)
	return cmd.CombinedOutput()
}

// DetectFans scans IPMI SDR for fan sensors
func (s *IPMIService) DetectFans(ctx context.Context) ([]models.DetectedFan, error) {
	output, err := s.runCommand(ctx, "sdr", "list", "full")
	if err != nil {
		return nil, fmt.Errorf("failed to run ipmitool: %w, output: %s", err, string(output))
	}

	return s.parseFanSensors(string(output))
}

// parseFanSensors parses ipmitool sdr output for fan sensors
func (s *IPMIService) parseFanSensors(output string) ([]models.DetectedFan, error) {
	var fans []models.DetectedFan

	// Pattern: "Fan1 RPM         | 2400 RPM          | ok"
	// Or: "FAN1             | 3600 RPM          | ok"
	fanRegex := regexp.MustCompile(`(?i)^([^\|]+)\s*\|\s*(\d+)\s*(RPM)\s*\|\s*(\w+)`)
	
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		
		// Check if line contains "fan" (case insensitive)
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

// GetFanSpeeds reads current RPM for all fans
func (s *IPMIService) GetFanSpeeds(ctx context.Context) (map[string]int, error) {
	output, err := s.runCommand(ctx, "sdr", "list", "full")
	if err != nil {
		return nil, fmt.Errorf("failed to get fan speeds: %w", err)
	}

	fans, err := s.parseFanSensors(string(output))
	if err != nil {
		return nil, err
	}

	result := make(map[string]int)
	for _, fan := range fans {
		result[fan.SensorID] = fan.RPM
		log.Debug().Str("sensor", fan.SensorID).Int("rpm", fan.RPM).Msg("Fan speed read")
	}

	if len(result) == 0 {
		log.Debug().Str("output", string(output)).Msg("No fans found in IPMI output")
	}

	return result, nil
}

// GetFanDutyCycles reads current duty cycle percentages for all fans (ASRock Rack)
// Returns a map of fan index (0-15) to duty cycle percentage (0-100)
func (s *IPMIService) GetFanDutyCycles(ctx context.Context) (map[int]int, error) {
	// Try ASRock Rack ROMED8 command: raw 0x3a 0xd7
	output, err := s.runCommand(ctx, "raw", "0x3a", "0xd7")
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get fan duty cycles via raw command")
		return nil, err
	}

	// Parse output: " 33 33 33 33 33 33 33 1e 1e 1e 1e 1e 1e 1e 1e 1e"
	result := make(map[int]int)
	fields := strings.Fields(strings.TrimSpace(string(output)))
	
	for i, field := range fields {
		if i >= 16 {
			break
		}
		var duty int
		if _, err := fmt.Sscanf(field, "%x", &duty); err == nil {
			// Clamp to 0-100
			if duty > 100 {
				duty = 100
			}
			result[i] = duty
		}
	}

	log.Debug().Interface("duties", result).Msg("Fan duty cycles read")
	return result, nil
}

// FanReading contains both RPM and duty cycle for a fan
type FanReading struct {
	RPM       int
	DutyCycle int // 0-100%
}

// GetFanReadings returns comprehensive fan data including both RPM and duty cycle
func (s *IPMIService) GetFanReadings(ctx context.Context) (map[string]FanReading, error) {
	result := make(map[string]FanReading)

	// Get RPM readings
	output, err := s.runCommand(ctx, "sdr", "list", "full")
	if err != nil {
		return nil, fmt.Errorf("failed to get fan speeds: %w", err)
	}

	fans, err := s.parseFanSensors(string(output))
	if err != nil {
		return nil, err
	}

	for _, fan := range fans {
		result[fan.SensorID] = FanReading{RPM: fan.RPM}
	}

	// Try to get duty cycles (ASRock Rack specific)
	duties, err := s.GetFanDutyCycles(ctx)
	if err == nil {
		// Map duty cycles to fan sensors by index
		// FAN1 -> index 0, FAN2 -> index 1, etc.
		for sensorID, reading := range result {
			// Extract fan number from sensor ID (e.g., "FAN1" -> 1, "FAN7" -> 7)
			var fanNum int
			if _, err := fmt.Sscanf(sensorID, "FAN%d", &fanNum); err == nil && fanNum >= 1 && fanNum <= 16 {
				if duty, ok := duties[fanNum-1]; ok {
					reading.DutyCycle = duty
					result[sensorID] = reading
				}
			}
		}
	}

	return result, nil
}

// SetFanSpeed sets fan speed for a PWM zone (0-100%)
func (s *IPMIService) SetFanSpeed(ctx context.Context, zone int, percent int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	// Check for cached format
	s.mu.RLock()
	cachedFormat := s.speedFormat
	s.mu.RUnlock()

	// If we have a cached format, use it directly
	if cachedFormat != FormatUnknown {
		return s.setFanSpeedWithFormat(ctx, zone, percent, cachedFormat)
	}

	// Auto-detect format by trying each one
	return s.detectAndSetFanSpeed(ctx, zone, percent)
}

// setFanSpeedWithFormat sets fan speed using a specific command format
func (s *IPMIService) setFanSpeedWithFormat(ctx context.Context, zone int, percent int, format IPMICommandFormat) error {
	switch format {
	case FormatAsrockROMED8:
		// ASRock ROMED8 uses a single 16-byte command to set all fan speeds
		// We need to track zone->speed mapping and build the complete command
		// Zone mapping (based on database): Zone 0 = FAN1 (index 0), Zone 1 = FAN2-FAN7 (indices 1-6)
		s.mu.Lock()
		// Update the speed for the appropriate fans based on zone
		switch zone {
		case 0:
			s.zoneSpeeds[0] = percent // FAN1 only
		case 1:
			for i := 1; i <= 6; i++ { // FAN2-FAN7
				s.zoneSpeeds[i] = percent
			}
		default:
			// For other zones or "all" (-1), set all active fans
			for i := 0; i <= 6; i++ {
				s.zoneSpeeds[i] = percent
			}
		}
		
		// Build the 16-byte command with current zone speeds
		args := make([]string, 16)
		for i := 0; i < 16; i++ {
			args[i] = fmt.Sprintf("0x%02x", s.zoneSpeeds[i])
		}
		s.mu.Unlock()

		output, err := s.runCommand(ctx, "raw", "0x3a", "0xd6",
			args[0], args[1], args[2], args[3], args[4], args[5], args[6],
			args[7], args[8], args[9], args[10], args[11], args[12], args[13], args[14], args[15])
		if err != nil {
			return fmt.Errorf("ASRock ROMED8 command failed: %w, output: %s", err, string(output))
		}
		log.Debug().Int("zone", zone).Int("percent", percent).Interface("speeds", s.zoneSpeeds[:7]).Msg("Set fan speed (ASRock ROMED8)")
		return nil

	case FormatAsrockLegacy:
		value64ths := fmt.Sprintf("0x%02x", percent*64/100)
		output, err := s.runCommand(ctx, "raw", "0x3a", "0x01",
			value64ths, value64ths, value64ths, value64ths,
			value64ths, value64ths, value64ths, value64ths)
		if err != nil {
			return fmt.Errorf("ASRock legacy command failed: %w, output: %s", err, string(output))
		}
		log.Debug().Int("zone", zone).Int("percent", percent).Msg("Set fan speed (ASRock legacy)")
		return nil

	case FormatDell:
		hexValue := fmt.Sprintf("0x%02x", percent*255/100)
		zoneHex := "0xff"
		if zone >= 0 && zone < 8 {
			zoneHex = fmt.Sprintf("0x%02x", zone)
		}
		output, err := s.runCommand(ctx, "raw", "0x30", "0x30", "0x02", zoneHex, hexValue)
		if err != nil {
			return fmt.Errorf("Dell command failed: %w, output: %s", err, string(output))
		}
		log.Debug().Int("zone", zone).Int("percent", percent).Msg("Set fan speed (Dell)")
		return nil

	case FormatSupermicro:
		hexValue := fmt.Sprintf("0x%02x", percent*255/100)
		zoneArg := "0x00"
		if zone >= 0 {
			zoneArg = fmt.Sprintf("0x%02x", zone)
		}
		output, err := s.runCommand(ctx, "raw", "0x30", "0x70", "0x66", "0x01", zoneArg, hexValue)
		if err != nil {
			return fmt.Errorf("Supermicro command failed: %w, output: %s", err, string(output))
		}
		log.Debug().Int("zone", zone).Int("percent", percent).Msg("Set fan speed (Supermicro)")
		return nil
	}

	return fmt.Errorf("unknown IPMI format: %d", format)
}

// detectAndSetFanSpeed tries each format and caches the one that works
func (s *IPMIService) detectAndSetFanSpeed(ctx context.Context, zone int, percent int) error {
	// Try ASRock Rack ROMED8-2T format first: raw 0x3a 0xd6 <16 bytes>
	// Use zone-aware speed setting
	s.mu.Lock()
	switch zone {
	case 0:
		s.zoneSpeeds[0] = percent
	case 1:
		for i := 1; i <= 6; i++ {
			s.zoneSpeeds[i] = percent
		}
	default:
		for i := 0; i <= 6; i++ {
			s.zoneSpeeds[i] = percent
		}
	}
	args := make([]string, 16)
	for i := 0; i < 16; i++ {
		args[i] = fmt.Sprintf("0x%02x", s.zoneSpeeds[i])
	}
	s.mu.Unlock()

	output, err := s.runCommand(ctx, "raw", "0x3a", "0xd6",
		args[0], args[1], args[2], args[3], args[4], args[5], args[6],
		args[7], args[8], args[9], args[10], args[11], args[12], args[13], args[14], args[15])
	if err == nil {
		s.mu.Lock()
		s.speedFormat = FormatAsrockROMED8
		s.mu.Unlock()
		log.Info().Int("zone", zone).Int("percent", percent).Msg("Set fan speed (ASRock ROMED8) - format cached")
		return nil
	}
	log.Debug().Err(err).Str("output", string(output)).Msg("ASRock ROMED8 command failed, trying legacy format")

	// Try legacy AsRockRack format: raw 0x3a 0x01 <8 bytes>
	value64ths := fmt.Sprintf("0x%02x", percent*64/100)
	output, err = s.runCommand(ctx, "raw", "0x3a", "0x01",
		value64ths, value64ths, value64ths, value64ths,
		value64ths, value64ths, value64ths, value64ths)
	if err == nil {
		s.mu.Lock()
		s.speedFormat = FormatAsrockLegacy
		s.mu.Unlock()
		log.Info().Int("zone", zone).Int("percent", percent).Msg("Set fan speed (ASRock legacy) - format cached")
		return nil
	}
	log.Debug().Err(err).Str("output", string(output)).Msg("ASRock legacy command failed, trying Dell format")

	// Convert percent to hex (0-FF for Dell)
	hexValue := fmt.Sprintf("0x%02x", percent*255/100)
	zoneHex := "0xff"
	if zone >= 0 && zone < 8 {
		zoneHex = fmt.Sprintf("0x%02x", zone)
	}

	// Try Dell format: raw 0x30 0x30 0x02 0xff <value>
	output, err = s.runCommand(ctx, "raw", "0x30", "0x30", "0x02", zoneHex, hexValue)
	if err == nil {
		s.mu.Lock()
		s.speedFormat = FormatDell
		s.mu.Unlock()
		log.Info().Int("zone", zone).Int("percent", percent).Msg("Set fan speed (Dell) - format cached")
		return nil
	}
	log.Debug().Err(err).Str("output", string(output)).Msg("Dell command failed, trying Supermicro format")

	// Try Supermicro format: raw 0x30 0x70 0x66 0x01 <zone> <value>
	zoneArg := "0x00"
	if zone >= 0 {
		zoneArg = fmt.Sprintf("0x%02x", zone)
	}
	output, err = s.runCommand(ctx, "raw", "0x30", "0x70", "0x66", "0x01", zoneArg, hexValue)
	if err == nil {
		s.mu.Lock()
		s.speedFormat = FormatSupermicro
		s.mu.Unlock()
		log.Info().Int("zone", zone).Int("percent", percent).Msg("Set fan speed (Supermicro) - format cached")
		return nil
	}

	return fmt.Errorf("failed to set fan speed: %w, output: %s", err, string(output))
}

// SetAllFanSpeeds sets all fans to the same speed
func (s *IPMIService) SetAllFanSpeeds(ctx context.Context, percent int) error {
	return s.SetFanSpeed(ctx, 0, percent)
}

// SetManualMode enables or disables manual fan control
func (s *IPMIService) SetManualMode(ctx context.Context, enabled bool) error {
	s.mu.Lock()
	s.manualMode = enabled
	cachedFormat := s.modeFormat
	s.mu.Unlock()

	// If we have a cached format, use it directly
	if cachedFormat != FormatUnknown {
		return s.setManualModeWithFormat(ctx, enabled, cachedFormat)
	}

	// Auto-detect format
	return s.detectAndSetManualMode(ctx, enabled)
}

// setManualModeWithFormat sets manual mode using a specific command format
func (s *IPMIService) setManualModeWithFormat(ctx context.Context, enabled bool, format IPMICommandFormat) error {
	var modeVal string
	if enabled {
		modeVal = "0x01"
	} else {
		modeVal = "0x00"
	}

	switch format {
	case FormatAsrockROMED8:
		output, err := s.runCommand(ctx, "raw", "0x3a", "0xd8",
			modeVal, modeVal, modeVal, modeVal, modeVal, modeVal, modeVal,
			"0x00", "0x00", "0x00", "0x00", "0x00", "0x00", "0x00", "0x00", "0x00")
		if err != nil {
			return fmt.Errorf("ASRock ROMED8 mode command failed: %w, output: %s", err, string(output))
		}
		log.Debug().Bool("enabled", enabled).Msg("Set manual mode (ASRock ROMED8)")
		return nil

	case FormatDell:
		mode := "0x00"
		if !enabled {
			mode = "0x01"
		}
		output, err := s.runCommand(ctx, "raw", "0x30", "0x30", "0x01", mode)
		if err != nil {
			return fmt.Errorf("Dell mode command failed: %w, output: %s", err, string(output))
		}
		log.Debug().Bool("enabled", enabled).Msg("Set manual mode (Dell)")
		return nil

	case FormatSupermicro:
		mode := "0x00"
		if !enabled {
			mode = "0x01"
		}
		output, err := s.runCommand(ctx, "raw", "0x30", "0x45", "0x01", mode)
		if err != nil {
			return fmt.Errorf("Supermicro mode command failed: %w, output: %s", err, string(output))
		}
		log.Debug().Bool("enabled", enabled).Msg("Set manual mode (Supermicro)")
		return nil

	case FormatAsrockLegacy:
		// Legacy ASRock doesn't have a separate mode command, use ROMED8
		_, err := s.runCommand(ctx, "raw", "0x3a", "0xd8",
			modeVal, modeVal, modeVal, modeVal, modeVal, modeVal, modeVal,
			"0x00", "0x00", "0x00", "0x00", "0x00", "0x00", "0x00", "0x00", "0x00")
		if err != nil {
			log.Debug().Err(err).Msg("ASRock legacy mode command not supported, continuing")
			return nil
		}
		log.Debug().Bool("enabled", enabled).Msg("Set manual mode (ASRock legacy)")
		return nil
	}

	return nil
}

// detectAndSetManualMode tries each format and caches the one that works
func (s *IPMIService) detectAndSetManualMode(ctx context.Context, enabled bool) error {
	var modeVal string
	if enabled {
		modeVal = "0x01"
	} else {
		modeVal = "0x00"
	}

	// Try ASRock Rack ROMED8-2T format: raw 0x3a 0xd8 <16 bytes>
	output, err := s.runCommand(ctx, "raw", "0x3a", "0xd8",
		modeVal, modeVal, modeVal, modeVal, modeVal, modeVal, modeVal,
		"0x00", "0x00", "0x00", "0x00", "0x00", "0x00", "0x00", "0x00", "0x00")
	if err == nil {
		s.mu.Lock()
		s.modeFormat = FormatAsrockROMED8
		s.mu.Unlock()
		log.Info().Bool("enabled", enabled).Msg("Set manual fan control mode (ASRock ROMED8) - format cached")
		return nil
	}
	log.Debug().Err(err).Str("output", string(output)).Msg("ASRock ROMED8 mode command failed, trying Dell/Supermicro")

	// Fallback to Dell/Supermicro commands
	mode := "0x00"
	if !enabled {
		mode = "0x01"
	}

	// Dell format: raw 0x30 0x30 0x01 <mode>
	output, err = s.runCommand(ctx, "raw", "0x30", "0x30", "0x01", mode)
	if err == nil {
		s.mu.Lock()
		s.modeFormat = FormatDell
		s.mu.Unlock()
		log.Info().Bool("enabled", enabled).Msg("Set manual fan control mode (Dell) - format cached")
		return nil
	}
	log.Debug().Err(err).Str("output", string(output)).Msg("Dell manual mode command failed, trying Supermicro")

	// Supermicro format: raw 0x30 0x45 0x01 <mode>
	output, err = s.runCommand(ctx, "raw", "0x30", "0x45", "0x01", mode)
	if err == nil {
		s.mu.Lock()
		s.modeFormat = FormatSupermicro
		s.mu.Unlock()
		log.Info().Bool("enabled", enabled).Msg("Set manual fan control mode (Supermicro) - format cached")
		return nil
	}

	log.Debug().Err(err).Str("output", string(output)).Msg("Manual mode command not supported, continuing")
	return nil
}

// IsManualMode returns whether manual mode is enabled
func (s *IPMIService) IsManualMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manualMode
}

// IdentifyFan spins a specific fan zone to 100% for identification
func (s *IPMIService) IdentifyFan(ctx context.Context, zone int, duration time.Duration) error {
	// Save current manual mode state
	wasManual := s.IsManualMode()

	// Enable manual mode
	if err := s.SetManualMode(ctx, true); err != nil {
		return err
	}

	// Set the fan to 100%
	if err := s.SetFanSpeed(ctx, zone, 100); err != nil {
		return err
	}

	// Wait for duration
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
	}

	// Restore manual mode if it wasn't enabled before
	if !wasManual {
		if err := s.SetManualMode(ctx, false); err != nil {
			log.Warn().Err(err).Msg("Failed to restore automatic fan control")
		}
	}

	return nil
}

// TestConnection tests the IPMI connection
func (s *IPMIService) TestConnection(ctx context.Context) error {
	output, err := s.runCommand(ctx, "chassis", "status")
	if err != nil {
		return fmt.Errorf("IPMI connection failed: %w, output: %s", err, string(output))
	}
	return nil
}

// GetChassisStatus returns chassis power status
func (s *IPMIService) GetChassisStatus(ctx context.Context) (map[string]string, error) {
	output, err := s.runCommand(ctx, "chassis", "status")
	if err != nil {
		return nil, fmt.Errorf("failed to get chassis status: %w", err)
	}

	result := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return result, nil
}
