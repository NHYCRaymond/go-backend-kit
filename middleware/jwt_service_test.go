package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEnhancedJWTService(t *testing.T) {
	tests := []struct {
		name     string
		config   JWTServiceConfig
		expected JWTServiceConfig
	}{
		{
			name: "with default values",
			config: JWTServiceConfig{
				SecretKey: "test-secret",
			},
			expected: JWTServiceConfig{
				SecretKey:             "test-secret",
				Issuer:                "go-backend-kit",
				AccessExpirationMins:  15,
				RefreshExpirationMins: 10080,
			},
		},
		{
			name: "with custom values",
			config: JWTServiceConfig{
				SecretKey:             "custom-secret",
				AccessExpirationMins:  30,
				RefreshExpirationMins: 20160,
				Issuer:                "custom-issuer",
				EnableJTI:             true,
				EnableDeviceTracking:  true,
			},
			expected: JWTServiceConfig{
				SecretKey:             "custom-secret",
				AccessExpirationMins:  30,
				RefreshExpirationMins: 20160,
				Issuer:                "custom-issuer",
				EnableJTI:             true,
				EnableDeviceTracking:  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewEnhancedJWTService(tt.config)
			assert.NotNil(t, service)
			assert.Equal(t, tt.expected.Issuer, service.config.Issuer)
			assert.Equal(t, tt.expected.AccessExpirationMins, service.config.AccessExpirationMins)
			assert.Equal(t, tt.expected.RefreshExpirationMins, service.config.RefreshExpirationMins)
		})
	}
}

func TestGenerateTokenPair(t *testing.T) {
	service := NewEnhancedJWTService(JWTServiceConfig{
		SecretKey:             "test-secret-key",
		AccessExpirationMins:  15,
		RefreshExpirationMins: 10080,
		EnableJTI:             true,
		EnableDeviceTracking:  true,
		EnableSessionTracking: true,
	})

	tests := []struct {
		name      string
		userID    string
		userType  string
		options   []TokenOption
		wantError bool
	}{
		{
			name:      "basic token generation",
			userID:    "user123",
			userType:  "REGULAR",
			wantError: false,
		},
		{
			name:     "with device ID",
			userID:   "user456",
			userType: "ADMIN",
			options: []TokenOption{
				WithDeviceID("device-001"),
			},
			wantError: false,
		},
		{
			name:     "with all options",
			userID:   "user789",
			userType: "REGULAR",
			options: []TokenOption{
				WithDeviceID("device-002"),
				WithSessionID("session-001"),
				WithIP("192.168.1.1"),
				WithUserAgent("Mozilla/5.0"),
				WithExtra(map[string]string{"key": "value"}),
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accessToken, refreshToken, err := service.GenerateTokenPair(tt.userID, tt.userType, tt.options...)
			
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			
			require.NoError(t, err)
			assert.NotEmpty(t, accessToken)
			assert.NotEmpty(t, refreshToken)
			assert.NotEqual(t, accessToken, refreshToken)
		})
	}
}

func TestParseAndValidateTokens(t *testing.T) {
	service := NewEnhancedJWTService(JWTServiceConfig{
		SecretKey:             "test-secret-key",
		AccessExpirationMins:  15,
		RefreshExpirationMins: 10080,
		EnableJTI:             true,
		EnableDeviceTracking:  true,
	})

	userID := "test-user"
	userType := "REGULAR"
	deviceID := "test-device"

	// Generate tokens
	accessToken, refreshToken, err := service.GenerateTokenPair(
		userID, userType,
		WithDeviceID(deviceID),
		WithIP("127.0.0.1"),
	)
	require.NoError(t, err)

	t.Run("parse valid access token", func(t *testing.T) {
		claims, err := service.ParseToken(accessToken)
		require.NoError(t, err)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, userType, claims.UserType)
		assert.Equal(t, AccessTokenType, claims.TokenType)
		assert.Equal(t, deviceID, claims.DeviceID)
		assert.Equal(t, "127.0.0.1", claims.IP)
		assert.NotEmpty(t, claims.JTI)
	})

	t.Run("parse valid refresh token", func(t *testing.T) {
		claims, err := service.ParseToken(refreshToken)
		require.NoError(t, err)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, userType, claims.UserType)
		assert.Equal(t, RefreshTokenType, claims.TokenType)
		assert.Equal(t, deviceID, claims.DeviceID)
	})

	t.Run("validate access token", func(t *testing.T) {
		claims, err := service.ValidateAccessToken(accessToken)
		require.NoError(t, err)
		assert.Equal(t, AccessTokenType, claims.TokenType)
	})

	t.Run("validate refresh token", func(t *testing.T) {
		claims, err := service.ValidateRefreshToken(refreshToken)
		require.NoError(t, err)
		assert.Equal(t, RefreshTokenType, claims.TokenType)
	})

	t.Run("reject wrong token type for access validation", func(t *testing.T) {
		_, err := service.ValidateAccessToken(refreshToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected access token")
	})

	t.Run("reject wrong token type for refresh validation", func(t *testing.T) {
		_, err := service.ValidateRefreshToken(accessToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected refresh token")
	})

	t.Run("reject invalid token", func(t *testing.T) {
		_, err := service.ParseToken("invalid.token.string")
		assert.Error(t, err)
	})

	t.Run("reject token with wrong secret", func(t *testing.T) {
		wrongService := NewEnhancedJWTService(JWTServiceConfig{
			SecretKey: "wrong-secret",
		})
		_, err := wrongService.ParseToken(accessToken)
		assert.Error(t, err)
	})
}

func TestTokenExpiration(t *testing.T) {
	service := NewEnhancedJWTService(JWTServiceConfig{
		SecretKey:             "test-secret-key",
		AccessExpirationMins:  1, // 1 minute for testing
		RefreshExpirationMins: 2, // 2 minutes for testing
	})

	accessToken, refreshToken, err := service.GenerateTokenPair("user123", "REGULAR")
	require.NoError(t, err)

	t.Run("check token remaining time", func(t *testing.T) {
		remaining, err := service.GetTokenRemainingTime(accessToken)
		require.NoError(t, err)
		assert.Greater(t, remaining, time.Duration(0))
		assert.LessOrEqual(t, remaining, time.Minute)
	})

	t.Run("check token not expired", func(t *testing.T) {
		expired := service.IsTokenExpired(accessToken)
		assert.False(t, expired)
	})

	t.Run("check refresh token has longer expiration", func(t *testing.T) {
		accessRemaining, err := service.GetTokenRemainingTime(accessToken)
		require.NoError(t, err)
		
		refreshRemaining, err := service.GetTokenRemainingTime(refreshToken)
		require.NoError(t, err)
		
		assert.Greater(t, refreshRemaining, accessRemaining)
	})
}

func TestTokenWithSessionTracking(t *testing.T) {
	service := NewEnhancedJWTService(JWTServiceConfig{
		SecretKey:             "test-secret-key",
		EnableSessionTracking: true,
	})

	t.Run("auto-generate session ID when enabled", func(t *testing.T) {
		token, _, err := service.GenerateTokenPair("user123", "REGULAR")
		require.NoError(t, err)
		
		claims, err := service.ParseToken(token)
		require.NoError(t, err)
		assert.NotEmpty(t, claims.SessionID)
	})

	t.Run("use provided session ID", func(t *testing.T) {
		sessionID := "custom-session-123"
		token, _, err := service.GenerateTokenPair(
			"user123", "REGULAR",
			WithSessionID(sessionID),
		)
		require.NoError(t, err)
		
		claims, err := service.ParseToken(token)
		require.NoError(t, err)
		assert.Equal(t, sessionID, claims.SessionID)
	})
}

func TestTokenOptions(t *testing.T) {
	service := NewEnhancedJWTService(JWTServiceConfig{
		SecretKey:            "test-secret-key",
		EnableDeviceTracking: true,
		EnableSessionTracking: true,  // Enable to test session ID
	})

	deviceID := "test-device"
	sessionID := "test-session"
	ip := "192.168.1.100"
	userAgent := "TestAgent/1.0"
	extra := map[string]string{
		"app_version": "1.0.0",
		"platform":    "iOS",
	}

	token, _, err := service.GenerateTokenPair(
		"user123", "REGULAR",
		WithDeviceID(deviceID),
		WithSessionID(sessionID),
		WithIP(ip),
		WithUserAgent(userAgent),
		WithExtra(extra),
	)
	require.NoError(t, err)

	claims, err := service.ParseToken(token)
	require.NoError(t, err)

	assert.Equal(t, deviceID, claims.DeviceID)
	assert.Equal(t, sessionID, claims.SessionID)
	assert.Equal(t, ip, claims.IP)
	assert.Equal(t, userAgent, claims.UserAgent)
	assert.Equal(t, extra, claims.Extra)
}

func TestJTIGeneration(t *testing.T) {
	service := NewEnhancedJWTService(JWTServiceConfig{
		SecretKey: "test-secret-key",
		EnableJTI: true,
	})

	// Generate multiple tokens
	tokens := make(map[string]bool)
	for i := 0; i < 10; i++ {
		token, _, err := service.GenerateTokenPair("user123", "REGULAR")
		require.NoError(t, err)
		
		claims, err := service.ParseToken(token)
		require.NoError(t, err)
		
		// Check JTI is unique
		assert.NotEmpty(t, claims.JTI)
		assert.False(t, tokens[claims.JTI], "JTI should be unique")
		tokens[claims.JTI] = true
	}
}

func TestIssuerConfiguration(t *testing.T) {
	customIssuer := "my-custom-app"
	service := NewEnhancedJWTService(JWTServiceConfig{
		SecretKey: "test-secret-key",
		Issuer:    customIssuer,
	})

	token, _, err := service.GenerateTokenPair("user123", "REGULAR")
	require.NoError(t, err)

	claims, err := service.ParseToken(token)
	require.NoError(t, err)
	assert.Equal(t, customIssuer, claims.Issuer)
}

func BenchmarkGenerateTokenPair(b *testing.B) {
	service := NewEnhancedJWTService(JWTServiceConfig{
		SecretKey:             "test-secret-key",
		EnableJTI:             true,
		EnableDeviceTracking:  true,
		EnableSessionTracking: true,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := service.GenerateTokenPair(
			"user123", "REGULAR",
			WithDeviceID("device-001"),
			WithIP("127.0.0.1"),
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseToken(b *testing.B) {
	service := NewEnhancedJWTService(JWTServiceConfig{
		SecretKey: "test-secret-key",
	})

	token, _, _ := service.GenerateTokenPair("user123", "REGULAR")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.ParseToken(token)
		if err != nil {
			b.Fatal(err)
		}
	}
}