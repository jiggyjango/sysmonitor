package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/jiggyjango/sysmonitor/db"
)

// MonitorConfig contains the configuration for monitoring multiple services.
type MonitorConfig struct {
	MySQLConfig  db.MySQLConfig
	RedisConfig  db.RedisConfig
	MonitorCtx   context.Context
	MonitorLimit time.Duration // Optional duration to control how long monitoring runs
}

// StartMonitoring initializes MySQL and Redis checkers and starts their monitoring.
func StartMonitoring(cfg MonitorConfig) error {
	// Create MySQL Checker
	mysqlChecker, err := db.NewMySQLChecker(cfg.MySQLConfig)
	if err != nil {
		return fmt.Errorf("error creating MySQL checker: %v", err)
	}

	// Create Redis Checker
	redisChecker := db.NewRedisChecker(cfg.RedisConfig)

	// Start monitoring MySQL and Redis in separate goroutines
	go func() {
		mysqlChecker.StartMonitoring()
	}()

	go func() {
		redisChecker.StartMonitoring()
	}()

	// Optionally, run for a limited time, controlled by MonitorLimit
	if cfg.MonitorLimit > 0 {
		select {
		case <-cfg.MonitorCtx.Done():
			// Context done (cancelled)
			return nil
		case <-time.After(cfg.MonitorLimit):
			// Monitor duration exceeded, stop the monitoring
			mysqlChecker.Stop()
			redisChecker.Stop()
			return nil
		}
	}

	// Keep running indefinitely until context is cancelled or program is terminated
	<-cfg.MonitorCtx.Done()
	mysqlChecker.Stop()
	redisChecker.Stop()

	return nil
}
