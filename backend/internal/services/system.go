package services

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"docker-fan-control/internal/models"

	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemService handles system metrics
type SystemService struct {
	// Cache for drive info (model, serial) to avoid repeated smartctl calls
	driveCache     map[string]*driveInfo
	driveCacheMu   sync.RWMutex
	driveCacheTime time.Time
}

type driveInfo struct {
	Model  string
	Serial string
	Type   string
}

// NewSystemService creates a new system service
func NewSystemService() *SystemService {
	return &SystemService{
		driveCache: make(map[string]*driveInfo),
	}
}

// GetMetrics returns system metrics
func (s *SystemService) GetMetrics() (*models.SystemMetrics, error) {
	metrics := &models.SystemMetrics{
		CPUPackages: []models.CPUPackageMetrics{},
		Drives:      []models.DriveMetrics{},
	}

	// Get CPU load
	cpuPercent, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercent) > 0 {
		metrics.CPULoad = cpuPercent[0]
	}

	// Get memory info
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		metrics.MemoryUsed = memInfo.Used
		metrics.MemoryTotal = memInfo.Total
	}

	// Get CPU package temperatures
	metrics.CPUPackages = s.GetCPUPackageTemperatures()

	// Set legacy CPUTemp field (max of all packages)
	if len(metrics.CPUPackages) > 0 {
		maxTemp := metrics.CPUPackages[0].Temperature
		for _, pkg := range metrics.CPUPackages[1:] {
			if pkg.Temperature > maxTemp {
				maxTemp = pkg.Temperature
			}
		}
		metrics.CPUTemp = &maxTemp
	}

	// Get drive temperatures
	metrics.Drives = s.GetDriveTemperatures()

	return metrics, nil
}

// GetCPUPackageTemperatures returns temperature readings for all CPU packages
func (s *SystemService) GetCPUPackageTemperatures() []models.CPUPackageMetrics {
	var packages []models.CPUPackageMetrics
	packageIndex := 0

	// Scan hwmon for CPU temperature sensors
	hwmonPaths, _ := filepath.Glob("/sys/class/hwmon/hwmon*/name")
	for _, namePath := range hwmonPaths {
		data, err := os.ReadFile(namePath)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(data))

		// Look for CPU temperature drivers
		if name == "coretemp" || name == "k10temp" || name == "zenpower" {
			dir := filepath.Dir(namePath)
			tempFiles, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))

			for _, tf := range tempFiles {
				// Check label to identify package temps vs core temps
				labelFile := strings.Replace(tf, "_input", "_label", 1)
				label := ""
				if labelData, err := os.ReadFile(labelFile); err == nil {
					label = strings.TrimSpace(string(labelData))
				}

				// Only include Package temperatures (not individual cores)
				// For AMD (k10temp/zenpower), include Tctl/Tdie
				isPackage := strings.Contains(label, "Package") ||
					strings.Contains(label, "Tctl") ||
					strings.Contains(label, "Tdie")

				if !isPackage && label != "" {
					continue
				}

				// Read temperature
				tempData, err := os.ReadFile(tf)
				if err != nil {
					continue
				}
				tempStr := strings.TrimSpace(string(tempData))
				temp, err := strconv.ParseFloat(tempStr, 64)
				if err != nil {
					continue
				}

				// Temperature is in millidegrees
				if temp > 1000 {
					temp = temp / 1000.0
				}

				if temp > 0 && temp < 150 {
					displayName := label
					if displayName == "" {
						displayName = name
					}
					packages = append(packages, models.CPUPackageMetrics{
						Index:       packageIndex,
						Name:        displayName,
						Temperature: temp,
					})
					packageIndex++
				}
			}
		}
	}

	// Fallback: try thermal zones if no hwmon packages found
	if len(packages) == 0 {
		thermalZones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
		for i, zonePath := range thermalZones {
			data, err := os.ReadFile(zonePath)
			if err != nil {
				continue
			}
			tempStr := strings.TrimSpace(string(data))
			temp, err := strconv.ParseFloat(tempStr, 64)
			if err != nil {
				continue
			}
			if temp > 1000 {
				temp = temp / 1000.0
			}
			if temp > 0 && temp < 150 {
				// Try to get zone type
				typeFile := strings.Replace(zonePath, "/temp", "/type", 1)
				zoneName := "CPU"
				if typeData, err := os.ReadFile(typeFile); err == nil {
					zoneName = strings.TrimSpace(string(typeData))
				}
				packages = append(packages, models.CPUPackageMetrics{
					Index:       i,
					Name:        zoneName,
					Temperature: temp,
				})
			}
		}
	}

	return packages
}

// GetDriveTemperatures returns temperature readings for all drives using smartctl
func (s *SystemService) GetDriveTemperatures() []models.DriveMetrics {
	var drives []models.DriveMetrics

	// First, scan for devices
	devices := s.scanDrives()
	if len(devices) == 0 {
		return drives
	}

	// Get temperature for each device
	for i, device := range devices {
		temp, info := s.getDriveTemperature(device)
		if temp > 0 {
			driveType := "hdd"
			if info != nil && info.Type != "" {
				driveType = info.Type
			}
			model := device
			serial := ""
			if info != nil {
				if info.Model != "" {
					model = info.Model
				}
				serial = info.Serial
			}

			drives = append(drives, models.DriveMetrics{
				Index:       i,
				Device:      device,
				Model:       model,
				Serial:      serial,
				Type:        driveType,
				Temperature: temp,
			})
		}
	}

	return drives
}

// scanDrives finds all block devices that support SMART
func (s *SystemService) scanDrives() []string {
	var devices []string

	// Try smartctl --scan first
	cmd := exec.Command("smartctl", "--scan", "--json")
	output, err := cmd.Output()
	if err == nil {
		var scanResult struct {
			Devices []struct {
				Name     string `json:"name"`
				Type     string `json:"type"`
				Protocol string `json:"protocol"`
			} `json:"devices"`
		}
		if json.Unmarshal(output, &scanResult) == nil {
			for _, dev := range scanResult.Devices {
				devices = append(devices, dev.Name)
			}
			return devices
		}
	}

	// Fallback: scan /dev for common drive patterns
	patterns := []string{
		"/dev/sd[a-z]",
		"/dev/nvme[0-9]n[0-9]",
		"/dev/hd[a-z]",
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		devices = append(devices, matches...)
	}

	return devices
}

// getDriveTemperature gets temperature and info for a single drive
func (s *SystemService) getDriveTemperature(device string) (int, *driveInfo) {
	// Check cache for drive info
	s.driveCacheMu.RLock()
	cachedInfo, hasCached := s.driveCache[device]
	cacheAge := time.Since(s.driveCacheTime)
	s.driveCacheMu.RUnlock()

	// Refresh cache every 5 minutes for model/serial info
	refreshInfo := !hasCached || cacheAge > 5*time.Minute

	// Run smartctl to get temperature (and info if needed)
	args := []string{"-A", "--json"}
	if refreshInfo {
		args = []string{"-i", "-A", "--json"}
	}
	args = append(args, device)

	cmd := exec.Command("smartctl", args...)
	output, _ := cmd.Output() // Ignore error - smartctl may return non-zero exit codes even with valid data

	if len(output) == 0 {
		return 0, cachedInfo
	}

	var result struct {
		ModelName    string `json:"model_name"`
		SerialNumber string `json:"serial_number"`
		DeviceType   string `json:"device_type"`
		RotationRate int    `json:"rotation_rate"`
		AtaSmartAttr struct {
			Table []struct {
				ID    int    `json:"id"`
				Name  string `json:"name"`
				Value int    `json:"value"`
				Raw   struct {
					Value int `json:"value"`
				} `json:"raw"`
			} `json:"table"`
		} `json:"ata_smart_attributes"`
		NvmeSmartHealth struct {
			Temperature int `json:"temperature"`
		} `json:"nvme_smart_health_information_log"`
		Temperature struct {
			Current int `json:"current"`
		} `json:"temperature"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return 0, cachedInfo
	}

	// Update cache with drive info if we got it
	if refreshInfo && (result.ModelName != "" || result.SerialNumber != "") {
		info := &driveInfo{
			Model:  result.ModelName,
			Serial: result.SerialNumber,
		}

		// Determine drive type
		if strings.Contains(strings.ToLower(device), "nvme") {
			info.Type = "nvme"
		} else if result.RotationRate == 0 {
			info.Type = "ssd"
		} else {
			info.Type = "hdd"
		}

		s.driveCacheMu.Lock()
		s.driveCache[device] = info
		s.driveCacheTime = time.Now()
		s.driveCacheMu.Unlock()
		cachedInfo = info
	}

	// Extract temperature
	temp := 0

	// Try direct temperature field first
	if result.Temperature.Current > 0 {
		temp = result.Temperature.Current
	}

	// Try NVMe health log
	if temp == 0 && result.NvmeSmartHealth.Temperature > 0 {
		temp = result.NvmeSmartHealth.Temperature
	}

	// Try SMART attributes (ID 194 = Temperature, ID 190 = Airflow Temperature)
	if temp == 0 {
		for _, attr := range result.AtaSmartAttr.Table {
			if attr.ID == 194 || attr.ID == 190 {
				temp = attr.Raw.Value
				// Some drives report in different format
				if temp > 100 {
					temp = temp & 0xFF // Take lower byte
				}
				break
			}
		}
	}

	return temp, cachedInfo
}

// GetCPUTemperature returns the maximum CPU package temperature (legacy method)
func (s *SystemService) GetCPUTemperature() float64 {
	packages := s.GetCPUPackageTemperatures()
	if len(packages) == 0 {
		return 0
	}
	maxTemp := packages[0].Temperature
	for _, pkg := range packages[1:] {
		if pkg.Temperature > maxTemp {
			maxTemp = pkg.Temperature
		}
	}
	return maxTemp
}

// GetCPULoad returns just the CPU load percentage
func (s *SystemService) GetCPULoad() float64 {
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil || len(cpuPercent) == 0 {
		return 0
	}
	return cpuPercent[0]
}

// GetMemoryUsage returns memory usage percentage
func (s *SystemService) GetMemoryUsage() float64 {
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return 0
	}
	return memInfo.UsedPercent
}

// LogSystemInfo logs system information on startup
func (s *SystemService) LogSystemInfo() {
	cpuInfo, err := cpu.Info()
	if err == nil && len(cpuInfo) > 0 {
		log.Info().
			Str("model", cpuInfo[0].ModelName).
			Int32("cores", cpuInfo[0].Cores).
			Msg("CPU detected")
	}

	memInfo, err := mem.VirtualMemory()
	if err == nil {
		log.Info().
			Uint64("total_gb", memInfo.Total/1024/1024/1024).
			Msg("Memory detected")
	}
}
