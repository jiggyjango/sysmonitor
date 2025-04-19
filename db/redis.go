package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
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

type RedisStats struct {
	ConnectedClients      int64
	UsedMemory            int64
	UsedMemoryHuman       string
	TotalKeys             int64
	TotalConnections      int64
	OpsPerSecond          int64
	KeyspaceHits          int64
	KeyspaceMisses        int64
	KeyspaceHitRatio      float64
	UptimeInSeconds       int64
	RejectedConnections   int64
	ReplBacklogSize       int64
	UsedCpuSys            float64
	UsedCpuUser           float64
	MaxMemory             int64
	MaxMemoryHuman        string
	MemFragmentationRatio float64
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

	// Get Redis stats
	stats, err := r.getRedisStats()
	if err != nil {
		return "", fmt.Errorf("error getting Redis stats: %v", err)
	}

	// Format stats as a string
	statsInfo := fmt.Sprintf("Redis Stats: Clients: %d, Memory: %s, Keys: %d, "+
		"Ops/sec: %d, Hit ratio: %.2f%%, Uptime: %d sec, CPU (sys/user): %.2f/%.2f",
		stats.ConnectedClients, stats.UsedMemoryHuman, stats.TotalKeys,
		stats.OpsPerSecond, stats.KeyspaceHitRatio, stats.UptimeInSeconds,
		stats.UsedCpuSys, stats.UsedCpuUser)

	// Add memory details if max memory is set
	if stats.MaxMemory > 0 {
		memPercent := float64(stats.UsedMemory) / float64(stats.MaxMemory) * 100
		statsInfo += fmt.Sprintf(", Mem usage: %.2f%% of %s", memPercent, stats.MaxMemoryHuman)
	}

	// Add fragmentation details
	if stats.MemFragmentationRatio > 0 {
		statsInfo += fmt.Sprintf(", Frag ratio: %.2f", stats.MemFragmentationRatio)
	}

	return statsInfo, nil
}

// getRedisStats fetches statistics from the Redis server using INFO command
func (r *RedisChecker) getRedisStats() (RedisStats, error) {
	var stats RedisStats

	// Check context before executing query
	if r.ctx.Err() != nil {
		return stats, r.ctx.Err()
	}

	// Get Redis INFO
	info, err := r.client.Info(r.ctx).Result()
	if err != nil {
		return stats, fmt.Errorf("failed to get Redis INFO: %v", err)
	}

	// Parse the INFO output
	sections := strings.Split(info, "\r\n")
	infoMap := make(map[string]string)

	for _, line := range sections {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			infoMap[parts[0]] = parts[1]
		}
	}

	// Extract the statistics we want
	if val, ok := infoMap["connected_clients"]; ok {
		stats.ConnectedClients, _ = strconv.ParseInt(val, 10, 64)
	}

	if val, ok := infoMap["used_memory"]; ok {
		stats.UsedMemory, _ = strconv.ParseInt(val, 10, 64)
		stats.UsedMemoryHuman = formatBytes(stats.UsedMemory)
	}

	if val, ok := infoMap["total_connections_received"]; ok {
		stats.TotalConnections, _ = strconv.ParseInt(val, 10, 64)
	}

	if val, ok := infoMap["instantaneous_ops_per_sec"]; ok {
		stats.OpsPerSecond, _ = strconv.ParseInt(val, 10, 64)
	}

	if val, ok := infoMap["keyspace_hits"]; ok {
		stats.KeyspaceHits, _ = strconv.ParseInt(val, 10, 64)
	}

	if val, ok := infoMap["keyspace_misses"]; ok {
		stats.KeyspaceMisses, _ = strconv.ParseInt(val, 10, 64)
	}

	// Calculate hit ratio if we have both hits and misses
	totalLookups := stats.KeyspaceHits + stats.KeyspaceMisses
	if totalLookups > 0 {
		stats.KeyspaceHitRatio = float64(stats.KeyspaceHits) / float64(totalLookups) * 100
	}

	if val, ok := infoMap["uptime_in_seconds"]; ok {
		stats.UptimeInSeconds, _ = strconv.ParseInt(val, 10, 64)
	}

	if val, ok := infoMap["rejected_connections"]; ok {
		stats.RejectedConnections, _ = strconv.ParseInt(val, 10, 64)
	}

	if val, ok := infoMap["used_cpu_sys"]; ok {
		stats.UsedCpuSys, _ = strconv.ParseFloat(val, 64)
	}

	if val, ok := infoMap["used_cpu_user"]; ok {
		stats.UsedCpuUser, _ = strconv.ParseFloat(val, 64)
	}

	if val, ok := infoMap["maxmemory"]; ok {
		stats.MaxMemory, _ = strconv.ParseInt(val, 10, 64)
		stats.MaxMemoryHuman = formatBytes(stats.MaxMemory)
	}

	if val, ok := infoMap["mem_fragmentation_ratio"]; ok {
		stats.MemFragmentationRatio, _ = strconv.ParseFloat(val, 64)
	}

	// Count total keys by iterating over keyspace info
	stats.TotalKeys = 0
	for key, val := range infoMap {
		if strings.HasPrefix(key, "db") { // db0, db1, etc.
			parts := strings.Split(val, ",")
			for _, part := range parts {
				if strings.HasPrefix(part, "keys=") {
					keyCount, _ := strconv.ParseInt(strings.TrimPrefix(part, "keys="), 10, 64)
					stats.TotalKeys += keyCount
				}
			}
		}
	}

	return stats, nil
}

// formatBytes converts bytes to human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
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

			case <-ticker.C:
				// Check if context is expired before using it
				if r.ctx.Err() != nil {
					r.resetContext()
				}

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

	r.ctx, r.cancel = context.WithTimeout(context.Background(), 15*time.Second)
}

// Stop will cancel the monitoring process
func (r *RedisChecker) Stop() {
	r.cancel()
}
