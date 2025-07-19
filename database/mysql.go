package database

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type MySQLDatabase struct {
	db             *gorm.DB
	config         config.MySQLConfig
	connected      bool
	connectionTime time.Duration
	lastError      string
	queryCount     int64
	errorCount     int64
	mutex          sync.RWMutex
}

func NewMySQL(cfg config.MySQLConfig) *MySQLDatabase {
	return &MySQLDatabase{
		config: cfg,
	}
}

func (m *MySQLDatabase) Connect(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	start := time.Now()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Asia%%2FShanghai&allowNativePasswords=true&tls=false&time_zone=%%27Asia%%2FShanghai%%27",
		m.config.User,
		m.config.Password,
		m.config.Host,
		m.config.Port,
		m.config.DBName,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		m.lastError = err.Error()
		m.errorCount++
		return fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		m.lastError = err.Error()
		m.errorCount++
		return fmt.Errorf("failed to get SQL DB: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		m.lastError = err.Error()
		m.errorCount++
		return fmt.Errorf("failed to ping MySQL: %w", err)
	}

	m.db = db
	m.connected = true
	m.connectionTime = time.Since(start)
	m.lastError = ""

	return nil
}

func (m *MySQLDatabase) Disconnect(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.connected || m.db == nil {
		return nil
	}

	sqlDB, err := m.db.DB()
	if err != nil {
		return err
	}

	if err := sqlDB.Close(); err != nil {
		m.lastError = err.Error()
		m.errorCount++
		return err
	}

	m.connected = false
	m.db = nil
	return nil
}

func (m *MySQLDatabase) HealthCheck(ctx context.Context) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.connected || m.db == nil {
		return fmt.Errorf("MySQL not connected")
	}

	sqlDB, err := m.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get SQL DB: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("MySQL ping failed: %w", err)
	}

	return nil
}

func (m *MySQLDatabase) GetClient() interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.db
}

func (m *MySQLDatabase) Type() DatabaseType {
	return TypeMySQL
}

func (m *MySQLDatabase) Stats() DatabaseStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := DatabaseStats{
		Type:           TypeMySQL,
		Connected:      m.connected,
		ConnectionTime: m.connectionTime,
		TotalQueries:   m.queryCount,
		ErrorCount:     m.errorCount,
		LastError:      m.lastError,
	}

	if m.connected && m.db != nil {
		if sqlDB, err := m.db.DB(); err == nil {
			dbStats := sqlDB.Stats()
			stats.MaxConnections = dbStats.MaxOpenConnections
			stats.ActiveConnections = dbStats.OpenConnections
			stats.IdleConnections = dbStats.Idle
		}
	}

	return stats
}

func (m *MySQLDatabase) IncrementQueryCount() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.queryCount++
}

func (m *MySQLDatabase) IncrementErrorCount() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.errorCount++
}

// Legacy function for backward compatibility
func NewMySQLLegacy(cfg config.MySQLConfig) (*gorm.DB, error) {
	mysqlDB := NewMySQL(cfg)
	if err := mysqlDB.Connect(context.Background()); err != nil {
		return nil, err
	}
	return mysqlDB.GetClient().(*gorm.DB), nil
}

// Legacy health check function
func MySQLHealthCheck(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get SQL DB: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("MySQL ping failed: %w", err)
	}

	return nil
}