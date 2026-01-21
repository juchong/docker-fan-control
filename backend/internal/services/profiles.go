package services

import (
	"context"
	"errors"

	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"

	"gorm.io/gorm"
)

var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrFanNotFound     = errors.New("fan not found")
)

// ProfileService handles profile management
type ProfileService struct {
	logger *EventLogger
}

// NewProfileService creates a new profile service
func NewProfileService(logger *EventLogger) *ProfileService {
	return &ProfileService{logger: logger}
}

// List returns all profiles
func (s *ProfileService) List(ctx context.Context) ([]models.ProfileSummary, error) {
	var profiles []models.Profile
	if err := database.DB.Preload("Fans").Preload("Inputs").Find(&profiles).Error; err != nil {
		return nil, err
	}

	summaries := make([]models.ProfileSummary, len(profiles))
	for i, p := range profiles {
		summaries[i] = models.ProfileSummary{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			Algorithm:   p.Algorithm,
			IsActive:    p.IsActive,
			Zones:       p.Zones,
			ZoneCount:   len(p.Zones),
			FanCount:    len(p.Fans), // Deprecated
			InputCount:  len(p.Inputs),
		}
	}

	return summaries, nil
}

// Get returns a profile by ID
func (s *ProfileService) Get(ctx context.Context, id uint) (*models.Profile, error) {
	var profile models.Profile
	if err := database.DB.Preload("Fans").Preload("Inputs").First(&profile, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProfileNotFound
		}
		return nil, err
	}
	return &profile, nil
}

// Create creates a new profile
func (s *ProfileService) Create(ctx context.Context, req *models.CreateProfileRequest) (*models.Profile, error) {
	profile := &models.Profile{
		Name:            req.Name,
		Description:     req.Description,
		Algorithm:       req.Algorithm,
		AlgorithmParams: req.AlgorithmParams,
		Priority:        req.Priority,
		Zones:           req.Zones,
	}

	// Start transaction
	tx := database.DB.Begin()

	if err := tx.Create(profile).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Associate fans (deprecated - for backward compatibility)
	if len(req.FanIDs) > 0 {
		var fans []models.Fan
		if err := tx.Find(&fans, req.FanIDs).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Model(profile).Association("Fans").Replace(fans); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	// Create inputs
	for _, input := range req.Inputs {
		input.ProfileID = profile.ID
		if err := tx.Create(&input).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// Reload with associations
	return s.Get(ctx, profile.ID)
}

// Update updates a profile
func (s *ProfileService) Update(ctx context.Context, id uint, req *models.UpdateProfileRequest) (*models.Profile, error) {
	var profile models.Profile
	if err := database.DB.First(&profile, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProfileNotFound
		}
		return nil, err
	}

	// Start transaction
	tx := database.DB.Begin()

	// Update fields
	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.Description != nil {
		profile.Description = *req.Description
	}
	if req.Algorithm != nil {
		profile.Algorithm = *req.Algorithm
	}
	if req.AlgorithmParams != nil {
		profile.AlgorithmParams = *req.AlgorithmParams
	}
	if req.Priority != nil {
		profile.Priority = *req.Priority
	}
	if req.Zones != nil {
		profile.Zones = *req.Zones
	}

	if err := tx.Save(&profile).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Update fans
	if req.FanIDs != nil {
		var fans []models.Fan
		if len(*req.FanIDs) > 0 {
			if err := tx.Find(&fans, *req.FanIDs).Error; err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		if err := tx.Model(&profile).Association("Fans").Replace(fans); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	// Update inputs
	if req.Inputs != nil {
		// Delete existing inputs
		if err := tx.Where("profile_id = ?", id).Delete(&models.ProfileInput{}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}

		// Create new inputs
		for _, input := range *req.Inputs {
			input.ProfileID = id
			input.ID = 0 // Reset ID for new creation
			if err := tx.Create(&input).Error; err != nil {
				tx.Rollback()
				return nil, err
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a profile
func (s *ProfileService) Delete(ctx context.Context, id uint) error {
	// Check if profile exists
	var profile models.Profile
	if err := database.DB.First(&profile, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProfileNotFound
		}
		return err
	}

	// Delete inputs
	if err := database.DB.Where("profile_id = ?", id).Delete(&models.ProfileInput{}).Error; err != nil {
		return err
	}

	// Delete fan associations
	if err := database.DB.Model(&profile).Association("Fans").Clear(); err != nil {
		return err
	}

	// Delete profile
	return database.DB.Delete(&profile).Error
}

// FanService handles fan management
type FanService struct {
	logger *EventLogger
}

// NewFanService creates a new fan service
func NewFanService(logger *EventLogger) *FanService {
	return &FanService{logger: logger}
}

// List returns all fans
func (s *FanService) List(ctx context.Context) ([]models.Fan, error) {
	var fans []models.Fan
	if err := database.DB.Find(&fans).Error; err != nil {
		return nil, err
	}
	return fans, nil
}

// Get returns a fan by ID
func (s *FanService) Get(ctx context.Context, id uint) (*models.Fan, error) {
	var fan models.Fan
	if err := database.DB.First(&fan, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFanNotFound
		}
		return nil, err
	}
	return &fan, nil
}

// Update updates a fan
func (s *FanService) Update(ctx context.Context, id uint, req *models.UpdateFanRequest) (*models.Fan, error) {
	var fan models.Fan
	if err := database.DB.First(&fan, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFanNotFound
		}
		return nil, err
	}

	if req.Label != nil {
		fan.Label = *req.Label
	}
	if req.IPMIZone != nil {
		fan.IPMIZone = req.IPMIZone
	}

	if err := database.DB.Save(&fan).Error; err != nil {
		return nil, err
	}

	return &fan, nil
}

// SaveDetectedFans saves detected fans to database
func (s *FanService) SaveDetectedFans(ctx context.Context, detected []models.DetectedFan) ([]models.Fan, error) {
	var savedFans []models.Fan

	for _, d := range detected {
		var fan models.Fan
		err := database.DB.Where("ipmi_sensor_id = ?", d.SensorID).First(&fan).Error
		
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new fan
			fan = models.Fan{
				IPMISensorID: d.SensorID,
				DetectedName: d.Name,
			}
			if err := database.DB.Create(&fan).Error; err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else {
			// Update detected name if changed
			if fan.DetectedName != d.Name {
				fan.DetectedName = d.Name
				database.DB.Save(&fan)
			}
		}

		savedFans = append(savedFans, fan)
	}

	return savedFans, nil
}

// GetFanProfiles returns profile IDs that a fan is assigned to
func (s *FanService) GetFanProfiles(ctx context.Context, fanID uint) ([]uint, error) {
	var profileIDs []uint
	err := database.DB.Table("profile_fans").
		Where("fan_id = ?", fanID).
		Pluck("profile_id", &profileIDs).Error
	return profileIDs, err
}
