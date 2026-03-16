package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"file_uploader/config"
)

// StorageManager 存储管理器
type StorageManager struct {
	storages        map[string]Storage
	securityConfigs map[string]*StorageSecurityConfig
	config          *config.Config
}

// NewStorageManager 创建存储管理器
func NewStorageManager(cfg *config.Config) (*StorageManager, error) {
	manager := &StorageManager{
		storages:        make(map[string]Storage),
		securityConfigs: make(map[string]*StorageSecurityConfig),
		config:          cfg,
	}

	// 统计信息
	var totalAttempted, successCount, failedCount int

	// 初始化默认存储
	totalAttempted++
	defaultStorage, err := manager.createStorage(cfg.Storage.Type, cfg)
	if err != nil {
		failedCount++
		fmt.Printf("⚠️  默认存储初始化失败: %v\n", err)
		fmt.Printf("   提示: 请检查默认存储配置是否正确\n")
		// 对于默认存储失败，我们仍然继续，但会在后续访问时报错
	} else {
		successCount++
		manager.storages["default"] = defaultStorage
		manager.storages[cfg.Storage.Type] = defaultStorage
		fmt.Printf("✅ 成功初始化默认存储: %s\n", cfg.Storage.Type)
	}

	// 初始化多存储配置
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
					fmt.Printf("跳过未启用的存储: %s\n", name)
					continue
				}
			}

			totalAttempted++
			storage, securityConfig, err := manager.createStorageFromConfig(name, storageConfig)
			if err != nil {
				failedCount++
				// 给出友好的错误提示，但不退出程序
				fmt.Printf("⚠️  跳过存储 '%s': %v\n", name, err)
				fmt.Printf("   提示: 请检查存储配置是否正确，或在 enabled_storages 中移除此存储\n")
				continue // 跳过这个存储，继续处理其他存储
			}
			successCount++
			manager.storages[name] = storage
			manager.securityConfigs[name] = securityConfig
			fmt.Printf("✅ 成功初始化存储: %s\n", name)
		}
	}

	// 检查是否至少有一个存储成功初始化
	if len(manager.storages) == 0 {
		return nil, fmt.Errorf("❌ 没有任何存储成功初始化，请检查配置文件")
	}

	// 显示详细的初始化统计信息
	if failedCount > 0 {
		fmt.Printf("📦 存储管理器初始化完成，共尝试 %d 个存储，成功 %d 个，失败 %d 个\n",
			totalAttempted, successCount, failedCount)
	} else {
		fmt.Printf("📦 存储管理器初始化完成，共成功初始化 %d 个存储\n", successCount)
	}

	return manager, nil
}

// GetStorage 获取指定的存储实例
func (m *StorageManager) GetStorage(storageType string) (Storage, error) {
	if storageType == "" {
		storageType = "default"
	}

	storage, exists := m.storages[storageType]
	if !exists {
		return nil, fmt.Errorf("存储类型 '%s' 不存在", storageType)
	}

	return storage, nil
}

// GetAvailableStorages 获取所有可用的存储类型
func (m *StorageManager) GetAvailableStorages() []string {
	var types []string
	for storageType := range m.storages {
		if storageType != "default" {
			types = append(types, storageType)
		}
	}
	return types
}

// GetStorageSecurityConfig 获取存储安全配置
func (m *StorageManager) GetStorageSecurityConfig(storageType string) *StorageSecurityConfig {
	if storageType == "" {
		storageType = "default"
	}

	// 查找存储特定的安全配置
	if config, exists := m.securityConfigs[storageType]; exists {
		return config
	}

	// 返回默认配置
	return &StorageSecurityConfig{
		RequireAuth:  nil, // 使用全局默认
		AllowReferer: nil, // 不检查Referer
	}
}

// createStorage 根据类型创建存储实例
func (m *StorageManager) createStorage(storageType string, cfg *config.Config) (Storage, error) {
	switch storageType {
	case "local":
		return NewLocalStorage(&cfg.Storage.Local), nil
	case "s3":
		return NewS3Storage(&cfg.Storage.S3)
	default:
		return nil, fmt.Errorf("不支持的存储类型: %s", storageType)
	}
}

// createStorageFromConfig 从配置创建存储实例
func (m *StorageManager) createStorageFromConfig(name string, storageConfig interface{}) (Storage, *StorageSecurityConfig, error) {
	configMap, ok := storageConfig.(map[interface{}]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("存储配置格式错误: %s", name)
	}

	// 获取存储类型
	storageTypeInterface, exists := configMap["type"]
	if !exists {
		return nil, nil, fmt.Errorf("存储配置缺少type字段: %s", name)
	}

	storageType, ok := storageTypeInterface.(string)
	if !ok {
		return nil, nil, fmt.Errorf("存储类型必须是字符串: %s", name)
	}

	// 解析安全配置
	securityConfig := m.parseSecurityConfig(configMap)

	var storage Storage
	var err error

	switch storageType {
	case "local":
		storage, err = m.createLocalStorageFromConfig(configMap)
	case "s3":
		storage, err = m.createS3StorageFromConfig(configMap)
	default:
		return nil, nil, fmt.Errorf("不支持的存储类型: %s", storageType)
	}

	if err != nil {
		return nil, nil, err
	}

	return storage, securityConfig, nil
}

// parseSecurityConfig 解析存储安全配置
func (m *StorageManager) parseSecurityConfig(configMap map[interface{}]interface{}) *StorageSecurityConfig {
	securityConfig := &StorageSecurityConfig{}

	// 解析 require_auth
	if requireAuthInterface, exists := configMap["require_auth"]; exists {
		if requireAuth, ok := requireAuthInterface.(bool); ok {
			securityConfig.RequireAuth = &requireAuth
		}
	}

	// 解析 allow_referer
	if allowRefererInterface, exists := configMap["allow_referer"]; exists {
		if allowRefererList, ok := allowRefererInterface.([]interface{}); ok {
			for _, refererInterface := range allowRefererList {
				if referer, ok := refererInterface.(string); ok {
					securityConfig.AllowReferer = append(securityConfig.AllowReferer, referer)
				}
			}
		}
	}

	return securityConfig
}

// createLocalStorageFromConfig 从配置创建本地存储
func (m *StorageManager) createLocalStorageFromConfig(configMap map[interface{}]interface{}) (Storage, error) {
	localConfig := &config.LocalConfig{}

	if uploadDir, exists := configMap["upload_dir"]; exists {
		if dir, ok := uploadDir.(string); ok {
			localConfig.UploadDir = dir
		}
	}

	if baseURL, exists := configMap["base_url"]; exists {
		if url, ok := baseURL.(string); ok {
			localConfig.BaseURL = url
		}
	}

	return NewLocalStorage(localConfig), nil
}

// createS3StorageFromConfig 从配置创建S3存储
func (m *StorageManager) createS3StorageFromConfig(configMap map[interface{}]interface{}) (Storage, error) {
	s3Config := &config.S3Config{}

	if region, exists := configMap["region"]; exists {
		if r, ok := region.(string); ok {
			s3Config.Region = r
		}
	}

	if bucket, exists := configMap["bucket"]; exists {
		if b, ok := bucket.(string); ok {
			s3Config.Bucket = b
		}
	}

	if accessKeyID, exists := configMap["access_key_id"]; exists {
		if key, ok := accessKeyID.(string); ok {
			s3Config.AccessKeyID = key
		}
	}

	if secretAccessKey, exists := configMap["secret_access_key"]; exists {
		if secret, ok := secretAccessKey.(string); ok {
			s3Config.SecretAccessKey = secret
		}
	}

	if baseURL, exists := configMap["base_url"]; exists {
		if url, ok := baseURL.(string); ok {
			s3Config.BaseURL = url
		}
	}

	if endpoint, exists := configMap["endpoint"]; exists {
		if ep, ok := endpoint.(string); ok {
			s3Config.Endpoint = ep
		}
	}

	return NewS3Storage(s3Config)
}

// ParseUploadPath 解析上传路径，提取存储类型和文件路径
// 支持格式：
// - /images/20250704/test.jpg -> (default, images/20250704/test.jpg)
// - /s3/images/20250704/test.jpg -> (s3, images/20250704/test.jpg)
// - /s3_1/images/20250704/test.jpg -> (s3_1, images/20250704/test.jpg)
func (m *StorageManager) ParseUploadPath(uploadPath string) (storageType, filePath string) {
	// 清理路径
	uploadPath = strings.TrimPrefix(uploadPath, "/")
	
	if uploadPath == "" {
		return "default", ""
	}

	// 分割路径
	parts := strings.SplitN(uploadPath, "/", 2)
	
	// 检查第一部分是否是存储类型
	if len(parts) >= 1 {
		firstPart := parts[0]
		
		// 检查是否是已配置的存储类型
		if _, exists := m.storages[firstPart]; exists && firstPart != "default" {
			if len(parts) == 2 {
				return firstPart, parts[1]
			} else {
				return firstPart, ""
			}
		}
	}

	// 如果不是存储类型，则使用默认存储
	return "default", uploadPath
}

// GetStoragePathMapping 获取存储名称到路径前缀的映射
func (m *StorageManager) GetStoragePathMapping() map[string]string {
	pathMapping := make(map[string]string)

	// 添加默认存储的路径映射
	if m.config.Storage.Type == "local" && m.config.Storage.Local.BaseURL != "" {
		if idx := strings.LastIndex(m.config.Storage.Local.BaseURL, "/"); idx > 7 {
			pathMapping["default"] = m.config.Storage.Local.BaseURL[idx:]
		} else {
			pathMapping["default"] = "/uploads" // 默认路径
		}
	}

	// 添加多存储配置的路径映射
	if m.config.Storage.Storages != nil {
		for name, storageConfig := range m.config.Storage.Storages {
			// 检查存储是否在启用列表中
			if len(m.config.Storage.EnabledStorages) > 0 {
				enabled := false
				for _, enabledName := range m.config.Storage.EnabledStorages {
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

			// 获取base_url来确定路径前缀
			baseURLInterface, exists := configMap["base_url"]
			if !exists {
				continue
			}

			baseURL, ok := baseURLInterface.(string)
			if !ok {
				continue
			}

			// 从base_url中提取路径部分
			if idx := strings.LastIndex(baseURL, "/"); idx > 7 {
				pathMapping[name] = baseURL[idx:]
			} else {
				pathMapping[name] = "/" + name // 默认使用存储名称
			}
		}
	}

	return pathMapping
}

// EnsureDirectory 确保目录存在（用于本地存储）
func EnsureDirectory(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir != "." && dir != "/" {
		return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil && os.IsNotExist(err) {
				return os.MkdirAll(dir, 0755)
			}
			return nil
		})
	}
	return nil
}
