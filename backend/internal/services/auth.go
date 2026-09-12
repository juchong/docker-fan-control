package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"docker-fan-control/internal/config"
	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
)

// AuthService handles authentication
type AuthService struct {
	jwtSecret      []byte
	tokenTTL       time.Duration
	sessionTimeout time.Duration
}

// JWTClaims represents JWT claims
type JWTClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// NewAuthService creates a new auth service
func NewAuthService(cfg *config.AuthConfig) *AuthService {
	return &AuthService{
		jwtSecret:      cfg.JWTSecret,
		tokenTTL:       cfg.TokenTTL,
		sessionTimeout: cfg.SessionTimeout,
	}
}

// dummyHash is a VALID 60-char bcrypt hash (cost 10) used to prevent timing
// attacks on user enumeration: bcrypt.CompareHashAndPassword must run the full
// hash for a non-existent user just as it does for a real one. The previous
// value was only 55 chars, so bcrypt returned ErrHashTooShort immediately and
// the timing defense did nothing. This hash is of a random string; it will
// never match a real password.
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

// Login validates credentials and returns a JWT token
func (s *AuthService) Login(ctx context.Context, username, password string) (*models.LoginResponse, error) {
	var user models.User
	userNotFound := false

	if err := database.DB.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			userNotFound = true
			// Don't return yet - perform dummy bcrypt comparison to prevent timing attacks
		} else {
			return nil, err
		}
	}

	// Always perform bcrypt comparison to prevent timing-based user enumeration
	hashToCompare := []byte(user.PasswordHash)
	if userNotFound {
		hashToCompare = dummyHash
	}

	if err := bcrypt.CompareHashAndPassword(hashToCompare, []byte(password)); err != nil || userNotFound {
		return nil, ErrInvalidCredentials
	}

	// Update last login
	now := time.Now()
	database.DB.Model(&user).Update("last_login", now)

	token, err := s.generateToken(&user)
	if err != nil {
		return nil, err
	}

	return &models.LoginResponse{
		Token: token,
		User:  user.ToResponse(),
	}, nil
}

// ValidateToken verifies a JWT token and returns the user
func (s *AuthService) ValidateToken(tokenStr string) (*models.User, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		// Validate signing algorithm to prevent algorithm confusion attacks
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	var user models.User
	if err := database.DB.First(&user, claims.UserID).Error; err != nil {
		return nil, ErrUserNotFound
	}

	return &user, nil
}

// CreateUser creates a new user
func (s *AuthService) CreateUser(ctx context.Context, username, password, role string) (*models.User, error) {
	// Check if user exists
	var existing models.User
	if err := database.DB.Where("username = ?", username).First(&existing).Error; err == nil {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
	}

	if err := database.DB.Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

// GetOrCreateProxyUser gets or creates a user from proxy auth
func (s *AuthService) GetOrCreateProxyUser(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := database.DB.Where("username = ?", username).First(&user).Error
	if err == nil {
		// Update last login
		now := time.Now()
		database.DB.Model(&user).Update("last_login", now)
		return &user, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Create new user with cryptographically secure random password
	// (they authenticate via proxy, so this password is never used)
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, err
	}
	randomPass := base64.StdEncoding.EncodeToString(randomBytes)
	hash, err := bcrypt.GenerateFromPassword([]byte(randomPass), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user = models.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         "user",
	}

	if err := database.DB.Create(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// ChangePassword changes a user's password
func (s *AuthService) ChangePassword(ctx context.Context, userID uint, currentPassword, newPassword string) error {
	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return database.DB.Model(&user).Update("password_hash", string(hash)).Error
}

// UpdateUser updates a user's role or password (admin only)
func (s *AuthService) UpdateUser(ctx context.Context, userID uint, req *models.UpdateUserRequest) (*models.User, error) {
	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return nil, ErrUserNotFound
	}

	if req.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		user.PasswordHash = string(hash)
	}

	if req.Role != nil {
		user.Role = *req.Role
	}

	if err := database.DB.Save(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// DeleteUser deletes a user
func (s *AuthService) DeleteUser(ctx context.Context, userID uint) error {
	return database.DB.Delete(&models.User{}, userID).Error
}

// ListUsers returns all users
func (s *AuthService) ListUsers(ctx context.Context) ([]models.UserResponse, error) {
	var users []models.User
	if err := database.DB.Find(&users).Error; err != nil {
		return nil, err
	}

	result := make([]models.UserResponse, len(users))
	for i, u := range users {
		result[i] = u.ToResponse()
	}
	return result, nil
}

// GetUserByID returns a user by ID
func (s *AuthService) GetUserByID(ctx context.Context, userID uint) (*models.User, error) {
	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return nil, ErrUserNotFound
	}
	return &user, nil
}

// GetJWTSecret returns the JWT secret
func (s *AuthService) GetJWTSecret() []byte {
	return s.jwtSecret
}

// EnsureDefaultAdmin creates the default admin user if no users exist,
// or resets the admin password if resetPassword is true
func (s *AuthService) EnsureDefaultAdmin(username, password string, resetPassword bool) error {
	var count int64
	database.DB.Model(&models.User{}).Count(&count)
	
	if count == 0 {
		// No users exist, create the default admin
		if password == "" {
			password = "admin" // Default password, should be changed
		}
		_, err := s.CreateUser(context.Background(), username, password, "admin")
		return err
	}

	// Users exist - check if we should reset the admin password
	if resetPassword && password != "" {
		var user models.User
		if err := database.DB.Where("username = ?", username).First(&user).Error; err != nil {
			// Default admin user doesn't exist, nothing to update
			return nil
		}

		// Update the password
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		return database.DB.Model(&user).Update("password_hash", string(hash)).Error
	}

	return nil
}

// generateToken generates a JWT token for a user
func (s *AuthService) generateToken(user *models.User) (string, error) {
	claims := &JWTClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// RefreshToken generates a new token for a user
func (s *AuthService) RefreshToken(user *models.User) (string, error) {
	return s.generateToken(user)
}
