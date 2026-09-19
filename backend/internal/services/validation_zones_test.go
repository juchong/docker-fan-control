package services

import (
	"context"
	"fmt"
	"strings"
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

// The wrapped profile error must name the failing check: the API returns
// err.Error() as the 400 body, and a bare "profile validation failed" left
// users guessing why a save was rejected.
func TestValidationErrorIncludesDetails(t *testing.T) {
	v := validatorWithZones(1, 2, 3)
	err := v.ValidateProfile(&models.Profile{
		Name:            "x",
		Algorithm:       "linear",
		AlgorithmParams: models.AlgorithmParams{"min_temp": 30.0, "max_temp": 80.0, "min_speed": 30.0, "max_speed": 100.0},
		Zones:           []int{2, 7},
		Inputs:          []models.ProfileInput{{InputType: models.InputTypeCPUTemp, Weight: 0}},
	})
	if err == nil {
		t.Fatal("expected a validation error")
	}
	msg := err.Error()
	for _, want := range []string{"zone 7 does not exist", "weight must be > 0"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should mention %q", msg, want)
		}
	}
}

// A partial update is validated against the stored profile, so a zones-only
// PUT must not fail on the name/algorithm it never sent — but it must still
// reject a resulting profile that keeps a zone this board doesn't have.
func TestValidateProfileUpdateMergesExisting(t *testing.T) {
	v := validatorWithZones(1, 2, 3)
	existing := &models.Profile{
		Name:            "CPU",
		Algorithm:       "linear",
		AlgorithmParams: models.AlgorithmParams{"min_temp": 30.0, "max_temp": 80.0, "min_speed": 30.0, "max_speed": 100.0},
		Zones:           []int{2, 7}, // 7 is stale
		Inputs:          []models.ProfileInput{{InputType: models.InputTypeCPUTemp, Weight: 1}},
	}

	// Re-pointing the zones alone is a valid update.
	zones := []int{1, 2}
	if err := v.ValidateProfileUpdate(existing, &models.UpdateProfileRequest{Zones: &zones}); err != nil {
		t.Errorf("zones-only update should pass: %v", err)
	}
	// An update that leaves the stale zone in place names it and nothing else.
	prio := 1
	err := v.ValidateProfileUpdate(existing, &models.UpdateProfileRequest{Priority: &prio})
	if err == nil {
		t.Fatal("expected the stale zone to be rejected")
	}
	if msg := err.Error(); !strings.Contains(msg, "zone 7") || strings.Contains(msg, "name is required") {
		t.Errorf("unexpected error: %s", msg)
	}
	// The stored profile itself is not mutated by validation.
	if len(existing.Zones) != 2 || existing.Zones[1] != 7 || existing.Priority != 0 {
		t.Errorf("existing profile was mutated: %+v", existing)
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
