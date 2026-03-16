package config

import (
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// HotReloader 配置热重载器
type HotReloader struct {
	configPath   string
	config       *Config
	watcher      *fsnotify.Watcher
	mu           sync.RWMutex
	callbacks    []func(*Config)
	stopChan     chan struct{}
	isRunning    bool
}

// NewHotReloader 创建配置热重载器
func NewHotReloader(configPath string) (*HotReloader, error) {
	// 加载初始配置
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	// 创建文件监听器
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	reloader := &HotReloader{
		configPath: configPath,
		config:     config,
		watcher:    watcher,
		callbacks:  make([]func(*Config), 0),
		stopChan:   make(chan struct{}),
		isRunning:  false,
	}

	return reloader, nil
}

// GetConfig 获取当前配置（线程安全）
func (hr *HotReloader) GetConfig() *Config {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return hr.config
}

// AddCallback 添加配置变更回调函数
func (hr *HotReloader) AddCallback(callback func(*Config)) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	hr.callbacks = append(hr.callbacks, callback)
}

// Start 启动热重载监听
func (hr *HotReloader) Start() error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if hr.isRunning {
		return nil
	}

	// 监听配置文件目录
	configDir := filepath.Dir(hr.configPath)
	err := hr.watcher.Add(configDir)
	if err != nil {
		return err
	}

	hr.isRunning = true
	go hr.watchLoop()

	log.Printf("配置文件热重载已启动，监听: %s", hr.configPath)
	return nil
}

// Stop 停止热重载监听
func (hr *HotReloader) Stop() {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if !hr.isRunning {
		return
	}

	close(hr.stopChan)
	hr.watcher.Close()
	hr.isRunning = false

	log.Printf("配置文件热重载已停止")
}

// watchLoop 监听文件变更的主循环
func (hr *HotReloader) watchLoop() {
	// 防抖动：在短时间内的多次变更只处理一次
	var debounceTimer *time.Timer
	const debounceDelay = 500 * time.Millisecond

	for {
		select {
		case event, ok := <-hr.watcher.Events:
			if !ok {
				return
			}

			// 只处理配置文件的写入事件
			if event.Name == hr.configPath && (event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create) {
				log.Printf("检测到配置文件变更: %s", event.Name)

				// 重置防抖动定时器
				if debounceTimer != nil {
					debounceTimer.Stop()
				}

				debounceTimer = time.AfterFunc(debounceDelay, func() {
					hr.reloadConfig()
				})
			}

		case err, ok := <-hr.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("配置文件监听错误: %v", err)

		case <-hr.stopChan:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		}
	}
}

// reloadConfig 重新加载配置文件
func (hr *HotReloader) reloadConfig() {
	log.Printf("开始重新加载配置文件: %s", hr.configPath)

	// 尝试加载新配置
	newConfig, err := LoadConfig(hr.configPath)
	if err != nil {
		log.Printf("重新加载配置文件失败: %v", err)
		return
	}

	// 验证新配置
	if err := hr.validateConfig(newConfig); err != nil {
		log.Printf("新配置验证失败: %v", err)
		return
	}

	// 更新配置
	hr.mu.Lock()
	oldConfig := hr.config
	hr.config = newConfig
	callbacks := make([]func(*Config), len(hr.callbacks))
	copy(callbacks, hr.callbacks)
	hr.mu.Unlock()

	// 更新全局配置
	SetGlobalConfig(newConfig)

	log.Printf("配置文件重新加载成功")

	// 执行回调函数
	for _, callback := range callbacks {
		go func(cb func(*Config)) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("配置变更回调函数执行失败: %v", r)
				}
			}()
			cb(newConfig)
		}(callback)
	}

	// 记录配置变更
	hr.logConfigChanges(oldConfig, newConfig)
}

// validateConfig 验证配置的有效性
func (hr *HotReloader) validateConfig(config *Config) error {
	// 基本验证
	if config.Server.Port == "" {
		return fmt.Errorf("服务器端口不能为空")
	}

	if config.Security.SecretKey == "" {
		return fmt.Errorf("安全密钥不能为空")
	}

	// 存储配置验证
	if config.Storage.Type != "local" && config.Storage.Type != "s3" {
		return fmt.Errorf("不支持的存储类型: %s", config.Storage.Type)
	}

	return nil
}

// logConfigChanges 记录配置变更
func (hr *HotReloader) logConfigChanges(oldConfig, newConfig *Config) {
	changes := []string{}

	// 检查服务器配置变更
	if oldConfig.Server.Port != newConfig.Server.Port {
		changes = append(changes, fmt.Sprintf("服务器端口: %s -> %s", oldConfig.Server.Port, newConfig.Server.Port))
	}

	if oldConfig.Server.Host != newConfig.Server.Host {
		changes = append(changes, fmt.Sprintf("服务器主机: %s -> %s", oldConfig.Server.Host, newConfig.Server.Host))
	}

	// 检查存储配置变更
	if oldConfig.Storage.Type != newConfig.Storage.Type {
		changes = append(changes, fmt.Sprintf("存储类型: %s -> %s", oldConfig.Storage.Type, newConfig.Storage.Type))
	}

	// 检查安全配置变更
	if oldConfig.Security.SignatureExpiry != newConfig.Security.SignatureExpiry {
		changes = append(changes, fmt.Sprintf("签名过期时间: %d -> %d", oldConfig.Security.SignatureExpiry, newConfig.Security.SignatureExpiry))
	}

	// 检查HTTPS配置变更
	if oldConfig.Server.HTTPS.Enabled != newConfig.Server.HTTPS.Enabled {
		changes = append(changes, fmt.Sprintf("HTTPS启用状态: %t -> %t", oldConfig.Server.HTTPS.Enabled, newConfig.Server.HTTPS.Enabled))
	}

	if len(changes) > 0 {
		log.Printf("配置变更详情:")
		for _, change := range changes {
			log.Printf("  - %s", change)
		}
	} else {
		log.Printf("配置文件已重新加载，但未检测到关键配置变更")
	}
}

// IsRunning 检查热重载器是否正在运行
func (hr *HotReloader) IsRunning() bool {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return hr.isRunning
}
