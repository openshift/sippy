package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"

	bqclient "github.com/openshift/sippy/pkg/bigquery"
)

// Health status constants
type HealthStatus string

const (
	HealthStatusHealthy     HealthStatus = "healthy"
	HealthStatusDegraded    HealthStatus = "degraded"
	HealthStatusUnhealthy   HealthStatus = "unhealthy"
	HealthStatusUnavailable HealthStatus = "unavailable"
)

// HealthTool implements a simple health check MCP tool
// This serves as an example of the tool pattern
type HealthTool struct {
	*BaseTool
}

// NewHealthTool creates a new health check tool instance
func NewHealthTool(deps *ToolDependencies) *HealthTool {
	return &HealthTool{
		BaseTool: NewBaseTool(deps),
	}
}

// GetDefinition returns the MCP tool definition for the health tool
func (ht *HealthTool) GetDefinition() mcp.Tool {
	return mcp.Tool{
		Name:        "health_check",
		Description: "Performs a health check of Sippy services itself including database, BigQuery, and Redis connectivity.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			// Simplified - no parameters for now
		},
	}
}

// HealthCheckResponse represents the health check response structure
type HealthCheckResponse struct {
	Status    HealthStatus           `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Services  map[string]ServiceInfo `json:"services"`
	Message   string                 `json:"message"`
}

// ServiceInfo represents the status of a service
type ServiceInfo struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
}

// GetHandler returns the request handler for the health tool
func (ht *HealthTool) GetHandler() func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Debug("Handling health_check tool call")

		// Perform health checks
		response := &HealthCheckResponse{
			Timestamp: time.Now(),
			Services:  make(map[string]ServiceInfo),
		}

		// Check database connectivity
		dbStatus := ht.checkDatabaseHealth()
		response.Services["database"] = dbStatus

		// Check BigQuery connectivity
		bqStatus := ht.checkBigQueryHealth(ctx)
		response.Services["bigquery"] = bqStatus

		// Check Redis connectivity
		redisStatus := ht.checkRedisHealth(ctx)
		response.Services["redis"] = redisStatus

		// Determine overall status
		overallHealthy := dbStatus.Status == HealthStatusHealthy && bqStatus.Status == HealthStatusHealthy && redisStatus.Status == HealthStatusHealthy
		if overallHealthy {
			response.Status = HealthStatusHealthy
			response.Message = "All services are operational"
		} else {
			response.Status = HealthStatusDegraded
			response.Message = "Some services are experiencing issues"
		}

		return ht.CreateJSONResponse(response)
	}
}

// checkDatabaseHealth checks if the database connection is healthy
func (ht *HealthTool) checkDatabaseHealth() ServiceInfo {
	if ht.deps.DBClient == nil || ht.deps.DBClient.DB == nil {
		return ServiceInfo{
			Status:  HealthStatusUnavailable,
			Message: "Database client not configured",
		}
	}

	// Try a simple query to test connectivity
	var result int
	if err := ht.deps.DBClient.DB.Raw("SELECT 1").Scan(&result).Error; err != nil {
		return ServiceInfo{
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("Database query failed: %v", err),
		}
	}

	return ServiceInfo{
		Status:  HealthStatusHealthy,
		Message: "Database connection successful",
	}
}

// checkBigQueryHealth checks if the BigQuery client is healthy
func (ht *HealthTool) checkBigQueryHealth(ctx context.Context) ServiceInfo {
	if ht.deps.BigQueryClient == nil {
		return ServiceInfo{
			Status:  HealthStatusUnavailable,
			Message: "BigQuery client not configured",
		}
	}

	// Test BigQuery connectivity with a simple query
	q := ht.deps.BigQueryClient.BQ.Query("SELECT 1 as test_value")
	it, err := bqclient.LoggedRead(ctx, q)
	if err != nil {
		return ServiceInfo{
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("BigQuery query failed: %v", err),
		}
	}

	// Try to read the result
	var result struct {
		TestValue int `bigquery:"test_value"`
	}
	err = it.Next(&result)
	if err != nil {
		return ServiceInfo{
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("BigQuery result read failed: %v", err),
		}
	}

	if result.TestValue != 1 {
		return ServiceInfo{
			Status:  HealthStatusUnhealthy,
			Message: "BigQuery returned unexpected result",
		}
	}

	return ServiceInfo{
		Status:  HealthStatusHealthy,
		Message: "BigQuery connection successful",
	}
}

// checkRedisHealth checks if the Redis cache client is healthy
func (ht *HealthTool) checkRedisHealth(ctx context.Context) ServiceInfo {
	if ht.deps.CacheClient == nil {
		return ServiceInfo{
			Status:  HealthStatusUnavailable,
			Message: "Redis client not configured",
		}
	}

	// Test Redis connectivity by setting and getting a test value
	testKey := "sippy_health_check"
	testValue := []byte("test")

	// Try to set a value
	err := ht.deps.CacheClient.Set(ctx, testKey, testValue, time.Minute)
	if err != nil {
		return ServiceInfo{
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("Redis set operation failed: %v", err),
		}
	}

	// Try to get the value back
	retrievedValue, err := ht.deps.CacheClient.Get(ctx, testKey, time.Minute)
	if err != nil {
		return ServiceInfo{
			Status:  HealthStatusUnhealthy,
			Message: fmt.Sprintf("Redis get operation failed: %v", err),
		}
	}

	if string(retrievedValue) != string(testValue) {
		return ServiceInfo{
			Status:  HealthStatusUnhealthy,
			Message: "Redis returned unexpected value",
		}
	}

	return ServiceInfo{
		Status:  HealthStatusHealthy,
		Message: "Redis connection successful",
	}
}
