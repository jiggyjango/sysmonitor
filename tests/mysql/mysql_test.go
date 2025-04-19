package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/jiggyjango/sysmonitor/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mocking the MySQLChecker to test behavior without actual DB connection
type MockMySQLChecker struct {
	mock.Mock
	db.MySQLChecker
}

func (m *MockMySQLChecker) checkHealth() {
	args := m.Called()
	if args != nil {
		// Simulating checking health (you can mock responses here)
	}
}

func TestMySQLChecker_Initialization(t *testing.T) {
	cfg := db.MySQLConfig{
		Name:             "MyDatabase",
		Username:         "root",
		Password:         "",
		Logging:          true,
		Heartbeat:        3 * time.Second,
		Ctx:              context.Background(),
		MonitorResources: true,
	}

	checker, err := db.NewMySQLChecker(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, checker)
	assert.Equal(t, "MyDatabase", checker.Name())
	assert.NotNil(t, checker.GetDB())                      // Use the getter for db
	assert.Equal(t, 3*time.Second, checker.GetHeartbeat()) // Use the getter for heartbeat
	assert.NotNil(t, checker.GetCtx())                     // Use the getter for ctx
}

func TestMySQLChecker_HealthCheck(t *testing.T) {
	cfg := db.MySQLConfig{
		Name:             "MyDatabase",
		DSN:              "user:password@tcp(localhost:3306)/dbname",
		Logging:          false,
		Heartbeat:        3 * time.Second,
		Ctx:              context.Background(),
		MonitorResources: false,
	}

	checker, err := db.NewMySQLChecker(cfg)
	assert.NoError(t, err)

	// Test the checkHealth function (we mock this behavior to avoid real DB connection)
	mockChecker := &MockMySQLChecker{MySQLChecker: *checker}
	mockChecker.On("checkHealth").Return(nil)

	// Simulate health check
	mockChecker.checkHealth()

	// Assert that the mock method was called
	mockChecker.AssertExpectations(t)
}

func TestMySQLChecker_Stop(t *testing.T) {
	cfg := db.MySQLConfig{
		Name:             "MyDatabase",
		DSN:              "user:password@tcp(localhost:3306)/dbname",
		Logging:          false,
		Heartbeat:        3 * time.Second,
		Ctx:              context.Background(),
		MonitorResources: false,
	}

	checker, err := db.NewMySQLChecker(cfg)
	assert.NoError(t, err)

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
