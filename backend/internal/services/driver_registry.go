package services

import (
	"context"
	"sort"
)

// DriverRegistry manages available IPMI drivers and their discovery

type DriverRegistry struct {
	drivers       []IPMIDriver
	preferredOrder map[string]int // Vendor/model to preferred order
}

// NewDriverRegistry creates a new driver registry
func NewDriverRegistry() *DriverRegistry {
	registry := &DriverRegistry{
		preferredOrder: make(map[string]int),
	}
	
	// Set preferred order (higher priority first)
	registry.SetPreferredOrder([]string{
		"ASRock Rack",
		"Dell",
		"Supermicro",
		"Generic",
	})
	
	return registry
}

// SetPreferredOrder sets the preferred driver detection order
func (r *DriverRegistry) SetPreferredOrder(order []string) {
	for idx, vendor := range order {
		r.preferredOrder[vendor] = idx
	}
}

// RegisterDriver adds a driver to the registry
func (r *DriverRegistry) RegisterDriver(driver IPMIDriver) {
	r.drivers = append(r.drivers, driver)
}

// GetAvailableDrivers returns all registered drivers
func (r *DriverRegistry) GetAvailableDrivers() []IPMIDriver {
	return r.drivers
}

// DetectBestDriver attempts to detect the best driver for the current system
func (r *DriverRegistry) DetectBestDriver(ctx context.Context) (IPMIDriver, error) {
	// Sort drivers by preferred order
	sortedDrivers := make([]IPMIDriver, len(r.drivers))
	copy(sortedDrivers, r.drivers)
	
	sort.Slice(sortedDrivers, func(i, j int) bool {
		vendorI := sortedDrivers[i].GetVendor()
		vendorJ := sortedDrivers[j].GetVendor()
		
		orderI, hasI := r.preferredOrder[vendorI]
		orderJ, hasJ := r.preferredOrder[vendorJ]
		
		if !hasI && !hasJ {
			return i < j
		}
		if !hasI {
			return false // j comes first
		}
		if !hasJ {
			return true // i comes first
		}
		
		return orderI < orderJ
	})
	
	// Try each driver in order
	for _, driver := range sortedDrivers {
		if driver.CanDetect(ctx) {
			if err := driver.Discover(ctx); err == nil {
				return driver, nil
			}
		}
	}
	
	// If no driver detected, return the generic driver
	for _, driver := range sortedDrivers {
		if driver.GetVendor() == "Generic" {
			return driver, nil
		}
	}
	
	return nil, nil
}

// GetDriverByVendor returns a driver by vendor name
func (r *DriverRegistry) GetDriverByVendor(vendor string) IPMIDriver {
	for _, driver := range r.drivers {
		if driver.GetVendor() == vendor {
			return driver
		}
	}
	return nil
}

// GetDriverByName returns a driver by vendor and model
func (r *DriverRegistry) GetDriverByName(vendor, model string) IPMIDriver {
	for _, driver := range r.drivers {
		if driver.GetVendor() == vendor && driver.GetModel() == model {
			return driver
		}
	}
	return nil
}

// GetDriverCapabilities returns capabilities for a specific vendor
func (r *DriverRegistry) GetDriverCapabilities(vendor string) *DriverCapabilities {
	driver := r.GetDriverByVendor(vendor)
	if driver != nil {
		caps := driver.GetCapabilities()
		return &caps
	}
	return nil
}

// GetAllDriverInfo returns information about all available drivers
func (r *DriverRegistry) GetAllDriverInfo() []DriverInfo {
	info := make([]DriverInfo, len(r.drivers))
	for i, driver := range r.drivers {
		info[i] = DriverInfo{
			Vendor:      driver.GetVendor(),
			Model:       driver.GetModel(),
			Capabilities: driver.GetCapabilities(),
			ZoneLayout:  driver.GetZoneLayout(),
		}
	}
	return info
}

// DriverInfo contains metadata about a driver
type DriverInfo struct {
	Vendor       string
	Model        string
	Capabilities DriverCapabilities
	ZoneLayout   ZoneLayout
}
