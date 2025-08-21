package middleware

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenType defines the type of JWT token
type TokenType string

const (
	AccessTokenType  TokenType = "access"
	RefreshTokenType TokenType = "refresh"
)

// EnhancedClaims represents enhanced JWT claims with additional fields
type EnhancedClaims struct {
	// User identification
	UserID   string `json:"uid"`
	UserType string `json:"user_type"` // REGULAR, ADMIN, TEMP, etc.
	
	// Token metadata
	TokenType TokenType `json:"token_type"`
	JTI       string    `json:"jti"`        // JWT ID for tracking
	DeviceID  string    `json:"device_id"`  // Device fingerprint
	SessionID string    `json:"session_id"` // Session tracking
	
	// Additional context
	IP        string            `json:"ip,omitempty"`
	UserAgent string            `json:"user_agent,omitempty"`
	Extra     map[string]string `json:"extra,omitempty"`
	
	jwt.RegisteredClaims
}

// JWTServiceConfig holds JWT service configuration
type JWTServiceConfig struct {
	SecretKey             string
	AccessExpirationMins  int
	RefreshExpirationMins int
	Issuer                string
	
	// Advanced options
	EnableJTI            bool // Enable JWT ID tracking
	EnableDeviceTracking bool // Enable device fingerprint
	EnableSessionTracking bool // Enable session management
	RefreshTokenRotation bool // Rotate refresh token on use
	MaxDevices           int  // Max concurrent devices (0 = unlimited)
}

// EnhancedJWTService provides advanced JWT operations
type EnhancedJWTService struct {
	config JWTServiceConfig
}

// NewEnhancedJWTService creates a new enhanced JWT service
func NewEnhancedJWTService(config JWTServiceConfig) *EnhancedJWTService {
	if config.Issuer == "" {
		config.Issuer = "go-backend-kit"
	}
	if config.AccessExpirationMins == 0 {
		config.AccessExpirationMins = 15 // Default 15 minutes
	}
	if config.RefreshExpirationMins == 0 {
		config.RefreshExpirationMins = 10080 // Default 7 days
	}
	
	return &EnhancedJWTService{
		config: config,
	}
}

// GenerateTokenPair generates both access and refresh tokens
func (s *EnhancedJWTService) GenerateTokenPair(userID, userType string, options ...TokenOption) (string, string, error) {
	opts := &tokenOptions{}
	for _, opt := range options {
		opt(opts)
	}
	
	accessToken, err := s.generateToken(userID, userType, AccessTokenType, s.config.AccessExpirationMins, opts)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}
	
	refreshToken, err := s.generateToken(userID, userType, RefreshTokenType, s.config.RefreshExpirationMins, opts)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	
	return accessToken, refreshToken, nil
}

// GenerateAccessToken creates a new access token
func (s *EnhancedJWTService) GenerateAccessToken(userID, userType string, options ...TokenOption) (string, error) {
	opts := &tokenOptions{}
	for _, opt := range options {
		opt(opts)
	}
	
	return s.generateToken(userID, userType, AccessTokenType, s.config.AccessExpirationMins, opts)
}

// GenerateRefreshToken creates a new refresh token
func (s *EnhancedJWTService) GenerateRefreshToken(userID, userType string, options ...TokenOption) (string, error) {
	opts := &tokenOptions{}
	for _, opt := range options {
		opt(opts)
	}
	
	return s.generateToken(userID, userType, RefreshTokenType, s.config.RefreshExpirationMins, opts)
}

// generateToken is the internal token generation method
func (s *EnhancedJWTService) generateToken(userID, userType string, tokenType TokenType, expirationMins int, opts *tokenOptions) (string, error) {
	now := time.Now()
	expTime := now.Add(time.Duration(expirationMins) * time.Minute)
	
	claims := EnhancedClaims{
		UserID:    userID,
		UserType:  userType,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(expTime),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	
	// Add JTI if enabled
	if s.config.EnableJTI {
		claims.JTI = uuid.New().String()
	}
	
	// Add device tracking if enabled
	if s.config.EnableDeviceTracking && opts.deviceID != "" {
		claims.DeviceID = opts.deviceID
	}
	
	// Add session tracking if enabled
	if s.config.EnableSessionTracking {
		if opts.sessionID != "" {
			claims.SessionID = opts.sessionID
		} else {
			claims.SessionID = uuid.New().String()
		}
	}
	
	// Add optional context
	if opts.ip != "" {
		claims.IP = opts.ip
	}
	if opts.userAgent != "" {
		claims.UserAgent = opts.userAgent
	}
	if opts.extra != nil {
		claims.Extra = opts.extra
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.config.SecretKey))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}
	
	return tokenString, nil
}

// ParseToken validates and parses a JWT token
func (s *EnhancedJWTService) ParseToken(tokenString string) (*EnhancedClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &EnhancedClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.SecretKey), nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}
	
	if claims, ok := token.Claims.(*EnhancedClaims); ok && token.Valid {
		return claims, nil
	}
	
	return nil, fmt.Errorf("invalid token")
}

// ValidateAccessToken validates an access token
func (s *EnhancedJWTService) ValidateAccessToken(tokenString string) (*EnhancedClaims, error) {
	claims, err := s.ParseToken(tokenString)
	if err != nil {
		return nil, err
	}
	
	if claims.TokenType != AccessTokenType {
		return nil, fmt.Errorf("invalid token type: expected access token, got %s", claims.TokenType)
	}
	
	return claims, nil
}

// ValidateRefreshToken validates a refresh token
func (s *EnhancedJWTService) ValidateRefreshToken(tokenString string) (*EnhancedClaims, error) {
	claims, err := s.ParseToken(tokenString)
	if err != nil {
		return nil, err
	}
	
	if claims.TokenType != RefreshTokenType {
		return nil, fmt.Errorf("invalid token type: expected refresh token, got %s", claims.TokenType)
	}
	
	return claims, nil
}

// GetTokenRemainingTime returns the remaining time before token expires
func (s *EnhancedJWTService) GetTokenRemainingTime(tokenString string) (time.Duration, error) {
	claims, err := s.ParseToken(tokenString)
	if err != nil {
		return 0, err
	}
	
	if claims.ExpiresAt == nil {
		return 0, fmt.Errorf("token has no expiration time")
	}
	
	remaining := time.Until(claims.ExpiresAt.Time)
	if remaining < 0 {
		return 0, fmt.Errorf("token has expired")
	}
	
	return remaining, nil
}

// IsTokenExpired checks if a token is expired
func (s *EnhancedJWTService) IsTokenExpired(tokenString string) bool {
	remaining, err := s.GetTokenRemainingTime(tokenString)
	return err != nil || remaining <= 0
}

// tokenOptions holds optional parameters for token generation
type tokenOptions struct {
	deviceID  string
	sessionID string
	ip        string
	userAgent string
	extra     map[string]string
}

// TokenOption is a function that configures token options
type TokenOption func(*tokenOptions)

// WithDeviceID sets the device ID for the token
func WithDeviceID(deviceID string) TokenOption {
	return func(o *tokenOptions) {
		o.deviceID = deviceID
	}
}

// WithSessionID sets the session ID for the token
func WithSessionID(sessionID string) TokenOption {
	return func(o *tokenOptions) {
		o.sessionID = sessionID
	}
}

// WithIP sets the IP address for the token
func WithIP(ip string) TokenOption {
	return func(o *tokenOptions) {
		o.ip = ip
	}
}

// WithUserAgent sets the user agent for the token
func WithUserAgent(userAgent string) TokenOption {
	return func(o *tokenOptions) {
		o.userAgent = userAgent
	}
}

// WithExtra sets additional data for the token
func WithExtra(extra map[string]string) TokenOption {
	return func(o *tokenOptions) {
		o.extra = extra
	}
}