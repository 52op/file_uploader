package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"file_uploader/metrics"
)

// PrometheusMiddleware Prometheus指标收集中间件
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		
		// 增加活跃连接数
		m := metrics.GetMetrics()
		m.ActiveConnections.Inc()
		defer m.ActiveConnections.Dec()
		
		// 处理请求
		c.Next()
		
		// 记录请求指标
		duration := time.Since(start)
		status := strconv.Itoa(c.Writer.Status())
		endpoint := getEndpointPattern(c.FullPath())
		
		m.RecordHTTPRequest(c.Request.Method, endpoint, status, duration)
	}
}

// getEndpointPattern 获取端点模式（用于指标标签）
func getEndpointPattern(fullPath string) string {
	if fullPath == "" {
		return "unknown"
	}
	
	// 将具体的路径参数替换为模式
	switch {
	case fullPath == "/health":
		return "/health"
	case fullPath == "/":
		return "/"
	case fullPath == "/metrics":
		return "/metrics"
	case fullPath == "/api/v1/upload":
		return "/api/v1/upload"
	case fullPath == "/api/v1/files/:filename":
		return "/api/v1/files/:filename"
	case fullPath == "/api/v1/batch/upload":
		return "/api/v1/batch/upload"
	case fullPath == "/api/v1/batch/delete":
		return "/api/v1/batch/delete"
	case fullPath == "/api/v1/batch/info":
		return "/api/v1/batch/info"
	case fullPath == "/api/v1/folders":
		return "/api/v1/folders"
	case fullPath == "/api/v1/folders/:path":
		return "/api/v1/folders/:path"
	case fullPath == "/api/v1/cert/info":
		return "/api/v1/cert/info"
	case fullPath == "/api/v1/cert/obtain":
		return "/api/v1/cert/obtain"
	case fullPath == "/api/v1/cert/renew":
		return "/api/v1/cert/renew"
	case fullPath == "/api/v1/cert/ensure":
		return "/api/v1/cert/ensure"
	case fullPath == "/api/v1/cert/status":
		return "/api/v1/cert/status"
	default:
		// 对于静态文件路径，使用通用模式
		if len(fullPath) > 0 && fullPath[0] == '/' {
			return "/static/*"
		}
		return "unknown"
	}
}
