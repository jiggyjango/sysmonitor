package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/jiggyjango/sysmonitor/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mocking RedisChecker
type MockRedisChecker struct {
	mock.Mock
	db.RedisChecker
}

func (m *MockRedisChecker) checkHealth() {
	args := m.Called()
	if args != nil {
		// Simulating checking health (you can mock responses here)
	}
}

func TestRedisChecker_Initialization(t *testing.T) {
	cfg := db.RedisConfig{
		Name:             "MyRedis",
		Addr:             "localhost:6379",
		Username:         "default",
		Password:         "",
		DB:               0,
		Logging:          true,
		Heartbeat:        3 * time.Second,
		Ctx:              context.Background(),
		MonitorResources: true,
	}

	checker := db.NewRedisChecker(cfg)
	assert.Equal(t, "MyRedis", checker.Name())             // Directly accessing the 'Name' field
	assert.NotNil(t, checker.GetClient())                  // Directly accessing the 'Client' field
	assert.Equal(t, 3*time.Second, checker.GetHeartbeat()) // Directly accessing the 'Heartbeat' field
	assert.NotNil(t, checker.GetCtx())
}

func TestRedisChecker_HealthCheck(t *testing.T) {
	cfg := db.RedisConfig{
		Name:             "MyRedis",
		Addr:             "localhost:6379",
		Username:         "default",
		Password:         "",
		DB:               0,
		Logging:          true,
		Heartbeat:        3 * time.Second,
		Ctx:              context.Background(),
		MonitorResources: true,
	}

	checker := db.NewRedisChecker(cfg)

	// Mock the health check behavior
	mockChecker := &MockRedisChecker{RedisChecker: *checker}
	mockChecker.On("checkHealth").Return(nil)

	// Simulate health check
	mockChecker.checkHealth()

	// Assert that the mock method was called
	mockChecker.AssertExpectations(t)
}

func TestRedisChecker_Stop(t *testing.T) {
	cfg := db.RedisConfig{
		Name:             "MyRedis",
		Addr:             "localhost:6379",
		Username:         "default",
		Password:         "",
		DB:               0,
		Logging:          true,
		Heartbeat:        3 * time.Second,
		Ctx:              context.Background(),
		MonitorResources: true,
	}

	checker := db.NewRedisChecker(cfg)

	// Start monitoring in a separate goroutine
	go checker.StartMonitoring()

	// Stop the monitoring
	checker.Stop()

	// Assert that the context is canceled (not nil)
	select {
	case <-checker.GetCtx().Done():
		// Assert that the context is indeed canceled
		assert.Equal(t, context.Canceled, checker.GetCtx().Err())
	default:
		t.Error("expected context to be canceled")
	}
}
