package middleware

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})

	return client, mr
}

func createTestClaims(userID, userType string) *EnhancedClaims {
	now := time.Now()
	return &EnhancedClaims{
		UserID:    userID,
		UserType:  userType,
		TokenType: RefreshTokenType,
		JTI:       uuid.New().String(),
		SessionID: uuid.New().String(),
		DeviceID:  "test-device",
		IP:        "127.0.0.1",
		UserAgent: "test-agent",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
		},
	}
}

func TestNewRefreshTokenManager(t *testing.T) {
	client, _ := setupTestRedis(t)

	tests := []struct {
		name           string
		expirationMins int
		enableRotation bool
		expected       int
	}{
		{
			name:           "with default expiration",
			expirationMins: 0,
			enableRotation: false,
			expected:       10080, // Default 7 days
		},
		{
			name:           "with custom expiration",
			expirationMins: 20160, // 14 days
			enableRotation: true,
			expected:       20160,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewRefreshTokenManager(client, tt.expirationMins, tt.enableRotation)
			assert.NotNil(t, manager)
			assert.Equal(t, tt.expected, manager.refreshExpirationMins)
			assert.Equal(t, tt.enableRotation, manager.enableRotation)
			assert.True(t, manager.tokenFamily) // Always enabled by default
		})
	}
}

func TestStoreAndValidateRefreshToken(t *testing.T) {
	client, _ := setupTestRedis(t)
	manager := NewRefreshTokenManager(client, 10080, false)
	ctx := context.Background()

	userID := "user123"
	tokenString := "test.refresh.token"
	claims := createTestClaims(userID, "REGULAR")

	t.Run("store refresh token", func(t *testing.T) {
		err := manager.StoreRefreshToken(ctx, userID, tokenString, claims)
		assert.NoError(t, err)

		// Verify token is stored
		key := fmt.Sprintf("refresh_token:%s", userID)
		storedToken, err := client.Get(ctx, key).Result()
		assert.NoError(t, err)
		assert.Equal(t, tokenString, storedToken)
	})

	t.Run("validate stored token", func(t *testing.T) {
		valid, err := manager.ValidateRefreshToken(ctx, userID, tokenString)
		assert.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("reject invalid token", func(t *testing.T) {
		valid, err := manager.ValidateRefreshToken(ctx, userID, "wrong.token")
		assert.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("reject token for wrong user", func(t *testing.T) {
		valid, err := manager.ValidateRefreshToken(ctx, "wrong-user", tokenString)
		assert.NoError(t, err)
		assert.False(t, valid)
	})
}

func TestTokenRotation(t *testing.T) {
	client, _ := setupTestRedis(t)
	manager := NewRefreshTokenManager(client, 10080, true) // Enable rotation
	ctx := context.Background()

	userID := "user123"
	oldToken := "old.refresh.token"
	newToken := "new.refresh.token"
	oldClaims := createTestClaims(userID, "REGULAR")
	newClaims := createTestClaims(userID, "REGULAR")
	newClaims.SessionID = oldClaims.SessionID // Preserve session ID

	// Store initial token
	err := manager.StoreRefreshToken(ctx, userID, oldToken, oldClaims)
	require.NoError(t, err)

	t.Run("rotate token successfully", func(t *testing.T) {
		err := manager.RotateRefreshToken(ctx, userID, oldToken, newToken, newClaims)
		assert.NoError(t, err)

		// Old token should be invalid
		valid, _ := manager.ValidateRefreshToken(ctx, userID, oldToken)
		assert.False(t, valid)

		// New token should be valid
		valid, _ = manager.ValidateRefreshToken(ctx, userID, newToken)
		assert.True(t, valid)
	})

	t.Run("preserve family ID during rotation", func(t *testing.T) {
		metaKey := fmt.Sprintf("refresh_token:%s:meta", userID)
		familyID, err := client.HGet(ctx, metaKey, "family_id").Result()
		assert.NoError(t, err)
		assert.Equal(t, oldClaims.SessionID, familyID)
	})
}

func TestRevokeRefreshToken(t *testing.T) {
	client, _ := setupTestRedis(t)
	manager := NewRefreshTokenManager(client, 10080, false)
	ctx := context.Background()

	userID := "user123"
	tokenString := "test.refresh.token"
	claims := createTestClaims(userID, "REGULAR")

	// Store token
	err := manager.StoreRefreshToken(ctx, userID, tokenString, claims)
	require.NoError(t, err)

	t.Run("revoke token successfully", func(t *testing.T) {
		err := manager.RevokeRefreshToken(ctx, userID)
		assert.NoError(t, err)

		// Token should no longer be valid
		valid, _ := manager.ValidateRefreshToken(ctx, userID, tokenString)
		assert.False(t, valid)

		// Metadata should be deleted
		metaKey := fmt.Sprintf("refresh_token:%s:meta", userID)
		exists, _ := client.Exists(ctx, metaKey).Result()
		assert.Equal(t, int64(0), exists)
	})
}

func TestRevokeAllUserTokens(t *testing.T) {
	client, _ := setupTestRedis(t)
	manager := NewRefreshTokenManager(client, 10080, false)
	ctx := context.Background()

	userID := "user123"
	
	// Store multiple tokens with different devices
	tokens := []struct {
		token    string
		deviceID string
	}{
		{"token1", "device1"},
		{"token2", "device2"},
		{"token3", "device3"},
	}

	for _, tt := range tokens {
		claims := createTestClaims(userID, "REGULAR")
		claims.DeviceID = tt.deviceID
		err := manager.StoreRefreshToken(ctx, userID, tt.token, claims)
		require.NoError(t, err)
	}

	t.Run("revoke all tokens successfully", func(t *testing.T) {
		err := manager.RevokeAllUserTokens(ctx, userID)
		assert.NoError(t, err)

		// All tokens should be invalid
		for _, tt := range tokens {
			valid, _ := manager.ValidateRefreshToken(ctx, userID, tt.token)
			assert.False(t, valid)
		}

		// Check revocation marker exists
		revocationKey := fmt.Sprintf("refresh_token:revoked:%s", userID)
		exists, _ := client.Exists(ctx, revocationKey).Result()
		assert.Equal(t, int64(1), exists)
	})
}

func TestDeviceTokenManagement(t *testing.T) {
	client, _ := setupTestRedis(t)
	manager := NewRefreshTokenManager(client, 10080, false)
	ctx := context.Background()

	userID := "user123"
	deviceID := "device-001"
	tokenString := "device.refresh.token"
	claims := createTestClaims(userID, "REGULAR")
	claims.DeviceID = deviceID

	t.Run("store token with device ID", func(t *testing.T) {
		err := manager.StoreRefreshToken(ctx, userID, tokenString, claims)
		assert.NoError(t, err)

		// Check device token is stored
		deviceKey := fmt.Sprintf("refresh_token:%s:device:%s", userID, deviceID)
		storedToken, err := client.Get(ctx, deviceKey).Result()
		assert.NoError(t, err)
		assert.Equal(t, tokenString, storedToken)
	})

	t.Run("revoke device tokens", func(t *testing.T) {
		err := manager.RevokeDeviceTokens(ctx, userID, deviceID)
		assert.NoError(t, err)

		// Device token should be deleted
		deviceKey := fmt.Sprintf("refresh_token:%s:device:%s", userID, deviceID)
		exists, _ := client.Exists(ctx, deviceKey).Result()
		assert.Equal(t, int64(0), exists)
	})
}

func TestTokenFamily(t *testing.T) {
	client, _ := setupTestRedis(t)
	manager := NewRefreshTokenManager(client, 10080, true)
	manager.tokenFamily = true
	ctx := context.Background()

	userID := "user123"
	familyID := uuid.New().String()

	t.Run("check valid token family", func(t *testing.T) {
		valid, err := manager.CheckTokenFamily(ctx, userID, familyID)
		assert.NoError(t, err)
		assert.True(t, valid) // Should be valid as it's not revoked
	})

	t.Run("revoke token family", func(t *testing.T) {
		err := manager.RevokeFamilyTokens(ctx, userID, familyID)
		assert.NoError(t, err)

		// Family should now be invalid
		valid, err := manager.CheckTokenFamily(ctx, userID, familyID)
		assert.NoError(t, err)
		assert.False(t, valid)
	})
}

func TestGetActiveTokens(t *testing.T) {
	client, _ := setupTestRedis(t)
	manager := NewRefreshTokenManager(client, 10080, false)
	ctx := context.Background()

	userID := "user123"
	
	// Store multiple tokens
	jtis := []string{"jti1", "jti2", "jti3"}
	for _, jti := range jtis {
		claims := createTestClaims(userID, "REGULAR")
		claims.JTI = jti
		tokenString := fmt.Sprintf("token.%s", jti)
		err := manager.StoreRefreshToken(ctx, userID, tokenString, claims)
		require.NoError(t, err)
	}

	t.Run("get active tokens", func(t *testing.T) {
		tokens, err := manager.GetActiveTokens(ctx, userID)
		assert.NoError(t, err)
		assert.Len(t, tokens, 3)
		
		// Check all JTIs are present
		jtiMap := make(map[string]bool)
		for _, token := range tokens {
			jtiMap[token.JTI] = true
		}
		for _, jti := range jtis {
			assert.True(t, jtiMap[jti])
		}
	})
}

func TestRefreshTokenExpiration(t *testing.T) {
	client, mr := setupTestRedis(t)
	manager := NewRefreshTokenManager(client, 1, false) // 1 minute expiration for testing
	ctx := context.Background()

	userID := "user123"
	tokenString := "expiring.token"
	claims := createTestClaims(userID, "REGULAR")

	t.Run("token expires after TTL", func(t *testing.T) {
		err := manager.StoreRefreshToken(ctx, userID, tokenString, claims)
		assert.NoError(t, err)

		// Token should be valid initially
		valid, _ := manager.ValidateRefreshToken(ctx, userID, tokenString)
		assert.True(t, valid)

		// Fast-forward time in miniredis
		mr.FastForward(2 * time.Minute)

		// Token should be expired
		valid, _ = manager.ValidateRefreshToken(ctx, userID, tokenString)
		assert.False(t, valid)
	})
}

func TestRotationHistory(t *testing.T) {
	client, _ := setupTestRedis(t)
	manager := NewRefreshTokenManager(client, 10080, true)
	ctx := context.Background()

	userID := "user123"
	
	// Perform multiple rotations
	for i := 0; i < 12; i++ {
		oldToken := fmt.Sprintf("token.%d", i)
		newToken := fmt.Sprintf("token.%d", i+1)
		oldClaims := createTestClaims(userID, "REGULAR")
		oldClaims.JTI = fmt.Sprintf("jti.%d", i)
		newClaims := createTestClaims(userID, "REGULAR")
		newClaims.JTI = fmt.Sprintf("jti.%d", i+1)
		
		// Store initial token
		if i == 0 {
			err := manager.StoreRefreshToken(ctx, userID, oldToken, oldClaims)
			require.NoError(t, err)
		}
		
		// Rotate
		err := manager.RotateRefreshToken(ctx, userID, oldToken, newToken, newClaims)
		require.NoError(t, err)
	}

	t.Run("rotation history is limited", func(t *testing.T) {
		historyKey := fmt.Sprintf("refresh_token:history:%s", userID)
		history, err := client.LRange(ctx, historyKey, 0, -1).Result()
		assert.NoError(t, err)
		assert.LessOrEqual(t, len(history), 10) // Should keep only last 10 rotations
	})
}

func BenchmarkStoreRefreshToken(b *testing.B) {
	client, _ := setupTestRedis(&testing.T{})
	manager := NewRefreshTokenManager(client, 10080, false)
	ctx := context.Background()

	claims := createTestClaims("user123", "REGULAR")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userID := fmt.Sprintf("user%d", i)
		tokenString := fmt.Sprintf("token%d", i)
		_ = manager.StoreRefreshToken(ctx, userID, tokenString, claims)
	}
}

func BenchmarkValidateRefreshToken(b *testing.B) {
	client, _ := setupTestRedis(&testing.T{})
	manager := NewRefreshTokenManager(client, 10080, false)
	ctx := context.Background()

	userID := "user123"
	tokenString := "bench.token"
	claims := createTestClaims(userID, "REGULAR")
	_ = manager.StoreRefreshToken(ctx, userID, tokenString, claims)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.ValidateRefreshToken(ctx, userID, tokenString)
	}
}