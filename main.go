package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"file_uploader/acme"
	"file_uploader/config"
	"file_uploader/handlers"
	"file_uploader/metrics"
	"file_uploader/middleware"
	"file_uploader/stats"
	"file_uploader/storage"
)

//go:embed templates/*
var templatesFS embed.FS

var (
	logDir     = flag.String("log-dir", "", "日志文件输出目录，如果为空则只输出到控制台")
	configPath = flag.String("config", "config/config.yaml", "配置文件路径")
	logFile    *os.File
	currentLogDate string
)

// setupLogger 设置日志输出（第一阶段：基本设置）
func setupLogger() error {
	if *logDir == "" {
		// 只输出到控制台
		log.SetOutput(os.Stdout)
		return nil
	}

	// 创建日志目录
	if err := os.MkdirAll(*logDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 设置日志文件
	if err := rotateLogFile(); err != nil {
		return err
	}

	return nil
}

// startLogRotationChecker 启动日志轮转检查（第二阶段：配置加载后）
func startLogRotationChecker(cfg *config.Config) {
	if *logDir == "" {
		return // 如果没有设置日志目录，不需要轮转检查
	}
	go logRotationChecker(cfg)
}

// rotateLogFile 轮转日志文件
func rotateLogFile() error {
	today := time.Now().Format("2006-01-02")

	// 如果是同一天，不需要轮转
	if currentLogDate == today && logFile != nil {
		return nil
	}

	// 关闭旧的日志文件
	if logFile != nil {
		logFile.Close()
	}

	// 创建新的日志文件
	logFileName := filepath.Join(*logDir, fmt.Sprintf("%s.log", today))
	var err error
	logFile, err = os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %v", err)
	}

	// 设置日志输出到文件和控制台
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)

	currentLogDate = today
	log.Printf("日志文件已轮转到: %s", logFileName)

	return nil
}

// logRotationChecker 定期检查是否需要轮转日志
func logRotationChecker(cfg *config.Config) {
	// 解析日志轮转间隔配置
	interval := 1 * time.Hour // 默认值
	if cfg.Network.LogRotationInterval != "" {
		if parsedInterval, err := time.ParseDuration(cfg.Network.LogRotationInterval); err == nil {
			interval = parsedInterval
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := rotateLogFile(); err != nil {
			log.Printf("日志轮转失败: %v", err)
		}
	}
}

func main() {
	// 解析命令行参数
	flag.Parse()

	// 设置日志输出
	if err := setupLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "设置日志失败: %v\n", err)
		os.Exit(1)
	}

	// 确保程序退出时关闭日志文件
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	if *logDir != "" {
		log.Printf("日志输出目录: %s", *logDir)
	} else {
		log.Println("日志仅输出到控制台")
	}

	// 初始化配置热重载器
	var hotReloader *config.HotReloader
	var err error
	hotReloader, err = config.NewHotReloader(*configPath)
	if err != nil {
		log.Fatalf("初始化配置热重载器失败: %v", err)
	}

	// 获取初始配置
	cfg := hotReloader.GetConfig()
	config.SetGlobalConfig(cfg)
	log.Printf("使用配置文件: %s", *configPath)

	// 启动配置热重载
	if err := hotReloader.Start(); err != nil {
		log.Printf("启动配置热重载失败: %v", err)
	} else {
		log.Printf("配置文件热重载已启用")
	}

	// 启动日志轮转检查（配置加载后）
	startLogRotationChecker(cfg)

	// 检查并生成默认防盗链图片
	ensureAntiHotlinkImage(cfg)

	// 初始化Prometheus指标
	_ = metrics.GetMetrics()
	log.Printf("Prometheus指标初始化完成")

	// 初始化统计收集器
	statsCollector := stats.GetStats()
	log.Printf("统计收集器初始化完成")

	// 初始化存储管理器
	storageManager, err := storage.NewStorageManager(cfg)
	if err != nil {
		log.Fatalf("初始化存储管理器失败: %v", err)
	}
	log.Printf("存储管理器初始化完成，可用存储: %v", storageManager.GetAvailableStorages())

	// 启动定期统计更新（每30秒更新一次）
	statsCollector.StartPeriodicUpdate(storageManager, cfg, 30*time.Second)
	log.Printf("统计数据定期更新已启动")

	// 添加配置变更回调
	hotReloader.AddCallback(func(newConfig *config.Config) {
		log.Printf("检测到配置变更，正在更新相关组件...")

		// 更新存储管理器
		newStorageManager, err := storage.NewStorageManager(newConfig)
		if err != nil {
			log.Printf("更新存储管理器失败: %v", err)
		} else {
			// 这里可以考虑优雅地替换存储管理器
			// 为了简化，我们只记录日志
			log.Printf("存储管理器配置已更新，可用存储: %v", newStorageManager.GetAvailableStorages())
		}

		// 更新统计收集器的配置引用
		statsCollector.UpdateStorageStats(storageManager, newConfig)
		log.Printf("配置变更处理完成")
	})

	// 初始化ACME证书管理器（如果启用）
	var acmeManager *acme.Manager
	if cfg.Server.HTTPS.Enabled && cfg.Server.HTTPS.ACME.Enabled {
		var err error
		acmeManager, err = acme.NewManager(&cfg.Server.HTTPS.ACME, &cfg.Network)
		if err != nil {
			log.Printf("初始化ACME管理器失败: %v", err)
			log.Printf("将使用手动证书模式")
		} else {
			log.Printf("ACME证书管理器初始化成功")

			// 确保证书可用
			if err := acmeManager.EnsureCertificate(); err != nil {
				log.Printf("确保ACME证书可用失败: %v", err)
			} else {
				// 启动自动续期
				acmeManager.StartAutoRenewal()

				// 更新证书路径配置
				certPath, keyPath := acmeManager.GetCertificatePaths()
				cfg.Server.HTTPS.CertFile = certPath
				cfg.Server.HTTPS.KeyFile = keyPath
				log.Printf("ACME证书路径已更新: cert=%s, key=%s", certPath, keyPath)
			}
		}
	}

	// 创建处理器实例
	uploadHandler := handlers.NewUploadHandler(storageManager, cfg)
	batchHandler := handlers.NewBatchHandler(storageManager, cfg)

	// 创建证书管理处理器（如果ACME启用）
	var certHandler *handlers.CertHandler
	if acmeManager != nil {
		certHandler = handlers.NewCertHandler(acmeManager)
	}

	// 设置Gin模式
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建Gin路由器
	router := gin.New()

	// 加载嵌入的HTML模板
	tmpl := template.Must(template.New("").ParseFS(templatesFS, "templates/*"))
	router.SetHTMLTemplate(tmpl)

	// 添加中间件
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(middleware.PrometheusMiddleware())

	// 健康检查端点（不需要签名验证）
	router.GET("/health", uploadHandler.HealthCheck)

	// Prometheus指标端点
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// 首页统计页面
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"title": "File Uploader Statistics",
		})
	})

	// 统计数据API端点
	router.GET("/api/stats", func(c *gin.Context) {
		summary := statsCollector.GetSummary()
		c.JSON(http.StatusOK, summary)
	})

	// 详细统计数据API端点（调试用）
	router.GET("/api/stats/detailed", func(c *gin.Context) {
		// 更新统计数据
		statsCollector.UpdateStorageStats(storageManager, cfg)

		// 获取详细统计
		detailedStats, err := statsCollector.GetJSON()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "获取详细统计失败",
				"details": err.Error(),
			})
			return
		}

		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, string(detailedStats))
	})

	// 配置管理API端点
	configAPI := router.Group("/api/v1/config")
	configAPI.Use(middleware.SignatureAuth())
	{
		// 获取当前配置信息（隐藏敏感信息）
		configAPI.GET("/info", func(c *gin.Context) {
			currentConfig := hotReloader.GetConfig()
			safeConfig := map[string]interface{}{
				"server": map[string]interface{}{
					"port": currentConfig.Server.Port,
					"host": currentConfig.Server.Host,
					"https_enabled": currentConfig.Server.HTTPS.Enabled,
				},
				"storage": map[string]interface{}{
					"type": currentConfig.Storage.Type,
					"enabled_storages": currentConfig.Storage.EnabledStorages,
				},
				"security": map[string]interface{}{
					"signature_expiry": currentConfig.Security.SignatureExpiry,
				},
				"hot_reload": map[string]interface{}{
					"enabled": hotReloader.IsRunning(),
				},
			}
			c.JSON(http.StatusOK, safeConfig)
		})

		// 手动重载配置
		configAPI.POST("/reload", func(c *gin.Context) {
			// 这里我们通过重新创建热重载器来触发重载
			newReloader, err := config.NewHotReloader(*configPath)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error": "重新加载配置失败",
					"details": err.Error(),
				})
				return
			}

			// 更新全局配置
			newConfig := newReloader.GetConfig()
			config.SetGlobalConfig(newConfig)

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "配置重新加载成功",
				"timestamp": time.Now().Unix(),
			})
		})
	}

	// API路由组（需要签名验证）
	api := router.Group("/api/v1")
	api.Use(middleware.SignatureAuth())
	{
		// 文件上传
		api.POST("/upload", uploadHandler.UploadFile)

		// 文件删除（使用*filepath匹配包含路径的文件名）
		api.DELETE("/files/*filepath", uploadHandler.DeleteFile)
		log.Printf("注册删除文件路由: DELETE /api/v1/files/*filepath")

		// 批量操作API
		batch := api.Group("/batch")
		{
			// 批量文件上传
			batch.POST("/upload", batchHandler.BatchUpload)

			// 批量文件删除
			batch.POST("/delete", batchHandler.BatchDelete)

			// 批量文件信息查询
			batch.POST("/info", batchHandler.BatchInfo)
		}

		// 文件夹操作API
		folders := api.Group("/folders")
		{
			// 创建文件夹
			folders.POST("", batchHandler.CreateFolder)

			// 列出文件夹内容
			folders.POST("/list", batchHandler.ListFolder)

			// 删除文件夹
			folders.DELETE("/*path", batchHandler.DeleteFolder)
		}

		// 获取文件信息（使用*filepath匹配包含路径的文件名）
		api.GET("/files/*filepath", uploadHandler.GetFileInfo)
	}

	// 添加静态文件服务
	addStaticFileServices(router, storageManager, cfg)

	// 添加证书管理端点（如果ACME启用）
	if certHandler != nil {
		cert := router.Group("/api/v1/cert")
		cert.Use(middleware.SignatureAuth())
		{
			cert.GET("/info", certHandler.GetCertInfo)           // 获取证书信息
			cert.POST("/obtain", certHandler.ObtainCertificate)  // 申请证书
			cert.POST("/renew", certHandler.RenewCertificate)    // 续期证书
			cert.POST("/ensure", certHandler.EnsureCertificate)  // 确保证书可用
			cert.GET("/status", certHandler.GetACMEStatus)       // 获取ACME状态
		}
	}

	// 添加调试端点（仅在开发模式下）
	if gin.Mode() == gin.DebugMode {
		debug := router.Group("/debug")
		{
			// 签名信息查看
			debug.GET("/signature", func(c *gin.Context) {
				info := middleware.GetSignatureInfo(c)
				c.JSON(http.StatusOK, info)
			})

			// 签名生成辅助端点
			debug.GET("/generate-signature", func(c *gin.Context) {
				path := c.Query("path")
				if path == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "path参数不能为空"})
					return
				}

				signature, expires, err := middleware.GenerateSignature(path, time.Hour, cfg.Security.SecretKey)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}

				c.JSON(http.StatusOK, gin.H{
					"path": path,
					"expires": expires,
					"signature": signature,
					"url": fmt.Sprintf("%s?expires=%d&signature=%s", path, expires, signature),
				})
			})

			// 配置信息查看（隐藏敏感信息）
			debug.GET("/config", func(c *gin.Context) {
				safeCfg := map[string]interface{}{
					"server": cfg.Server,
					"storage": map[string]interface{}{
						"type": cfg.Storage.Type,
					},
					"security": map[string]interface{}{
						"signature_expiry": cfg.Security.SignatureExpiry,
					},
				}
				c.JSON(http.StatusOK, safeCfg)
			})

			// ACME状态查看（如果启用）
			if certHandler != nil {
				debug.GET("/acme", certHandler.GetACMEStatus)
			}
		}
	}

	log.Printf("存储类型: %s", cfg.Storage.Type)

	// 打印API端点信息
	printAPIEndpoints(cfg)

	// 启动服务器
	startServer(router, cfg, hotReloader)
}

// startServer 启动服务器（支持HTTP和HTTPS）
func startServer(router *gin.Engine, cfg *config.Config, hotReloader *config.HotReloader) {
	// 创建HTTP服务器
	httpAddr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: router,
	}

	// 如果启用HTTPS，创建HTTPS服务器
	var httpsServer *http.Server
	if cfg.Server.HTTPS.Enabled {
		httpsAddr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.HTTPS.Port)
		httpsServer = &http.Server{
			Addr:    httpsAddr,
			Handler: router,
		}
	}

	// 启动HTTP服务器
	go func() {
		log.Printf("HTTP服务器启动在 %s", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP服务器启动失败: %v", err)
		}
	}()

	// 启动HTTPS服务器（如果启用）
	if cfg.Server.HTTPS.Enabled {
		go func() {
			log.Printf("HTTPS服务器启动在 %s", httpsServer.Addr)
			if err := httpsServer.ListenAndServeTLS(cfg.Server.HTTPS.CertFile, cfg.Server.HTTPS.KeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTPS服务器启动失败: %v", err)
			}
		}()
	}

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务器...")

	// 停止配置热重载器
	hotReloader.Stop()

	// 优雅关闭服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP服务器关闭失败: %v", err)
	}

	if httpsServer != nil {
		if err := httpsServer.Shutdown(ctx); err != nil {
			log.Printf("HTTPS服务器关闭失败: %v", err)
		}
	}

	log.Println("服务器已关闭")
}

// corsMiddleware CORS中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// printAPIEndpoints 打印API端点信息
func printAPIEndpoints(cfg *config.Config) {
	// HTTP URL
	httpBaseURL := fmt.Sprintf("http://%s:%s", cfg.Server.Host, cfg.Server.Port)
	if cfg.Server.Host == "0.0.0.0" {
		httpBaseURL = fmt.Sprintf("http://localhost:%s", cfg.Server.Port)
	}

	// HTTPS URL (如果启用)
	var httpsBaseURL string
	if cfg.Server.HTTPS.Enabled {
		httpsBaseURL = fmt.Sprintf("https://%s:%s", cfg.Server.Host, cfg.Server.HTTPS.Port)
		if cfg.Server.Host == "0.0.0.0" {
			httpsBaseURL = fmt.Sprintf("https://localhost:%s", cfg.Server.HTTPS.Port)
		}
	}

	log.Println("=== API端点信息 ===")

	// 打印HTTP端点
	log.Printf("HTTP服务:")
	log.Printf("  健康检查: GET %s/health", httpBaseURL)
	log.Printf("  文件上传: POST %s/api/v1/upload", httpBaseURL)
	log.Printf("  文件删除: DELETE %s/api/v1/files/{filename}", httpBaseURL)
	log.Printf("  文件信息: GET %s/api/v1/files/{filename}", httpBaseURL)

	// 打印静态文件服务端点
	printStaticFileEndpoints(httpBaseURL, cfg)

	// 打印HTTPS端点（如果启用）
	if cfg.Server.HTTPS.Enabled {
		log.Printf("HTTPS服务:")
		log.Printf("  健康检查: GET %s/health", httpsBaseURL)
		log.Printf("  文件上传: POST %s/api/v1/upload", httpsBaseURL)
		log.Printf("  文件删除: DELETE %s/api/v1/files/{filename}", httpsBaseURL)
		log.Printf("  文件信息: GET %s/api/v1/files/{filename}", httpsBaseURL)

		// 打印静态文件服务端点
		printStaticFileEndpoints(httpsBaseURL, cfg)
	}

	if gin.Mode() == gin.DebugMode {
		log.Printf("调试端点:")
		log.Printf("  签名信息: GET %s/debug/signature", httpBaseURL)
		log.Printf("  配置信息: GET %s/debug/config", httpBaseURL)
		if cfg.Server.HTTPS.Enabled {
			log.Printf("  签名信息: GET %s/debug/signature", httpsBaseURL)
			log.Printf("  配置信息: GET %s/debug/config", httpsBaseURL)
		}
	}

	log.Println("=== 签名说明 ===")
	log.Println("API请求需要签名验证，参数格式：")
	log.Println("?expires=时间戳&signature=签名值")
	log.Println("签名内容：文件路径 + 过期时间 + 随机数")
	log.Println("签名算法：HMAC-SHA256")
	log.Println("==================")
}

// printStaticFileEndpoints 打印静态文件服务端点
func printStaticFileEndpoints(baseURL string, cfg *config.Config) {
	// 默认本地存储
	if cfg.Storage.Type == "local" {
		requireAuth := cfg.GetStorageAuthRequirement("")
		authStatus := "公开访问"
		if requireAuth {
			authStatus = "需要签名验证"
		}
		log.Printf("  静态文件: GET %s/uploads/{filename} (%s)", baseURL, authStatus)
	}

	// 多存储配置中的本地存储
	if cfg.Storage.Storages != nil {
		for name, storageConfig := range cfg.Storage.Storages {
			// 检查存储是否在启用列表中
			if len(cfg.Storage.EnabledStorages) > 0 {
				enabled := false
				for _, enabledName := range cfg.Storage.EnabledStorages {
					if enabledName == name {
						enabled = true
						break
					}
				}
				if !enabled {
					continue
				}
			}

			// 检查是否是本地存储类型
			configMap, ok := storageConfig.(map[interface{}]interface{})
			if !ok {
				continue
			}

			storageTypeInterface, exists := configMap["type"]
			if !exists {
				continue
			}

			storageType, ok := storageTypeInterface.(string)
			if !ok || storageType != "local" {
				continue
			}

			// 获取base_url来确定路由路径
			baseURLInterface, exists := configMap["base_url"]
			if !exists {
				continue
			}

			baseURLStr, ok := baseURLInterface.(string)
			if !ok {
				continue
			}

			// 从base_url中提取路径部分
			routePath := "/" + name // 默认使用存储名称
			if idx := strings.LastIndex(baseURLStr, "/"); idx > 7 { // 跳过 https://
				routePath = baseURLStr[idx:]
			}

			// 获取该存储的认证要求并打印端点
			requireAuth := cfg.GetStorageAuthRequirement(name)
			authStatus := "公开访问"
			if requireAuth {
				authStatus = "需要签名验证"
			}
			log.Printf("  静态文件: GET %s%s/{filename} (%s)", baseURL, routePath, authStatus)
		}
	}
}

// addStaticFileServices 为所有本地存储添加静态文件服务
func addStaticFileServices(router *gin.Engine, storageManager *storage.StorageManager, cfg *config.Config) {
	// 创建Referer检查中间件
	refererMiddleware := middleware.NewRefererCheckMiddleware(storageManager, cfg)

	// 用于跟踪已注册的路径，避免冲突
	registeredPaths := make(map[string]string)

	// 为默认本地存储添加静态文件服务
	if cfg.Storage.Type == "local" {
		// 从base_url中解析路径，而不是硬编码
		routePath := "/uploads" // 默认路径
		if cfg.Storage.Local.BaseURL != "" {
			if idx := strings.LastIndex(cfg.Storage.Local.BaseURL, "/"); idx > 7 { // 跳过 https://
				routePath = cfg.Storage.Local.BaseURL[idx:]
			}
		}

		registeredPaths[routePath] = "默认本地存储"
		requireAuth := cfg.GetStorageAuthRequirement("")

		if requireAuth {
			// 使用带签名验证的静态文件处理器（包含Referer检查）
			staticHandler := handlers.NewStaticFileHandler(storageManager, cfg)
			router.GET(routePath+"/*filepath", refererMiddleware.CheckReferer(), staticHandler.ServeFile("", cfg.Storage.Local.UploadDir))
			log.Printf("静态文件服务(需签名+Referer检查): GET %s/* -> %s", routePath, cfg.Storage.Local.UploadDir)
		} else {
			// 使用无签名验证的静态文件服务（但包含Referer检查和访问统计）
			staticHandler := handlers.NewStaticFileHandler(storageManager, cfg)
			router.GET(routePath+"/*filepath", refererMiddleware.CheckReferer(), staticHandler.ServeFilePublic("", cfg.Storage.Local.UploadDir))
			log.Printf("静态文件服务(公开+Referer检查+统计): GET %s/* -> %s", routePath, cfg.Storage.Local.UploadDir)
		}
	}

	// 为多存储配置中的本地存储添加静态文件服务
	if cfg.Storage.Storages != nil {
		for name, storageConfig := range cfg.Storage.Storages {
			// 检查存储是否在启用列表中
			if len(cfg.Storage.EnabledStorages) > 0 {
				enabled := false
				for _, enabledName := range cfg.Storage.EnabledStorages {
					if enabledName == name {
						enabled = true
						break
					}
				}
				if !enabled {
					continue
				}
			}

			// 检查是否是本地存储类型
			configMap, ok := storageConfig.(map[interface{}]interface{})
			if !ok {
				continue
			}

			storageTypeInterface, exists := configMap["type"]
			if !exists {
				continue
			}

			storageType, ok := storageTypeInterface.(string)
			if !ok || storageType != "local" {
				continue
			}

			// 获取上传目录和base_url
			uploadDirInterface, exists := configMap["upload_dir"]
			if !exists {
				continue
			}

			uploadDir, ok := uploadDirInterface.(string)
			if !ok {
				continue
			}

			// 获取base_url来确定路由路径
			baseURLInterface, exists := configMap["base_url"]
			if !exists {
				continue
			}

			baseURL, ok := baseURLInterface.(string)
			if !ok {
				continue
			}

			// 从base_url中提取路径部分
			// 例如：https://domain.com:8443/backup -> /backup
			routePath := "/" + name // 默认使用存储名称
			if idx := strings.LastIndex(baseURL, "/"); idx > 7 { // 跳过 https://
				routePath = baseURL[idx:]
			}

			// 检查路径冲突
			if existingStorage, exists := registeredPaths[routePath]; exists {
				log.Printf("警告: 路径冲突 %s，已被 %s 使用，跳过存储 %s", routePath, existingStorage, name)
				continue
			}
			registeredPaths[routePath] = fmt.Sprintf("存储:%s", name)

			// 根据存储配置决定是否需要签名验证
			requireAuth := cfg.GetStorageAuthRequirement(name)

			if requireAuth {
				// 使用带签名验证的静态文件处理器（包含Referer检查）
				staticHandler := handlers.NewStaticFileHandler(storageManager, cfg)
				router.GET(routePath+"/*filepath", refererMiddleware.CheckReferer(), staticHandler.ServeFile(name, uploadDir))
				log.Printf("静态文件服务(需签名+Referer检查): GET %s/* -> %s", routePath, uploadDir)
			} else {
				// 使用无签名验证的静态文件服务（但包含Referer检查和访问统计）
				staticHandler := handlers.NewStaticFileHandler(storageManager, cfg)
				router.GET(routePath+"/*filepath", refererMiddleware.CheckReferer(), staticHandler.ServeFilePublic(name, uploadDir))
				log.Printf("静态文件服务(公开+Referer检查+统计): GET %s/* -> %s", routePath, uploadDir)
			}
		}
	}
}

// ensureAntiHotlinkImage 确保防盗链图片存在，如果不存在则生成默认图片
func ensureAntiHotlinkImage(cfg *config.Config) {
	// 获取防盗链图片路径
	antiHotlinkImagePath := cfg.Upload.AntiHotlinkImage
	if antiHotlinkImagePath == "" {
		antiHotlinkImagePath = "./static/anti-hotlink.png" // 默认路径
	}

	// 检查文件是否存在
	if _, err := os.Stat(antiHotlinkImagePath); err == nil {
		log.Printf("防盗链图片已存在: %s", antiHotlinkImagePath)
		return
	}

	// 创建目录
	dir := filepath.Dir(antiHotlinkImagePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("创建防盗链图片目录失败: %v", err)
		return
	}

	// 生成默认防盗链图片
	img := generateDefaultAntiHotlinkImage()

	// 保存图片到文件
	file, err := os.Create(antiHotlinkImagePath)
	if err != nil {
		log.Printf("创建防盗链图片文件失败: %v", err)
		return
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		log.Printf("保存防盗链图片失败: %v", err)
		return
	}

	log.Printf("已生成默认防盗链图片: %s", antiHotlinkImagePath)
	log.Printf("提示: 您可以替换此图片为自定义的防盗链图片")
}

// generateDefaultAntiHotlinkImage 生成默认的防盗链图片
func generateDefaultAntiHotlinkImage() image.Image {
	// 创建一个400x300的图片
	width, height := 400, 300
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// 设置背景色为浅灰色
	bgColor := color.RGBA{240, 240, 240, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	// 绘制一个简单的禁止符号（圆圈+斜线）
	drawProhibitSymbol(img, width/2, 80, 30)

	// 设置文字颜色为深灰色
	textColor := color.RGBA{80, 80, 80, 255}

	// 绘制文字
	face := basicfont.Face7x13
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(textColor),
		Face: face,
	}

	// 绘制多行文字（使用英文避免中文乱码）
	lines := []string{
		"Access Denied",
		"",
		"This resource does not allow hotlinking.",
		"",
		"If you are the administrator,",
		"please check your configuration file.",
		"",
		"Error Code: HOTLINK_DENIED",
		"",
		"Tip: Replace this default image",
		"with your custom anti-hotlink image.",
	}

	lineHeight := 18
	startY := 30

	for i, line := range lines {
		// 计算文字居中位置
		textWidth := font.MeasureString(face, line)
		x := (width - textWidth.Round()) / 2
		y := startY + i*lineHeight

		drawer.Dot = fixed.Point26_6{
			X: fixed.I(x),
			Y: fixed.I(y),
		}
		drawer.DrawString(line)
	}

	return img
}

// drawProhibitSymbol 绘制禁止符号（圆圈+斜线）
func drawProhibitSymbol(img *image.RGBA, centerX, centerY, radius int) {
	red := color.RGBA{200, 50, 50, 255}

	// 绘制圆圈
	for y := centerY - radius; y <= centerY + radius; y++ {
		for x := centerX - radius; x <= centerX + radius; x++ {
			dx := x - centerX
			dy := y - centerY
			distance := dx*dx + dy*dy

			// 圆圈边框（粗线）
			if distance >= (radius-3)*(radius-3) && distance <= radius*radius {
				if x >= 0 && x < img.Bounds().Max.X && y >= 0 && y < img.Bounds().Max.Y {
					img.Set(x, y, red)
				}
			}
		}
	}

	// 绘制斜线（从左上到右下）
	for i := -radius + 5; i <= radius - 5; i++ {
		for thickness := -2; thickness <= 2; thickness++ {
			x := centerX + i
			y := centerY + i + thickness
			if x >= 0 && x < img.Bounds().Max.X && y >= 0 && y < img.Bounds().Max.Y {
				img.Set(x, y, red)
			}
		}
	}
}
