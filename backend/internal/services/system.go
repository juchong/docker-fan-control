package services

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
	// sysRoot/devRoot default to /sys and /dev; tests point them at fakes.
	sysRoot string
	devRoot string

	// Cache for drive info (model, serial) to avoid repeated smartctl calls
	driveCache     map[string]*driveInfo
	driveCacheMu   sync.RWMutex
	driveCacheTime time.Time

	// smartctl fallback results are cached briefly (it is an external process
	// and GetMetrics runs every control cycle and every monitoring request).
	driveFallback   []models.DriveMetrics
	driveFallbackAt time.Time

	driveSourceLogged bool
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

	// Get motherboard/VRM/chipset temperatures (Super-I/O)
	metrics.BoardTemps = s.GetBoardTemperatures()

	return metrics, nil
}

// GetBoardTemperatures returns motherboard/VRM/chipset temperatures from the
// Super-I/O chips (Nuvoton nct6xxx, ITE it8xxx — every chip, on boards with
// more than one). Aux/unconnected channels commonly read bogus values (0,
// ~127, or -55), so readings outside a plausible range are dropped. These are
// exposed as OPT-IN profile inputs only and never drive the emergency
// threshold (see controller.getMaxTemperature).
func (s *SystemService) GetBoardTemperatures() []models.BoardTempMetrics {
	var temps []models.BoardTempMetrics
	idx := 0

	hwmonPaths, _ := filepath.Glob(filepath.Join(s.hwmonRoot(), "hwmon*/name"))
	sort.Strings(hwmonPaths)
	for _, namePath := range hwmonPaths {
		data, err := os.ReadFile(namePath)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(data))
		if !strings.HasPrefix(name, "nct6") && !strings.HasPrefix(name, "it8") {
			continue
		}

		dir := filepath.Dir(namePath)
		tempFiles, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
		sort.Strings(tempFiles) // stable index ordering
		for _, tf := range tempFiles {
			tempData, err := os.ReadFile(tf)
			if err != nil {
				continue
			}
			temp, err := strconv.ParseFloat(strings.TrimSpace(string(tempData)), 64)
			if err != nil {
				continue
			}
			if temp > 1000 {
				temp = temp / 1000.0 // millidegrees
			}
			// Per-sensor sanity: drop implausible/disconnected-sensor readings
			// (nct6xxx aux channels report ~127C or 0C when nothing is attached).
			if temp < 5 || temp > 125 {
				continue
			}

			label := ""
			if lb, err := os.ReadFile(strings.Replace(tf, "_input", "_label", 1)); err == nil {
				label = strings.TrimSpace(string(lb))
			}
			if label == "" {
				// No label from the driver (it87 exposes none): name the sensor
				// by chip and hardware number, e.g. "it8689 temp5", trimming
				// Gigabyte's SIV suffix ("it8689_9a0a0908").
				chip := name
				if i := strings.IndexByte(chip, '_'); i > 0 {
					chip = chip[:i]
				}
				n := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(tf), "temp"), "_input")
				label = fmt.Sprintf("%s temp%s", chip, n)
			}

			temps = append(temps, models.BoardTempMetrics{
				Index:       idx,
				Name:        label,
				Temperature: temp,
			})
			idx++
		}
	}

	return temps
}

// cpuModelName caches the marketing CPU name (e.g., "AMD Ryzen 9 9950X"), which
// is stable for the life of the process.
var (
	cpuModelOnce sync.Once
	cpuModelName string
)

// getCPUModelName returns the CPU's marketing model name via gopsutil, falling
// back to /proc/cpuinfo. Empty if it can't be determined.
func getCPUModelName() string {
	cpuModelOnce.Do(func() {
		if infos, err := cpu.Info(); err == nil {
			for _, info := range infos {
				if m := strings.TrimSpace(info.ModelName); m != "" {
					cpuModelName = m
					return
				}
			}
		}
		// Fallback: parse /proc/cpuinfo directly.
		if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "model name") {
					if idx := strings.Index(line, ":"); idx >= 0 {
						cpuModelName = strings.TrimSpace(line[idx+1:])
						return
					}
				}
			}
		}
	})
	return cpuModelName
}

// GetCPUPackageTemperatures returns temperature readings for all CPU packages
func (s *SystemService) GetCPUPackageTemperatures() []models.CPUPackageMetrics {
	var packages []models.CPUPackageMetrics
	packageIndex := 0
	cpuModel := getCPUModelName()

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
						Model:       cpuModel,
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
					Model:       cpuModel,
					Temperature: temp,
				})
			}
		}
	}

	return packages
}

// ---- drives ---------------------------------------------------------------

func (s *SystemService) sysfs() string {
	if s.sysRoot != "" {
		return s.sysRoot
	}
	return "/sys"
}

func (s *SystemService) hwmonRoot() string { return filepath.Join(s.sysfs(), "class", "hwmon") }

func (s *SystemService) devfs() string {
	if s.devRoot != "" {
		return s.devRoot
	}
	return "/dev"
}

// GetDriveTemperatures returns temperature readings for all drives.
//
// The primary source is hwmon, which the kernel populates for NVMe (in-tree
// nvme driver) and — with the drivetemp module loaded — SATA/SAS drives. It is
// read through /sys, so an unprivileged container with no /dev block nodes
// still sees its drives. smartctl is only a fallback for drives hwmon didn't
// cover, and only when /dev actually has block nodes (upstream's privileged
// IPMI deployments, kernels without these hwmon drivers).
//
// Drives are ordered by device name, never by hwmon number: profile inputs
// address them as drive_temp<index> and must not remap across boots.
func (s *SystemService) GetDriveTemperatures() []models.DriveMetrics {
	drives := s.hwmonDrives()

	covered := make(map[string]bool, len(drives))
	for _, d := range drives {
		covered[d.Device] = true
	}
	drives = append(drives, s.smartctlDrives(covered)...)

	sort.Slice(drives, func(i, j int) bool { return drives[i].Device < drives[j].Device })
	for i := range drives {
		drives[i].Index = i
	}

	s.logDriveSourcesOnce(drives)
	if drives == nil {
		drives = []models.DriveMetrics{} // JSON [] rather than null
	}
	return drives
}

var nvmeNamespaceRe = regexp.MustCompile(`^nvme\d+n\d+$`)

// hwmonDrives reads every nvme / drivetemp hwmon chip.
func (s *SystemService) hwmonDrives() []models.DriveMetrics {
	var out []models.DriveMetrics
	namePaths, _ := filepath.Glob(filepath.Join(s.hwmonRoot(), "hwmon*/name"))
	for _, np := range namePaths {
		dir := filepath.Dir(np)
		var d models.DriveMetrics
		var ok bool
		switch readTrim(np) {
		case "nvme":
			d, ok = s.nvmeDrive(dir)
		case "drivetemp":
			d, ok = s.drivetempDrive(dir)
		default:
			continue
		}
		if ok {
			out = append(out, d)
		}
	}
	return out
}

// nvmeDrive reads one nvme hwmon chip. Its device link is the NVMe controller
// (model/serial/firmware_rev, and the namespace directory that names the block
// device); the Composite channel is the drive's own aggregate temperature and
// carries its warning/critical thresholds, the rest are extra sensors.
func (s *SystemService) nvmeDrive(hwmonDir string) (models.DriveMetrics, bool) {
	ctrl, err := filepath.EvalSymlinks(filepath.Join(hwmonDir, "device"))
	if err != nil {
		return models.DriveMetrics{}, false
	}
	temps := readHwmonTemps(hwmonDir)
	if len(temps) == 0 {
		return models.DriveMetrics{}, false
	}
	primary := 0
	for i, t := range temps {
		if strings.EqualFold(t.label, "Composite") {
			primary = i
			break
		}
	}

	device := "/dev/" + filepath.Base(ctrl)
	if entries, err := os.ReadDir(ctrl); err == nil {
		var ns []string
		for _, e := range entries {
			if nvmeNamespaceRe.MatchString(e.Name()) {
				ns = append(ns, e.Name())
			}
		}
		if len(ns) > 0 {
			sort.Strings(ns)
			device = "/dev/" + ns[0]
		}
	}

	d := models.DriveMetrics{
		Device:      device,
		Model:       readTrim(filepath.Join(ctrl, "model")),
		Serial:      readTrim(filepath.Join(ctrl, "serial")),
		Firmware:    readTrim(filepath.Join(ctrl, "firmware_rev")),
		Type:        "nvme",
		Temperature: int(math.Round(temps[primary].temp)),
		Max:         temps[primary].max,
		Crit:        temps[primary].crit,
		Source:      "hwmon",
	}
	if d.Model == "" {
		d.Model = device
	}
	for i, t := range temps {
		if i == primary {
			continue
		}
		label := t.label
		if label == "" {
			label = fmt.Sprintf("temp%d", t.n)
		}
		d.Sensors = append(d.Sensors, models.DriveSensor{Label: label, Temperature: t.temp})
	}
	return d, true
}

// drivetempDrive reads one drivetemp hwmon chip (SATA/SAS via the in-tree
// drivetemp module). Its device link is the SCSI device: model/vendor, and the
// block/ directory that names the disk.
func (s *SystemService) drivetempDrive(hwmonDir string) (models.DriveMetrics, bool) {
	scsi, err := filepath.EvalSymlinks(filepath.Join(hwmonDir, "device"))
	if err != nil {
		return models.DriveMetrics{}, false
	}
	temps := readHwmonTemps(hwmonDir)
	if len(temps) == 0 {
		return models.DriveMetrics{}, false
	}

	block := ""
	if entries, err := os.ReadDir(filepath.Join(scsi, "block")); err == nil && len(entries) > 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		block = names[0]
	}
	device := "/dev/" + filepath.Base(scsi)
	typ := "hdd"
	if block != "" {
		device = "/dev/" + block
		if readTrim(filepath.Join(s.sysfs(), "block", block, "queue", "rotational")) == "0" {
			typ = "ssd"
		}
	}
	model := readTrim(filepath.Join(scsi, "model"))
	if vendor := readTrim(filepath.Join(scsi, "vendor")); vendor != "" && !strings.EqualFold(vendor, "ATA") {
		model = strings.TrimSpace(vendor + " " + model)
	}
	if model == "" {
		model = device
	}
	return models.DriveMetrics{
		Device:      device,
		Model:       model,
		Type:        typ,
		Temperature: int(math.Round(temps[0].temp)),
		Max:         temps[0].max,
		Crit:        temps[0].crit,
		Source:      "hwmon",
	}, true
}

// smartctlDrives is the legacy path, for hosts that hand the container block
// devices but whose kernel exposes no hwmon for them. It only runs when /dev
// has candidate nodes, skips drives hwmon already reported, and caches its
// result briefly.
func (s *SystemService) smartctlDrives(covered map[string]bool) []models.DriveMetrics {
	if !s.devHasBlockNodes() {
		return nil
	}
	s.driveCacheMu.RLock()
	cached, at := s.driveFallback, s.driveFallbackAt
	s.driveCacheMu.RUnlock()
	if !at.IsZero() && time.Since(at) < 10*time.Second {
		return cached
	}

	var out []models.DriveMetrics
	for _, device := range s.scanDrives() {
		if covered[device] {
			continue
		}
		temp, info := s.getDriveTemperature(device)
		if temp <= 0 {
			continue
		}
		d := models.DriveMetrics{Device: device, Model: device, Type: "hdd", Temperature: temp, Source: "smartctl"}
		if info != nil {
			if info.Model != "" {
				d.Model = info.Model
			}
			d.Serial = info.Serial
			if info.Type != "" {
				d.Type = info.Type
			}
		}
		out = append(out, d)
	}

	s.driveCacheMu.Lock()
	s.driveFallback, s.driveFallbackAt = out, time.Now()
	s.driveCacheMu.Unlock()
	return out
}

func (s *SystemService) devHasBlockNodes() bool {
	for _, p := range []string{"sd[a-z]*", "nvme[0-9]*n[0-9]*", "hd[a-z]*"} {
		if m, _ := filepath.Glob(filepath.Join(s.devfs(), p)); len(m) > 0 {
			return true
		}
	}
	return false
}

func (s *SystemService) logDriveSourcesOnce(drives []models.DriveMetrics) {
	s.driveCacheMu.Lock()
	defer s.driveCacheMu.Unlock()
	if s.driveSourceLogged {
		return
	}
	s.driveSourceLogged = true
	hw, sm := 0, 0
	for _, d := range drives {
		if d.Source == "smartctl" {
			sm++
		} else {
			hw++
		}
	}
	log.Info().Int("hwmon", hw).Int("smartctl", sm).Msg("Drive temperature sources")
}

// hwmonTemp is one tempN channel of a hwmon chip.
type hwmonTemp struct {
	n     int
	label string
	temp  float64 // °C
	max   *int    // °C, nil when absent or a placeholder
	crit  *int
}

// readHwmonTemps reads every tempN channel of a hwmon dir in channel order.
func readHwmonTemps(dir string) []hwmonTemp {
	inputs, _ := filepath.Glob(filepath.Join(dir, "temp[0-9]*_input"))
	var out []hwmonTemp
	for _, in := range inputs {
		raw, err := strconv.ParseFloat(readTrim(in), 64)
		if err != nil {
			continue
		}
		base := strings.TrimSuffix(in, "_input")
		n, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(base), "temp"))
		out = append(out, hwmonTemp{
			n:     n,
			label: readTrim(base + "_label"),
			temp:  raw / 1000,
			max:   plausibleThreshold(readTrim(base + "_max")),
			crit:  plausibleThreshold(readTrim(base + "_crit")),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].n < out[j].n })
	return out
}

// plausibleThreshold parses a millidegree threshold, dropping the placeholder
// values drives report for channels that have none (NVMe "Sensor N" channels
// expose e.g. 65261850 = 0xFFFF kelvin).
func plausibleThreshold(v string) *int {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	c := int(math.Round(f / 1000))
	if c <= 0 || c > 150 {
		return nil
	}
	return &c
}

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
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
