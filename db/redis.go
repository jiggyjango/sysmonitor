package db

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

type RedisChecker struct {
	name             string
	client           *redis.Client
	ctx              context.Context
	cancel           context.CancelFunc
	logging          bool
	heartbeat        time.Duration
	monitorResources bool // Flag for resource monitoring
}

type RedisConfig struct {
	Name             string
	Addr             string
	Username         string // Redis username (for ACL support, Redis 6+)
	Password         string // Redis password
	DB               int
	Logging          bool
	Heartbeat        time.Duration
	Ctx              context.Context
	MonitorResources bool // Flag for resource monitoring
}

func NewRedisChecker(cfg RedisConfig) *RedisChecker {
	var baseCtx context.Context
	var baseCancel context.CancelFunc

	// If no context is provided, create one with timeout (default 15 seconds)
	if cfg.Ctx == nil {
		baseCtx, baseCancel = context.WithTimeout(context.Background(), 15*time.Second)
	} else {
		baseCtx = cfg.Ctx
	}

	// Create a cancelable context for monitoring
	ctx, cancel := context.WithCancel(baseCtx)

	if cfg.Addr == "" {
		cfg.Addr = "localhost:6379" // Default Redis server address
	}

	clientOptions := &redis.Options{
		Addr:     cfg.Addr,
		DB:       cfg.DB,
		Password: cfg.Password, // Password for Redis
	}

	// Redis 6+ username and password support (ACLs)
	if cfg.Username != "" {
		clientOptions.Username = cfg.Username
	}

	client := redis.NewClient(clientOptions)

	// Ensure heartbeat interval is set
	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 3 * time.Second // Default heartbeat interval
	}

	// Create the Redis checker struct
	checker := &RedisChecker{
		name:             cfg.Name,
		client:           client,
		ctx:              ctx,
		cancel:           cancel,
		logging:          cfg.Logging,
		heartbeat:        cfg.Heartbeat,
		monitorResources: cfg.MonitorResources,
	}

	// Cleanup the base context if it's a timeout context
	if baseCancel != nil {
		go func() {
			<-ctx.Done()
			baseCancel() // Clean up the base context once the monitoring stops
		}()
	}

	return checker
}

func (r *RedisChecker) Name() string {
	if err := r.connect(); err != nil {
		return fmt.Sprintf("Redis [%s] - Error: %v", r.name, err)
	}
	return r.name
}

func (r *RedisChecker) connect() error {
	select {
	case <-r.ctx.Done():
		return fmt.Errorf("connection attempt canceled: %v", r.ctx.Err())
	default:
		err := r.client.Ping(r.ctx).Err()
		if err != nil {
			for i := 0; i < 3; i++ {
				select {
				case <-r.ctx.Done():
					return fmt.Errorf("connection attempt canceled: %v", r.ctx.Err())
				case <-time.After(2 * time.Second):
					err = r.client.Ping(r.ctx).Err()
					if err == nil {
						return nil
					}
				}
			}
			return fmt.Errorf("unable to connect to Redis after retries: %v", err)
		}
	}
	return nil
}

func (r *RedisChecker) Check() (bool, string) {
	if err := r.connect(); err != nil {
		return false, fmt.Sprintf("Redis connection failed: %v", err)
	}

	select {
	case <-r.ctx.Done():
		return false, fmt.Sprintf("check canceled: %v", r.ctx.Err())
	default:
		err := r.client.Ping(r.ctx).Err()
		if err != nil {
			return false, fmt.Sprintf("Redis ping failed: %v", err)
		}
	}
	return true, "Redis OK"
}

func (r *RedisChecker) checkRedisResourceUsage() (string, error) {
	if !r.monitorResources {
		return "", nil
	}

	cpuUsage, err := cpu.Percent(0, false)
	if err != nil {
		return "", fmt.Errorf("error getting CPU usage: %v", err)
	}

	memUsage, err := mem.VirtualMemory()
	if err != nil {
		return "", fmt.Errorf("error getting memory usage: %v", err)
	}

	resourceInfo := fmt.Sprintf("CPU Usage: %.2f%%, Memory Usage: %.2f%%, Total Memory: %.2f GB",
		cpuUsage[0], memUsage.UsedPercent, float64(memUsage.Total)/1024/1024/1024)

	return resourceInfo, nil
}

func (r *RedisChecker) GetClient() *redis.Client {
	return r.client
}

func (r *RedisChecker) GetHeartbeat() time.Duration {
	return r.heartbeat
}

func (r *RedisChecker) GetCtx() context.Context {
	return r.ctx
}

// StartMonitoring will start logging at regular intervals (heartbeat)
func (r *RedisChecker) StartMonitoring() {
	if r.logging {
		ticker := time.NewTicker(r.heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-r.ctx.Done():
				// Reset the context if it’s canceled or expired
				r.resetContext()
				// Restart the ticker for the new context
				ticker = time.NewTicker(r.heartbeat)
			case <-ticker.C:
				status, detail := r.Check()
				fmt.Printf("[Redis] Status: %v, Detail: %v\n", status, detail)
				if r.monitorResources {
					resourceInfo, err := r.checkRedisResourceUsage()
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
func (r *RedisChecker) resetContext() {
	// Cancel the current context
	if r.cancel != nil {
		r.cancel()
	}

	// Create a new context with a timeout or custom duration
	var baseCtx context.Context
	var baseCancel context.CancelFunc

	if r.ctx == nil {
		baseCtx, baseCancel = context.WithTimeout(context.Background(), 15*time.Second)
	} else {
		baseCtx = r.ctx
	}

	r.ctx, r.cancel = context.WithCancel(baseCtx)

	// Clean up the base context after setting up (so timeout doesn't run forever)
	if baseCancel != nil {
		go func() {
			<-r.ctx.Done()
			baseCancel()
		}()
	}
}

// Stop will cancel the monitoring process
func (r *RedisChecker) Stop() {
	r.cancel()
}
