# Enhanced JWT Authentication Example

This example demonstrates the enhanced JWT authentication system with advanced features including refresh token rotation, device tracking, session management, and audit logging.

## Features

- **Enhanced JWT Claims**: Includes JTI, device ID, session ID, IP, and user agent
- **Refresh Token Management**: Secure storage in Redis with rotation support
- **Device Tracking**: Track and limit concurrent device sessions
- **Token Family**: Track token chains for enhanced security
- **Audit Logging**: Comprehensive audit trail of all authentication events
- **Token Revocation**: Revoke individual tokens or all user sessions
- **Role-Based Access**: Support for different user roles (REGULAR, ADMIN)

## Prerequisites

- Go 1.21+
- Redis server running on localhost:6379
- Update config.yaml with your settings

## Running the Example

1. Start Redis:
```bash
redis-server
```

2. Run the application:
```bash
go run main.go
```

## API Endpoints

### Public Endpoints

#### Login
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "user",
    "password": "user123",
    "device_id": "device-001"
  }'
```

Response:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "access_token": "eyJhbGc...",
    "refresh_token": "eyJhbGc...",
    "token_type": "Bearer",
    "expires_in": 900,
    "user": {
      "id": "user_regular_001",
      "type": "REGULAR"
    }
  }
}
```

#### Refresh Token
```bash
curl -X POST http://localhost:8080/api/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "eyJhbGc..."
  }'
```

### Protected Endpoints

#### Get Profile
```bash
curl -X GET http://localhost:8080/api/auth/profile \
  -H "Authorization: Bearer eyJhbGc..."
```

#### Logout
```bash
curl -X POST http://localhost:8080/api/auth/logout \
  -H "Authorization: Bearer eyJhbGc..."
```

#### Revoke All Tokens
```bash
curl -X POST http://localhost:8080/api/auth/revoke-all \
  -H "Authorization: Bearer eyJhbGc..."
```

#### Get Active Sessions
```bash
curl -X GET http://localhost:8080/api/auth/sessions \
  -H "Authorization: Bearer eyJhbGc..."
```

### Admin Endpoints

#### Get Audit Logs
```bash
curl -X GET "http://localhost:8080/api/admin/audit/logs?user_id=user_001" \
  -H "Authorization: Bearer eyJhbGc..."
```

#### Revoke User Tokens
```bash
curl -X POST http://localhost:8080/api/admin/users/user_001/revoke \
  -H "Authorization: Bearer eyJhbGc..."
```

## Test Credentials

### Regular User
- Username: `user`
- Password: `user123`

### Admin User
- Username: `admin`
- Password: `admin123`

## Security Features

### 1. Token Rotation
When refresh token rotation is enabled, each refresh generates a new refresh token, invalidating the old one. This prevents replay attacks.

### 2. Device Tracking
The system tracks device IDs and can limit the number of concurrent devices per user.

### 3. Token Family
Tokens are grouped into families (sessions). If a token reuse is detected, the entire family can be revoked.

### 4. Audit Logging
All authentication events are logged with detailed context:
- Login/logout events
- Token refresh/revocation
- Failed authentication attempts
- Admin actions

### 5. Token Blacklisting
Revoked tokens are added to a blacklist in Redis, preventing their use even if they haven't expired.

## Configuration

### JWT Settings
- `access_expiration_mins`: Access token lifetime (default: 15 minutes)
- `refresh_expiration_mins`: Refresh token lifetime (default: 7 days)
- `refresh_token_rotation`: Enable token rotation on refresh
- `max_devices`: Maximum concurrent devices per user
- `enable_jti`: Enable JWT ID tracking
- `enable_device_tracking`: Track device fingerprints
- `enable_session_tracking`: Track user sessions

### Audit Settings
- `enabled`: Enable audit logging
- `log_to_redis`: Store logs in Redis
- `retention_days`: How long to keep audit logs
- `sensitive_fields`: Fields to redact from logs

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│   Auth      │────▶│   Redis     │
└─────────────┘     │  Middleware │     └─────────────┘
                    └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │ JWT Service │
                    └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │   Refresh   │
                    │   Manager   │
                    └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │Audit Logger │
                    └─────────────┘
```

## Best Practices

1. **Always use HTTPS** in production to protect tokens in transit
2. **Rotate secrets regularly** and never commit them to version control
3. **Monitor audit logs** for suspicious activity
4. **Implement rate limiting** on authentication endpoints
5. **Use short-lived access tokens** with longer-lived refresh tokens
6. **Implement device fingerprinting** for enhanced security
7. **Revoke all tokens** when user changes password
8. **Store sensitive data encrypted** in Redis

## Troubleshooting

### Redis Connection Error
Ensure Redis is running:
```bash
redis-cli ping
```

### Token Validation Errors
Check that:
1. The secret key matches between generation and validation
2. The token hasn't expired
3. The token type is correct (access vs refresh)

### Audit Logs Not Appearing
Verify:
1. Audit is enabled in config
2. Redis connection is working
3. Retention period hasn't expired

## Production Considerations

1. **Use environment variables** for sensitive configuration
2. **Implement proper user authentication** against a database
3. **Add monitoring and alerting** for authentication failures
4. **Implement rate limiting** to prevent brute force attacks
5. **Use TLS/SSL** for all connections
6. **Regularly rotate JWT secrets**
7. **Implement proper logging** without exposing sensitive data
8. **Consider using asymmetric keys** (RS256) for JWT signing