package drivers

import (
	"context"
	"testing"

	"docker-fan-control/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockIPMIExecutor is a mock implementation of IPMIExecutor for testing
type MockIPMIExecutor struct {
	mock.Mock
}

// RunCommand mock implementation
func (m *MockIPMIExecutor) RunCommand(ctx context.Context, args ...string) ([]byte, error) {
	callArgs := m.Called(ctx, args)
	return callArgs.Get(0).([]byte), callArgs.Error(1)
}

// TestASRockDriver_CanDetect tests ASRock driver detection
func TestASRockDriver_CanDetect(t *testing.T) {
	mockIPMI := new(MockIPMIExecutor)
	driver := NewASRockDriver(mockIPMI)
	
	ctx := context.Background()
	
	// Test successful detection
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x3a", "0xa7"}).Return([]byte("ASRock Rack ROMED8-2T"), nil)
	assert.True(t, driver.CanDetect(ctx))
	mockIPMI.AssertExpectations(t)
	
	// Test detection via command (each mock needs its own driver instance).
	mockIPMI2 := new(MockIPMIExecutor)
	mockIPMI2.On("RunCommand", ctx, []string{"raw", "0x3a", "0xa7"}).Return([]byte("Unknown"), nil)
	mockIPMI2.On("RunCommand", ctx, []string{"raw", "0x3a", "0xd7"}).Return([]byte("33 33 33 33 33 33 33 1e"), nil)
	assert.True(t, NewASRockDriver(mockIPMI2).CanDetect(ctx))
	mockIPMI2.AssertExpectations(t)

	// Test failed detection. The first probe erroring returns false immediately,
	// so the second probe is not issued.
	mockIPMI3 := new(MockIPMIExecutor)
	mockIPMI3.On("RunCommand", ctx, []string{"raw", "0x3a", "0xa7"}).Return([]byte(""), assert.AnError)
	assert.False(t, NewASRockDriver(mockIPMI3).CanDetect(ctx))
	mockIPMI3.AssertExpectations(t)
}

// TestDellDriver_CanDetect tests Dell driver detection
func TestDellDriver_CanDetect(t *testing.T) {
	mockIPMI := new(MockIPMIExecutor)
	driver := NewDellDriver(mockIPMI)
	
	ctx := context.Background()
	
	// Test successful detection
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x30", "0x30", "0x01", "0x00"}).Return([]byte(""), nil)
	assert.True(t, driver.CanDetect(ctx))
	mockIPMI.AssertExpectations(t)
	
	// Test detection via fan speed command (each mock needs its own driver).
	mockIPMI2 := new(MockIPMIExecutor)
	mockIPMI2.On("RunCommand", ctx, []string{"raw", "0x30", "0x30", "0x01", "0x00"}).Return([]byte(""), assert.AnError)
	mockIPMI2.On("RunCommand", ctx, []string{"raw", "0x30", "0x30", "0x02", "0x00", "0x32"}).Return([]byte(""), nil)
	assert.True(t, NewDellDriver(mockIPMI2).CanDetect(ctx))
	mockIPMI2.AssertExpectations(t)

	// Test failed detection
	mockIPMI3 := new(MockIPMIExecutor)
	mockIPMI3.On("RunCommand", ctx, []string{"raw", "0x30", "0x30", "0x01", "0x00"}).Return([]byte(""), assert.AnError)
	mockIPMI3.On("RunCommand", ctx, []string{"raw", "0x30", "0x30", "0x02", "0x00", "0x32"}).Return([]byte(""), assert.AnError)
	assert.False(t, NewDellDriver(mockIPMI3).CanDetect(ctx))
	mockIPMI3.AssertExpectations(t)
}

// TestSupermicroDriver_CanDetect tests Supermicro driver detection
func TestSupermicroDriver_CanDetect(t *testing.T) {
	mockIPMI := new(MockIPMIExecutor)
	driver := NewSupermicroDriver(mockIPMI)
	
	ctx := context.Background()
	
	// Test successful detection
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x30", "0x45", "0x01", "0x00"}).Return([]byte(""), nil)
	assert.True(t, driver.CanDetect(ctx))
	mockIPMI.AssertExpectations(t)
	
	// Test detection via fan speed command (each mock needs its own driver).
	mockIPMI2 := new(MockIPMIExecutor)
	mockIPMI2.On("RunCommand", ctx, []string{"raw", "0x30", "0x45", "0x01", "0x00"}).Return([]byte(""), assert.AnError)
	mockIPMI2.On("RunCommand", ctx, []string{"raw", "0x30", "0x70", "0x66", "0x01", "0x00", "0x32"}).Return([]byte(""), nil)
	assert.True(t, NewSupermicroDriver(mockIPMI2).CanDetect(ctx))
	mockIPMI2.AssertExpectations(t)

	// Test failed detection
	mockIPMI3 := new(MockIPMIExecutor)
	mockIPMI3.On("RunCommand", ctx, []string{"raw", "0x30", "0x45", "0x01", "0x00"}).Return([]byte(""), assert.AnError)
	mockIPMI3.On("RunCommand", ctx, []string{"raw", "0x30", "0x70", "0x66", "0x01", "0x00", "0x32"}).Return([]byte(""), assert.AnError)
	assert.False(t, NewSupermicroDriver(mockIPMI3).CanDetect(ctx))
	mockIPMI3.AssertExpectations(t)
}

// TestGenericDriver_CanDetect tests generic driver detection
func TestGenericDriver_CanDetect(t *testing.T) {
	driver := NewGenericDriver(nil)
	assert.True(t, driver.CanDetect(context.Background()))
}

// TestDriverCapabilities tests that drivers report correct capabilities
func TestDriverCapabilities(t *testing.T) {
	// ASRock capabilities
	asrock := NewASRockDriver(new(MockIPMIExecutor))
	caps := asrock.GetCapabilities()
	assert.True(t, caps.SupportsManualMode)
	assert.True(t, caps.SupportsDutyCycleReading)
	assert.True(t, caps.SupportsPerZoneControl)
	assert.Equal(t, 2, caps.MaxZones)
	assert.Equal(t, 16, caps.MaxFans)
	assert.True(t, caps.HasStaticRPMValues)
	
	// Dell capabilities
	dell := NewDellDriver(new(MockIPMIExecutor))
	caps = dell.GetCapabilities()
	assert.True(t, caps.SupportsManualMode)
	assert.False(t, caps.SupportsDutyCycleReading)
	assert.True(t, caps.SupportsPerZoneControl)
	assert.Equal(t, 8, caps.MaxZones)
	assert.Equal(t, 32, caps.MaxFans)
	assert.False(t, caps.HasStaticRPMValues)
	
	// Supermicro capabilities
	supermicro := NewSupermicroDriver(new(MockIPMIExecutor))
	caps = supermicro.GetCapabilities()
	assert.True(t, caps.SupportsManualMode)
	assert.False(t, caps.SupportsDutyCycleReading)
	assert.True(t, caps.SupportsPerZoneControl)
	assert.Equal(t, 8, caps.MaxZones)
	assert.Equal(t, 32, caps.MaxFans)
	assert.False(t, caps.HasStaticRPMValues)
	
	// Generic capabilities
	generic := NewGenericDriver(new(MockIPMIExecutor))
	caps = generic.GetCapabilities()
	assert.False(t, caps.SupportsManualMode)
	assert.False(t, caps.SupportsDutyCycleReading)
	assert.False(t, caps.SupportsPerZoneControl)
	assert.Equal(t, 1, caps.MaxZones)
	assert.Equal(t, 32, caps.MaxFans)
	assert.False(t, caps.HasStaticRPMValues)
}

// TestZoneLayout tests that drivers have correct zone layouts
func TestZoneLayout(t *testing.T) {
	// ASRock zone layout
	asrock := NewASRockDriver(new(MockIPMIExecutor))
	layout := asrock.GetZoneLayout()
	assert.Equal(t, 2, len(layout.Zones))
	
	// Zone 0 should be CPU zone with FAN1
	zone0 := layout.Zones[0]
	assert.Equal(t, 0, zone0.ID)
	assert.Equal(t, "CPU Zone", zone0.Name)
	assert.Equal(t, 1, len(zone0.FanIndices))
	assert.Equal(t, 0, zone0.FanIndices[0])
	assert.False(t, zone0.IsDefault)
	
	// Zone 1 should be System zone with FAN2-FAN7
	zone1 := layout.Zones[1]
	assert.Equal(t, 1, zone1.ID)
	assert.Equal(t, "System Zone", zone1.Name)
	assert.Equal(t, 6, len(zone1.FanIndices))
	assert.True(t, zone1.IsDefault)
	
	// Test GetZoneForFan
	zone, err := asrock.GetZoneForFan(0)
	assert.NoError(t, err)
	assert.Equal(t, 0, zone)
	
	zone, err = asrock.GetZoneForFan(1)
	assert.NoError(t, err)
	assert.Equal(t, 1, zone)
	
	zone, err = asrock.GetZoneForFan(10)
	assert.NoError(t, err)
	assert.Equal(t, 1, zone) // Should return default zone
}

// TestDriverMetadata tests vendor and model information
func TestDriverMetadata(t *testing.T) {
	asrock := NewASRockDriver(new(MockIPMIExecutor))
	assert.Equal(t, "ASRock Rack", asrock.GetVendor())
	assert.Equal(t, "ROMED8-2T", asrock.GetModel())
	
	dell := NewDellDriver(new(MockIPMIExecutor))
	assert.Equal(t, "Dell", dell.GetVendor())
	assert.Equal(t, "PowerEdge", dell.GetModel())
	
	supermicro := NewSupermicroDriver(new(MockIPMIExecutor))
	assert.Equal(t, "Supermicro", supermicro.GetVendor())
	assert.Equal(t, "X9/X10/X11", supermicro.GetModel())
	
	generic := NewGenericDriver(new(MockIPMIExecutor))
	assert.Equal(t, "Generic", generic.GetVendor())
	assert.Equal(t, "Unknown", generic.GetModel())
}

// TestDriverRegistry tests driver registration and discovery
func TestDriverRegistry(t *testing.T) {
	registry := services.NewDriverRegistry()
	
	// Test initial state
	drivers := registry.GetAvailableDrivers()
	assert.Empty(t, drivers)
	
	// Test registration
	mockIPMI := new(MockIPMIExecutor)
	registry.RegisterDriver(NewASRockDriver(mockIPMI))
	registry.RegisterDriver(NewDellDriver(mockIPMI))
	registry.RegisterDriver(NewSupermicroDriver(mockIPMI))
	registry.RegisterDriver(NewGenericDriver(mockIPMI))
	
	drivers = registry.GetAvailableDrivers()
	assert.Equal(t, 4, len(drivers))
	
	// Test GetDriverByVendor
	asrock := registry.GetDriverByVendor("ASRock Rack")
	assert.NotNil(t, asrock)
	assert.Equal(t, "ASRock Rack", asrock.GetVendor())
	
	dell := registry.GetDriverByVendor("Dell")
	assert.NotNil(t, dell)
	assert.Equal(t, "Dell", dell.GetVendor())
	
	// Test GetDriverByName
	asrock2 := registry.GetDriverByName("ASRock Rack", "ROMED8-2T")
	assert.NotNil(t, asrock2)
	assert.Equal(t, "ASRock Rack", asrock2.GetVendor())
	assert.Equal(t, "ROMED8-2T", asrock2.GetModel())
	
	// Test GetDriverCapabilities
	caps := registry.GetDriverCapabilities("ASRock Rack")
	assert.NotNil(t, caps)
	assert.True(t, caps.SupportsManualMode)
	
	// Test GetAllDriverInfo
	info := registry.GetAllDriverInfo()
	assert.Equal(t, 4, len(info))
	assert.Equal(t, "ASRock Rack", info[0].Vendor)
}

// TestDriverDiscoveryOrder tests that drivers are tried in correct order
func TestDriverDiscoveryOrder(t *testing.T) {
	registry := services.NewDriverRegistry()
	
	mockIPMI := new(MockIPMIExecutor)
	registry.RegisterDriver(NewASRockDriver(mockIPMI))
	registry.RegisterDriver(NewDellDriver(mockIPMI))
	registry.RegisterDriver(NewSupermicroDriver(mockIPMI))
	registry.RegisterDriver(NewGenericDriver(mockIPMI))
	
	ctx := context.Background()
	
	// Mock ASRock detection to fail, Dell to succeed. ASRock's CanDetect returns
	// false as soon as its first probe (0x3a 0xa7) errors, so its second probe is
	// never issued and must not be mocked.
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x3a", "0xa7"}).Return([]byte(""), assert.AnError)
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x30", "0x30", "0x01", "0x00"}).Return([]byte(""), nil)
	
	driver, err := registry.DetectBestDriver(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, driver)
	assert.Equal(t, "Dell", driver.GetVendor())
	
	mockIPMI.AssertExpectations(t)
}

// TestFallbackToGeneric tests that generic driver is used when no other driver detects
func TestFallbackToGeneric(t *testing.T) {
	registry := services.NewDriverRegistry()
	
	mockIPMI := new(MockIPMIExecutor)
	registry.RegisterDriver(NewASRockDriver(mockIPMI))
	registry.RegisterDriver(NewDellDriver(mockIPMI))
	registry.RegisterDriver(NewSupermicroDriver(mockIPMI))
	registry.RegisterDriver(NewGenericDriver(mockIPMI))
	
	ctx := context.Background()
	
	// Mock all drivers to fail detection. ASRock stops after its first probe when
	// that errors, so only 0x3a 0xa7 is issued (not 0x3a 0xd7).
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x3a", "0xa7"}).Return([]byte(""), assert.AnError)
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x30", "0x30", "0x01", "0x00"}).Return([]byte(""), assert.AnError)
	// Dell's CanDetect falls through to a second probe when the first fails.
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x30", "0x30", "0x02", "0x00", "0x32"}).Return([]byte(""), assert.AnError)
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x30", "0x45", "0x01", "0x00"}).Return([]byte(""), assert.AnError)
	mockIPMI.On("RunCommand", ctx, []string{"raw", "0x30", "0x70", "0x66", "0x01", "0x00", "0x32"}).Return([]byte(""), assert.AnError)
	// The Generic driver is selected as the fallback; its Discover() probes "mc info".
	mockIPMI.On("RunCommand", ctx, []string{"mc", "info"}).Return([]byte(""), assert.AnError)

	driver, err := registry.DetectBestDriver(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, driver)
	assert.Equal(t, "Generic", driver.GetVendor())
	
	mockIPMI.AssertExpectations(t)
}
