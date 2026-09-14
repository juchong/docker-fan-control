package services

import (
	"context"
	"fmt"
	"testing"

	"docker-fan-control/internal/models"
)

// fakeZoneDriver is a minimal IPMIDriver for validation tests. BaseDriver
// supplies the metadata + zone layout and defaults CanDetect()->true /
// Discover()->nil, so registering one makes it the active driver.
type fakeZoneDriver struct{ *BaseDriver }

func (f *fakeZoneDriver) DetectFans(context.Context) ([]models.DetectedFan, error) { return nil, nil }
func (f *fakeZoneDriver) GetFanSpeeds(context.Context) (map[string]int, error)     { return nil, nil }
func (f *fakeZoneDriver) SetFanSpeed(context.Context, int, int) error              { return nil }
func (f *fakeZoneDriver) SetAllFanSpeeds(context.Context, int) error               { return nil }
func (f *fakeZoneDriver) SetManualMode(context.Context, bool) error                { return nil }
func (f *fakeZoneDriver) IsManualMode() bool                                       { return false }

func validatorWithZones(ids ...int) *ProfileValidator {
	zones := make([]ZoneDefinition, 0, len(ids))
	for _, id := range ids {
		zones = append(zones, ZoneDefinition{ID: id, Name: fmt.Sprintf("Fan %d", id)})
	}
	d := &fakeZoneDriver{BaseDriver: NewBaseDriver("Fake", "test", DriverCapabilities{}, ZoneLayout{Zones: zones})}
	reg := NewDriverRegistry()
	reg.RegisterDriver(d)
	return &ProfileValidator{driverRegistry: reg}
}

func TestValidateZones(t *testing.T) {
	// hwmon-style zone IDs = PWM channel numbers (not a 0..N-1 range).
	v := validatorWithZones(1, 2, 3, 4, 5, 6, 7)

	if err := v.validateZones(nil); err != nil {
		t.Errorf("empty zones should be valid: %v", err)
	}
	if err := v.validateZones([]int{7, 4, 5, 2, 3, 6, 1}); err != nil {
		t.Errorf("zones within the driver layout should pass: %v", err)
	}
	// Regression: a real driver channel zone must validate purely by membership
	// in the ACTIVE driver's layout — no legacy custom {0,1} layout can override
	// it (that stale-override bug made profile saves fail with a 500).
	if err := v.validateZones([]int{5}); err != nil {
		t.Errorf("driver channel zone 5 should be valid: %v", err)
	}
	if err := v.validateZones([]int{8}); err == nil {
		t.Error("a zone not in the driver layout should be rejected")
	}
}

func TestValidateZonesNoDriver(t *testing.T) {
	v := &ProfileValidator{driverRegistry: NewDriverRegistry()} // no drivers registered

	if err := v.validateZones(nil); err != nil {
		t.Errorf("empty zones need no driver: %v", err)
	}
	if err := v.validateZones([]int{1}); err == nil {
		t.Error("expected an error validating zones with no active driver")
	}
}
