package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLChecker struct {
	name             string
	db               *sql.DB
	ctx              context.Context
	cancel           context.CancelFunc
	logging          bool
	heartbeat        time.Duration
	monitorResources bool // Flag for resource monitoring
}

type MySQLConfig struct {
	Name             string
	DSN              string
	Username         string
	Password         string
	Logging          bool
	Heartbeat        time.Duration
	Ctx              context.Context
	MonitorResources bool // Flag for resource monitoring
}

type MySQLStats struct {
	Connections       int
	ActiveConnections int
	Questions         int64
	SlowQueries       int64
	Uptime            int64
	ThreadsRunning    int
	ThreadsConnected  int
}

func NewMySQLChecker(cfg MySQLConfig) (*MySQLChecker, error) {
	// If no context is provided, we create a timeout-limited one
	var baseCtx context.Context
	var baseCancel context.CancelFunc

	if cfg.Ctx == nil {
		baseCtx, baseCancel = context.WithTimeout(context.Background(), 15*time.Second)
	} else {
		baseCtx = cfg.Ctx
	}

	// Wrap the base context in a cancelable one for monitoring control
	ctx, cancel := context.WithCancel(baseCtx)

	// Construct DSN with username and password if provided
	if cfg.DSN == "" {
		if cfg.Username == "" {
			cancel()
			if baseCancel != nil {
				baseCancel() // Clean up the base timeout context
			}
			return nil, fmt.Errorf("username must be provided if DSN is not set")
		}
		cfg.DSN = fmt.Sprintf("%s:%s@tcp(localhost:3306)/", cfg.Username, cfg.Password)
	}

	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		cancel()
		if baseCancel != nil {
			baseCancel() // Clean up the base timeout context
		}
		return nil, err
	}

	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 3 * time.Second
	}

	checker := &MySQLChecker{
		name:             cfg.Name,
		db:               db,
		ctx:              ctx,
		cancel:           cancel,
		logging:          cfg.Logging,
		heartbeat:        cfg.Heartbeat,
		monitorResources: cfg.MonitorResources,
	}

	// Clean up the base context after setting up (so timeout doesn't run forever)
	if baseCancel != nil {
		go func() {
			<-ctx.Done()
			baseCancel()
		}()
	}

	return checker, nil
}

func (m *MySQLChecker) Name() string {
	if err := m.connect(); err != nil {
		return fmt.Sprintf("MySQL [%s] - Error: %v", m.name, err)
	}
	return m.name
}

func (m *MySQLChecker) connect() error {
	select {
	case <-m.ctx.Done():
		return fmt.Errorf("connection attempt canceled: %v", m.ctx.Err())
	default:
		err := m.db.Ping()
		if err != nil {
			for i := 0; i < 3; i++ {
				select {
				case <-m.ctx.Done():
					return fmt.Errorf("connection attempt canceled: %v", m.ctx.Err())
				case <-time.After(2 * time.Second):
					err = m.db.Ping()
					if err == nil {
						return nil
					}
				}
			}
			return fmt.Errorf("unable to connect to MySQL after retries: %v", err)
		}
	}
	return nil
}

func (m *MySQLChecker) Check() (bool, string) {
	if err := m.connect(); err != nil {
		return false, fmt.Sprintf("MySQL connection failed: %v", err)
	}

	select {
	case <-m.ctx.Done():
		return false, fmt.Sprintf("check canceled: %v", m.ctx.Err())
	default:
		err := m.db.Ping()
		if err != nil {
			return false, fmt.Sprintf("MySQL ping failed: %v", err)
		}
	}
	return true, "MySQL OK"
}

func (m *MySQLChecker) checkMySQLResourceUsage() (string, error) {
	if !m.monitorResources {
		return "", nil
	}

	// Get MySQL stats
	stats, err := m.getMySQLStats()
	if err != nil {
		return "", fmt.Errorf("error getting MySQL stats: %v", err)
	}

	// Format stats as a string
	statsInfo := fmt.Sprintf("MySQL Stats: Connections: %d, Active: %d, Threads Running: %d, "+
		"Threads Connected: %d, Questions: %d, Slow Queries: %d, Uptime: %d seconds",
		stats.Connections, stats.ActiveConnections, stats.ThreadsRunning,
		stats.ThreadsConnected, stats.Questions, stats.SlowQueries, stats.Uptime)

	return statsInfo, nil
}

// getMySQLStats fetches statistics from MySQL server
func (m *MySQLChecker) getMySQLStats() (MySQLStats, error) {
	var stats MySQLStats

	// Check context before executing query
	if m.ctx.Err() != nil {
		return stats, m.ctx.Err()
	}

	// Query for global status variables
	rows, err := m.db.QueryContext(m.ctx, "SHOW GLOBAL STATUS")
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	// Process results
	var name, value string
	statusMap := make(map[string]string)
	for rows.Next() {
		if err := rows.Scan(&name, &value); err != nil {
			return stats, err
		}
		statusMap[name] = value
	}

	// Check for errors from iterating over rows
	if err := rows.Err(); err != nil {
		return stats, err
	}

	// Get connection information
	var maxConnections int
	err = m.db.QueryRowContext(m.ctx, "SHOW VARIABLES LIKE 'max_connections'").Scan(&name, &value)
	if err == nil {
		fmt.Sscanf(value, "%d", &maxConnections)
		stats.Connections = maxConnections
	}

	// Parse values from status map
	if v, ok := statusMap["Threads_connected"]; ok {
		fmt.Sscanf(v, "%d", &stats.ThreadsConnected)
		stats.ActiveConnections = stats.ThreadsConnected
	}
	if v, ok := statusMap["Threads_running"]; ok {
		fmt.Sscanf(v, "%d", &stats.ThreadsRunning)
	}
	if v, ok := statusMap["Questions"]; ok {
		fmt.Sscanf(v, "%d", &stats.Questions)
	}
	if v, ok := statusMap["Slow_queries"]; ok {
		fmt.Sscanf(v, "%d", &stats.SlowQueries)
	}
	if v, ok := statusMap["Uptime"]; ok {
		fmt.Sscanf(v, "%d", &stats.Uptime)
	}

	return stats, nil
}

func (m *MySQLChecker) GetDB() *sql.DB {
	return m.db
}

func (m *MySQLChecker) GetHeartbeat() time.Duration {
	return m.heartbeat
}

func (m *MySQLChecker) GetCtx() context.Context {
	return m.ctx
}

// StartMonitoring will start logging at regular intervals (heartbeat)
func (m *MySQLChecker) StartMonitoring() {
	if m.logging {
		ticker := time.NewTicker(m.heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				// Reset context if canceled or expired
				m.resetContext()
				// Continue with the next check
			case <-ticker.C:
				// Check if context is expired before using it
				if m.ctx.Err() != nil {
					m.resetContext()
				}

				status, detail := m.Check()
				fmt.Printf("[MySQL] Status: %v, Detail: %v\n", status, detail)
				if m.monitorResources {
					resourceInfo, err := m.checkMySQLResourceUsage()
					if err != nil {
						fmt.Printf("Error retrieving resources: %v\n", err)
					} else {
						fmt.Printf("Resource Info: %v\n", resourceInfo)
					}
				}
			}
		}
	}
}

// Reset the context when it's canceled or expired
func (m *MySQLChecker) resetContext() {
	// Cancel the current context if it exists
	if m.cancel != nil {
		m.cancel()
	}

	// Always create a fresh context from background
	m.ctx, m.cancel = context.WithTimeout(context.Background(), 15*time.Second)
}

// Stop will cancel the monitoring process
func (m *MySQLChecker) Stop() {
	m.cancel()
}
