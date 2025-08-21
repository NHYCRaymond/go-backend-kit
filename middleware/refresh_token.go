package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// RefreshTokenManager manages refresh token lifecycle
type RefreshTokenManager struct {
	redis                 *redis.Client
	refreshExpirationMins int
	enableRotation        bool
	tokenFamily           bool // Enable token family tracking
}

// NewRefreshTokenManager creates a new refresh token manager
func NewRefreshTokenManager(redis *redis.Client, expirationMins int, enableRotation bool) *RefreshTokenManager {
	if expirationMins == 0 {
		expirationMins = 10080 // Default 7 days
	}
	
	return &RefreshTokenManager{
		redis:                 redis,
		refreshExpirationMins: expirationMins,
		enableRotation:        enableRotation,
		tokenFamily:           true,
	}
}

// StoreRefreshToken stores a refresh token in Redis
func (m *RefreshTokenManager) StoreRefreshToken(ctx context.Context, userID, tokenString string, claims *EnhancedClaims) error {
	key := m.getRefreshTokenKey(userID)
	expiration := time.Duration(m.refreshExpirationMins) * time.Minute
	
	// Store token with metadata
	tokenData := map[string]interface{}{
		"token":      tokenString,
		"jti":        claims.JTI,
		"device_id":  claims.DeviceID,
		"session_id": claims.SessionID,
		"issued_at":  claims.IssuedAt.Unix(),
		"expires_at": claims.ExpiresAt.Unix(),
		"ip":         claims.IP,
		"user_agent": claims.UserAgent,
	}
	
	// If token family is enabled, track the family ID
	if m.tokenFamily && claims.SessionID != "" {
		tokenData["family_id"] = claims.SessionID
	}
	
	// Store main refresh token
	if err := m.redis.Set(ctx, key, tokenString, expiration).Err(); err != nil {
		return fmt.Errorf("failed to store refresh token: %w", err)
	}
	
	// Store token metadata
	metaKey := fmt.Sprintf("%s:meta", key)
	for field, value := range tokenData {
		if err := m.redis.HSet(ctx, metaKey, field, value).Err(); err != nil {
			return fmt.Errorf("failed to store token metadata: %w", err)
		}
	}
	m.redis.Expire(ctx, metaKey, expiration)
	
	// Track active tokens for the user
	activeKey := m.getActiveTokensKey(userID)
	m.redis.SAdd(ctx, activeKey, claims.JTI)
	m.redis.Expire(ctx, activeKey, expiration)
	
	// Track device sessions if device ID is present
	if claims.DeviceID != "" {
		deviceKey := m.getDeviceTokenKey(userID, claims.DeviceID)
		m.redis.Set(ctx, deviceKey, tokenString, expiration)
	}
	
	return nil
}

// ValidateRefreshToken validates a refresh token from Redis
func (m *RefreshTokenManager) ValidateRefreshToken(ctx context.Context, userID, tokenString string) (bool, error) {
	key := m.getRefreshTokenKey(userID)
	
	storedToken, err := m.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil // Token not found
	}
	if err != nil {
		return false, fmt.Errorf("failed to get refresh token: %w", err)
	}
	
	return storedToken == tokenString, nil
}

// RotateRefreshToken rotates a refresh token (if rotation is enabled)
func (m *RefreshTokenManager) RotateRefreshToken(ctx context.Context, userID, oldToken string, newToken string, newClaims *EnhancedClaims) error {
	if !m.enableRotation {
		// If rotation is not enabled, just validate the old token
		valid, err := m.ValidateRefreshToken(ctx, userID, oldToken)
		if err != nil {
			return err
		}
		if !valid {
			return fmt.Errorf("invalid refresh token")
		}
		return nil
	}
	
	// Get old token metadata
	metaKey := fmt.Sprintf("%s:meta", m.getRefreshTokenKey(userID))
	oldMeta, err := m.redis.HGetAll(ctx, metaKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get old token metadata: %w", err)
	}
	
	// Invalidate old token
	if err := m.RevokeRefreshToken(ctx, userID); err != nil {
		return fmt.Errorf("failed to revoke old token: %w", err)
	}
	
	// Store new token with family tracking
	if m.tokenFamily && oldMeta["family_id"] != "" {
		// Preserve the family ID for token chain tracking
		newClaims.SessionID = oldMeta["family_id"]
	}
	
	// Store the new token
	if err := m.StoreRefreshToken(ctx, userID, newToken, newClaims); err != nil {
		return fmt.Errorf("failed to store new token: %w", err)
	}
	
	// Track rotation history
	historyKey := fmt.Sprintf("refresh_token:history:%s", userID)
	historyEntry := fmt.Sprintf("%s->%s@%d", oldMeta["jti"], newClaims.JTI, time.Now().Unix())
	m.redis.LPush(ctx, historyKey, historyEntry)
	m.redis.LTrim(ctx, historyKey, 0, 9) // Keep last 10 rotations
	m.redis.Expire(ctx, historyKey, time.Duration(m.refreshExpirationMins)*time.Minute)
	
	return nil
}

// RevokeRefreshToken revokes a specific refresh token
func (m *RefreshTokenManager) RevokeRefreshToken(ctx context.Context, userID string) error {
	key := m.getRefreshTokenKey(userID)
	metaKey := fmt.Sprintf("%s:meta", key)
	
	// Get token metadata before deletion
	meta, _ := m.redis.HGetAll(ctx, metaKey).Result()
	
	// Delete the token and its metadata
	if err := m.redis.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete refresh token: %w", err)
	}
	
	if err := m.redis.Del(ctx, metaKey).Err(); err != nil {
		return fmt.Errorf("failed to delete token metadata: %w", err)
	}
	
	// Remove from active tokens set
	if jti := meta["jti"]; jti != "" {
		activeKey := m.getActiveTokensKey(userID)
		m.redis.SRem(ctx, activeKey, jti)
	}
	
	// Remove device session if exists
	if deviceID := meta["device_id"]; deviceID != "" {
		deviceKey := m.getDeviceTokenKey(userID, deviceID)
		m.redis.Del(ctx, deviceKey)
	}
	
	return nil
}

// RevokeAllUserTokens revokes all refresh tokens for a user
func (m *RefreshTokenManager) RevokeAllUserTokens(ctx context.Context, userID string) error {
	// Get all active token JTIs
	activeKey := m.getActiveTokensKey(userID)
	_, err := m.redis.SMembers(ctx, activeKey).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get active tokens: %w", err)
	}
	
	// Delete main refresh token
	if err := m.RevokeRefreshToken(ctx, userID); err != nil {
		return err
	}
	
	// Delete all device sessions
	pattern := fmt.Sprintf("refresh_token:%s:device:*", userID)
	iter := m.redis.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		m.redis.Del(ctx, iter.Val())
	}
	
	// Clear active tokens set
	m.redis.Del(ctx, activeKey)
	
	// Add to revocation list with timestamp
	revocationKey := fmt.Sprintf("refresh_token:revoked:%s", userID)
	m.redis.Set(ctx, revocationKey, time.Now().Unix(), time.Duration(m.refreshExpirationMins)*time.Minute)
	
	return nil
}

// RevokeDeviceTokens revokes all tokens for a specific device
func (m *RefreshTokenManager) RevokeDeviceTokens(ctx context.Context, userID, deviceID string) error {
	deviceKey := m.getDeviceTokenKey(userID, deviceID)
	
	// Get the token for this device
	token, err := m.redis.Get(ctx, deviceKey).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get device token: %w", err)
	}
	
	// Delete device token
	m.redis.Del(ctx, deviceKey)
	
	// If this was the active refresh token, revoke it
	mainKey := m.getRefreshTokenKey(userID)
	mainToken, _ := m.redis.Get(ctx, mainKey).Result()
	if mainToken == token {
		return m.RevokeRefreshToken(ctx, userID)
	}
	
	return nil
}

// GetActiveTokens returns information about active refresh tokens for a user
func (m *RefreshTokenManager) GetActiveTokens(ctx context.Context, userID string) ([]TokenInfo, error) {
	activeKey := m.getActiveTokensKey(userID)
	jtis, err := m.redis.SMembers(ctx, activeKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active tokens: %w", err)
	}
	
	var tokens []TokenInfo
	for _, jti := range jtis {
		// Try to get metadata for each JTI
		// Note: This is a simplified approach; in production, you might want to store this differently
		tokens = append(tokens, TokenInfo{
			JTI:       jti,
			IssuedAt:  time.Now(), // This would need to be stored properly
			ExpiresAt: time.Now().Add(time.Duration(m.refreshExpirationMins) * time.Minute),
		})
	}
	
	return tokens, nil
}

// CheckTokenFamily validates if a token belongs to a valid family chain
func (m *RefreshTokenManager) CheckTokenFamily(ctx context.Context, userID, familyID string) (bool, error) {
	if !m.tokenFamily {
		return true, nil // Family tracking not enabled
	}
	
	// Check if the family ID is still valid (not revoked)
	revokedKey := fmt.Sprintf("refresh_token:family:revoked:%s:%s", userID, familyID)
	exists, err := m.redis.Exists(ctx, revokedKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check family revocation: %w", err)
	}
	
	return exists == 0, nil // Valid if not in revoked list
}

// RevokeFamilyTokens revokes all tokens in a family chain
func (m *RefreshTokenManager) RevokeFamilyTokens(ctx context.Context, userID, familyID string) error {
	if !m.tokenFamily {
		return nil // Family tracking not enabled
	}
	
	// Mark family as revoked
	revokedKey := fmt.Sprintf("refresh_token:family:revoked:%s:%s", userID, familyID)
	m.redis.Set(ctx, revokedKey, time.Now().Unix(), time.Duration(m.refreshExpirationMins)*time.Minute)
	
	// The actual token validation will fail when CheckTokenFamily is called
	return nil
}

// Helper methods for key generation
func (m *RefreshTokenManager) getRefreshTokenKey(userID string) string {
	return fmt.Sprintf("refresh_token:%s", userID)
}

func (m *RefreshTokenManager) getActiveTokensKey(userID string) string {
	return fmt.Sprintf("refresh_token:active:%s", userID)
}

func (m *RefreshTokenManager) getDeviceTokenKey(userID, deviceID string) string {
	return fmt.Sprintf("refresh_token:%s:device:%s", userID, deviceID)
}

// TokenInfo holds information about an active token
type TokenInfo struct {
	JTI       string    `json:"jti"`
	DeviceID  string    `json:"device_id,omitempty"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// RefreshTokenResult holds the result of a token refresh operation
type RefreshTokenResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"` // Only if rotation is enabled
	ExpiresIn    int    `json:"expires_in"`              // Seconds until access token expires
}