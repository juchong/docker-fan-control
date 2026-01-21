package services

import (
	"sync"

	"docker-fan-control/internal/models"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/rs/zerolog/log"
)

// GPUService handles NVIDIA GPU monitoring
type GPUService struct {
	initialized bool
	deviceCount int
	mu          sync.RWMutex
}

// NewGPUService creates a new GPU service
func NewGPUService() *GPUService {
	s := &GPUService{}
	s.init()
	return s
}

// init initializes NVML
func (s *GPUService) init() {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		log.Warn().Str("error", nvml.ErrorString(ret)).Msg("Failed to initialize NVML - GPU monitoring disabled")
		return
	}

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		log.Warn().Str("error", nvml.ErrorString(ret)).Msg("Failed to get GPU count")
		nvml.Shutdown()
		return
	}

	s.initialized = true
	s.deviceCount = count
	log.Info().Int("count", count).Msg("NVML initialized successfully")
}

// IsAvailable returns whether GPU monitoring is available
func (s *GPUService) IsAvailable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

// GetDeviceCount returns the number of GPUs
func (s *GPUService) GetDeviceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deviceCount
}

// GetMetrics returns metrics for all GPUs
func (s *GPUService) GetMetrics() ([]models.GPUMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.initialized {
		return []models.GPUMetrics{}, nil
	}

	var metrics []models.GPUMetrics

	for i := 0; i < s.deviceCount; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			log.Warn().Int("index", i).Str("error", nvml.ErrorString(ret)).Msg("Failed to get GPU handle")
			continue
		}

		metric := models.GPUMetrics{Index: i}

		// Get name
		name, ret := device.GetName()
		if ret == nvml.SUCCESS {
			metric.Name = name
		}

		// Get temperature
		temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
		if ret == nvml.SUCCESS {
			metric.Temperature = int(temp)
		}

		// Get utilization
		util, ret := device.GetUtilizationRates()
		if ret == nvml.SUCCESS {
			metric.Load = int(util.Gpu)
		}

		// Get fan speed
		fanSpeed, ret := device.GetFanSpeed()
		if ret == nvml.SUCCESS {
			metric.FanSpeed = int(fanSpeed)
		}

		// Get memory info
		memInfo, ret := device.GetMemoryInfo()
		if ret == nvml.SUCCESS {
			metric.MemoryUsed = memInfo.Used
			metric.MemoryTotal = memInfo.Total
		}

		// Get power usage
		power, ret := device.GetPowerUsage()
		if ret == nvml.SUCCESS {
			metric.PowerUsage = float64(power) / 1000.0 // Convert mW to W
		}

		// Get power limit
		powerLimit, ret := device.GetPowerManagementLimit()
		if ret == nvml.SUCCESS {
			metric.PowerLimit = float64(powerLimit) / 1000.0 // Convert mW to W
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// GetTemperature returns temperature for a specific GPU
func (s *GPUService) GetTemperature(index int) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.initialized || index >= s.deviceCount {
		return 0, nil
	}

	device, ret := nvml.DeviceGetHandleByIndex(index)
	if ret != nvml.SUCCESS {
		return 0, nil
	}

	temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret != nvml.SUCCESS {
		return 0, nil
	}

	return int(temp), nil
}

// GetLoad returns GPU utilization for a specific GPU
func (s *GPUService) GetLoad(index int) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.initialized || index >= s.deviceCount {
		return 0, nil
	}

	device, ret := nvml.DeviceGetHandleByIndex(index)
	if ret != nvml.SUCCESS {
		return 0, nil
	}

	util, ret := device.GetUtilizationRates()
	if ret != nvml.SUCCESS {
		return 0, nil
	}

	return int(util.Gpu), nil
}

// GetMaxTemperature returns the maximum temperature across all GPUs
func (s *GPUService) GetMaxTemperature() int {
	metrics, err := s.GetMetrics()
	if err != nil || len(metrics) == 0 {
		return 0
	}

	maxTemp := 0
	for _, m := range metrics {
		if m.Temperature > maxTemp {
			maxTemp = m.Temperature
		}
	}
	return maxTemp
}

// GetAvgTemperature returns the average temperature across all GPUs
func (s *GPUService) GetAvgTemperature() float64 {
	metrics, err := s.GetMetrics()
	if err != nil || len(metrics) == 0 {
		return 0
	}

	total := 0
	for _, m := range metrics {
		total += m.Temperature
	}
	return float64(total) / float64(len(metrics))
}

// Close shuts down NVML
func (s *GPUService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		nvml.Shutdown()
		s.initialized = false
	}
}
