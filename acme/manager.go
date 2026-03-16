package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"

	"file_uploader/config"
)

// 导入DNS提供商
import (
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/alidns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/tencentcloud"
)

// Manager ACME证书管理器
type Manager struct {
	config               *config.ACMEConfig
	client               *lego.Client
	user                 *User
	certDir              string
	dnsProvider          challenge.Provider
	dnsManager           *DNSManager
	onCertificateUpdated func() error
}

// User ACME用户信息
type User struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

// GetEmail 获取用户邮箱
func (u *User) GetEmail() string {
	return u.Email
}

// GetRegistration 获取注册信息
func (u *User) GetRegistration() *registration.Resource {
	return u.Registration
}

// GetPrivateKey 获取私钥
func (u *User) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// SetCertificateUpdatedHook 设置证书更新后的回调
func (m *Manager) SetCertificateUpdatedHook(hook func() error) {
	m.onCertificateUpdated = hook
}

// NewManager 创建ACME证书管理器
func NewManager(cfg *config.ACMEConfig, networkCfg *config.NetworkConfig) (*Manager, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("ACME未启用")
	}

	// 验证必需配置
	if cfg.Email == "" {
		return nil, fmt.Errorf("ACME邮箱不能为空")
	}
	if len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("ACME域名列表不能为空")
	}

	// 设置默认值
	if cfg.Server == "" {
		cfg.Server = lego.LEDirectoryProduction // Let's Encrypt生产环境
	}
	if cfg.CertDir == "" {
		cfg.CertDir = "./certs"
	}
	if cfg.RenewBefore == 0 {
		cfg.RenewBefore = 30 // 30天
	}
	if cfg.KeyType == "" {
		cfg.KeyType = "RSA2048"
	}

	// 创建证书目录
	if err := os.MkdirAll(cfg.CertDir, 0755); err != nil {
		return nil, fmt.Errorf("创建证书目录失败: %v", err)
	}

	manager := &Manager{
		config:  cfg,
		certDir: cfg.CertDir,
	}

	// 初始化DNS管理器
	if cfg.DNS.AutoDNSRecord {
		dnsManager, err := NewDNSManager(&cfg.DNS, networkCfg)
		if err != nil {
			return nil, fmt.Errorf("初始化DNS管理器失败: %v", err)
		}
		manager.dnsManager = dnsManager
		log.Printf("[ACME] DNS管理器初始化完成")
	}

	// 初始化用户
	if err := manager.initUser(); err != nil {
		return nil, fmt.Errorf("初始化ACME用户失败: %v", err)
	}

	// 初始化ACME客户端
	if err := manager.initClient(); err != nil {
		return nil, fmt.Errorf("初始化ACME客户端失败: %v", err)
	}

	// 初始化DNS提供商
	if err := manager.initDNSProvider(); err != nil {
		return nil, fmt.Errorf("初始化DNS提供商失败: %v", err)
	}

	// 启动DNS检查任务（如果配置了定期检查）
	if manager.dnsManager != nil {
		manager.dnsManager.StartDNSCheckTask(cfg.Domains)
	}

	return manager, nil
}

// initUser 初始化ACME用户
func (m *Manager) initUser() error {
	userKeyPath := filepath.Join(m.certDir, "user.key")

	var privateKey crypto.PrivateKey
	var err error

	// 检查是否已有用户私钥
	if _, statErr := os.Stat(userKeyPath); os.IsNotExist(statErr) {
		// 生成新的用户私钥
		privateKey, err = m.generatePrivateKey()
		if err != nil {
			return fmt.Errorf("生成用户私钥失败: %v", err)
		}

		// 保存私钥
		if err := m.savePrivateKey(privateKey, userKeyPath); err != nil {
			return fmt.Errorf("保存用户私钥失败: %v", err)
		}
		log.Printf("[ACME] 生成新的用户私钥: %s", userKeyPath)
	} else {
		// 加载现有私钥
		privateKey, err = m.loadPrivateKey(userKeyPath)
		if err != nil {
			return fmt.Errorf("加载用户私钥失败: %v", err)
		}
		log.Printf("[ACME] 加载现有用户私钥: %s", userKeyPath)
	}

	m.user = &User{
		Email: m.config.Email,
		key:   privateKey,
	}

	return nil
}

// initClient 初始化ACME客户端
func (m *Manager) initClient() error {
	// 配置ACME客户端
	config := lego.NewConfig(m.user)
	config.CADirURL = m.config.Server
	config.Certificate.KeyType = m.getKeyType()

	// 创建客户端
	client, err := lego.NewClient(config)
	if err != nil {
		return fmt.Errorf("创建ACME客户端失败: %v", err)
	}

	m.client = client

	// 注册用户（如果尚未注册）
	if m.user.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return fmt.Errorf("注册ACME用户失败: %v", err)
		}
		m.user.Registration = reg
		log.Printf("[ACME] 用户注册成功: %s", m.user.Email)
	}

	return nil
}

// generatePrivateKey 生成私钥
func (m *Manager) generatePrivateKey() (crypto.PrivateKey, error) {
	switch strings.ToUpper(m.config.KeyType) {
	case "RSA2048":
		return rsa.GenerateKey(rand.Reader, 2048)
	case "RSA4096":
		return rsa.GenerateKey(rand.Reader, 4096)
	case "EC256":
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "EC384":
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	default:
		return rsa.GenerateKey(rand.Reader, 2048)
	}
}

// getKeyType 获取证书密钥类型
func (m *Manager) getKeyType() certcrypto.KeyType {
	switch strings.ToUpper(m.config.KeyType) {
	case "RSA2048":
		return certcrypto.RSA2048
	case "RSA4096":
		return certcrypto.RSA4096
	case "EC256":
		return certcrypto.EC256
	case "EC384":
		return certcrypto.EC384
	default:
		return certcrypto.RSA2048
	}
}

// savePrivateKey 保存私钥到文件
func (m *Manager) savePrivateKey(key crypto.PrivateKey, path string) error {
	var keyBytes []byte
	var keyType string

	switch k := key.(type) {
	case *rsa.PrivateKey:
		keyBytes = x509.MarshalPKCS1PrivateKey(k)
		keyType = "RSA PRIVATE KEY"
	case *ecdsa.PrivateKey:
		var err error
		keyBytes, err = x509.MarshalECPrivateKey(k)
		if err != nil {
			return err
		}
		keyType = "EC PRIVATE KEY"
	default:
		return fmt.Errorf("不支持的私钥类型")
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  keyType,
		Bytes: keyBytes,
	})

	return os.WriteFile(path, keyPEM, 0600)
}

// loadPrivateKey 从文件加载私钥
func (m *Manager) loadPrivateKey(path string) (crypto.PrivateKey, error) {
	keyPEM, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("无效的PEM格式")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("不支持的私钥类型: %s", block.Type)
	}
}

// GetCertificatePaths 获取证书文件路径
func (m *Manager) GetCertificatePaths() (certPath, keyPath string) {
	domain := m.config.Domains[0] // 使用第一个域名作为文件名
	certPath = filepath.Join(m.certDir, domain+".crt")
	keyPath = filepath.Join(m.certDir, domain+".key")
	return
}

// CertificateExists 检查证书是否存在
func (m *Manager) CertificateExists() bool {
	certPath, keyPath := m.GetCertificatePaths()
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	return certErr == nil && keyErr == nil
}

// NeedsRenewal 检查证书是否需要续期
func (m *Manager) NeedsRenewal() (bool, error) {
	if !m.CertificateExists() {
		return true, nil // 证书不存在，需要申请
	}

	certPath, _ := m.GetCertificatePaths()
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return true, err
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return true, fmt.Errorf("无效的证书格式")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return true, err
	}

	// 检查是否在续期时间窗口内
	renewTime := cert.NotAfter.AddDate(0, 0, -m.config.RenewBefore)
	return time.Now().After(renewTime), nil
}

// initDNSProvider 初始化DNS提供商
func (m *Manager) initDNSProvider() error {
	var provider challenge.Provider
	var err error

	switch strings.ToLower(m.config.DNS.Provider) {
	case "alidns":
		provider, err = m.initAliDNS()
	case "tencentcloud":
		provider, err = m.initTencentCloud()
	case "cloudflare":
		provider, err = m.initCloudflare()
	default:
		return fmt.Errorf("不支持的DNS提供商: %s", m.config.DNS.Provider)
	}

	if err != nil {
		return err
	}

	m.dnsProvider = provider

	// 设置DNS挑战
	err = m.client.Challenge.SetDNS01Provider(provider)
	if err != nil {
		return fmt.Errorf("设置DNS01挑战失败: %v", err)
	}

	log.Printf("[ACME] DNS提供商初始化成功: %s", m.config.DNS.Provider)
	return nil
}

// initAliDNS 初始化阿里云DNS
func (m *Manager) initAliDNS() (challenge.Provider, error) {
	config := alidns.NewDefaultConfig()

	if accessKeyId, ok := m.config.DNS.Config["access_key_id"].(string); ok {
		config.APIKey = accessKeyId
	} else {
		return nil, fmt.Errorf("阿里云DNS缺少access_key_id配置")
	}

	if accessKeySecret, ok := m.config.DNS.Config["access_key_secret"].(string); ok {
		config.SecretKey = accessKeySecret
	} else {
		return nil, fmt.Errorf("阿里云DNS缺少access_key_secret配置")
	}

	if region, ok := m.config.DNS.Config["region"].(string); ok && region != "" {
		config.RegionID = region
	}

	return alidns.NewDNSProviderConfig(config)
}

// initTencentCloud 初始化腾讯云DNS
func (m *Manager) initTencentCloud() (challenge.Provider, error) {
	config := tencentcloud.NewDefaultConfig()

	if secretId, ok := m.config.DNS.Config["secret_id"].(string); ok {
		config.SecretID = secretId
	} else {
		return nil, fmt.Errorf("腾讯云DNS缺少secret_id配置")
	}

	if secretKey, ok := m.config.DNS.Config["secret_key"].(string); ok {
		config.SecretKey = secretKey
	} else {
		return nil, fmt.Errorf("腾讯云DNS缺少secret_key配置")
	}

	if region, ok := m.config.DNS.Config["region"].(string); ok && region != "" {
		config.Region = region
	}

	return tencentcloud.NewDNSProviderConfig(config)
}

// initCloudflare 初始化Cloudflare DNS
func (m *Manager) initCloudflare() (challenge.Provider, error) {
	config := cloudflare.NewDefaultConfig()

	if email, ok := m.config.DNS.Config["email"].(string); ok {
		config.AuthEmail = email
	}

	if apiKey, ok := m.config.DNS.Config["api_key"].(string); ok {
		config.AuthKey = apiKey
	}

	if apiToken, ok := m.config.DNS.Config["api_token"].(string); ok {
		config.AuthToken = apiToken
	}

	if config.AuthEmail == "" && config.AuthToken == "" {
		return nil, fmt.Errorf("Cloudflare DNS需要配置email+api_key或api_token")
	}

	return cloudflare.NewDNSProviderConfig(config)
}

// ObtainCertificate 申请证书
func (m *Manager) ObtainCertificate() error {
	log.Printf("[ACME] 开始申请证书，域名: %v", m.config.Domains)

	// 检查并更新DNS记录（如果启用了自动DNS管理）
	if m.dnsManager != nil {
		log.Printf("[ACME] 检查并更新DNS记录")
		if err := m.dnsManager.CheckAndUpdateDNSRecords(m.config.Domains); err != nil {
			log.Printf("[ACME] DNS记录检查失败: %v", err)
			// 不中断证书申请流程，只记录警告
		} else {
			// DNS记录更新后，等待一段时间让DNS传播
			log.Printf("[ACME] DNS记录更新完成，等待DNS传播...")
			time.Sleep(30 * time.Second)
		}
	}

	request := certificate.ObtainRequest{
		Domains: m.config.Domains,
		Bundle:  true,
	}

	certificates, err := m.client.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("申请证书失败: %v", err)
	}

	// 保存证书和私钥
	certPath, keyPath := m.GetCertificatePaths()

	if err := os.WriteFile(certPath, certificates.Certificate, 0644); err != nil {
		return fmt.Errorf("保存证书失败: %v", err)
	}

	if err := os.WriteFile(keyPath, certificates.PrivateKey, 0600); err != nil {
		return fmt.Errorf("保存私钥失败: %v", err)
	}

	log.Printf("[ACME] 证书申请成功")
	log.Printf("[ACME] 证书文件: %s", certPath)
	log.Printf("[ACME] 私钥文件: %s", keyPath)

	return m.afterCertificateUpdated("申请")
}

// RenewCertificate 续期证书
func (m *Manager) RenewCertificate() error {
	log.Printf("[ACME] 开始续期证书，域名: %v", m.config.Domains)

	certPath, _ := m.GetCertificatePaths()
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("读取现有证书失败: %v", err)
	}

	certificates, err := m.client.Certificate.Renew(certificate.Resource{
		Domain:      m.config.Domains[0],
		Certificate: certPEM,
	}, true, false, "")
	if err != nil {
		return fmt.Errorf("续期证书失败: %v", err)
	}

	// 保存新证书和私钥
	certPath, keyPath := m.GetCertificatePaths()

	if err := os.WriteFile(certPath, certificates.Certificate, 0644); err != nil {
		return fmt.Errorf("保存续期证书失败: %v", err)
	}

	if err := os.WriteFile(keyPath, certificates.PrivateKey, 0600); err != nil {
		return fmt.Errorf("保存续期私钥失败: %v", err)
	}

	log.Printf("[ACME] 证书续期成功")
	return m.afterCertificateUpdated("续期")
}

func (m *Manager) afterCertificateUpdated(action string) error {
	certPath, _ := m.GetCertificatePaths()
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("读取%s后证书失败: %v", action, err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("解析%s后证书失败: 无效的PEM格式", action)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("解析%s后证书失败: %v", action, err)
	}

	log.Printf("[ACME] 新证书有效期: not_before=%s, not_after=%s, 剩余=%d天",
		cert.NotBefore.Format(time.RFC3339),
		cert.NotAfter.Format(time.RFC3339),
		int(time.Until(cert.NotAfter).Hours()/24),
	)

	if m.onCertificateUpdated != nil {
		if err := m.onCertificateUpdated(); err != nil {
			return fmt.Errorf("触发证书热加载失败: %v", err)
		}
		log.Printf("[ACME] 新证书已预热加载到HTTPS服务")
	}

	return nil
}

// EnsureCertificate 确保证书可用（申请或续期）
func (m *Manager) EnsureCertificate() error {
	needsRenewal, err := m.NeedsRenewal()
	if err != nil {
		return fmt.Errorf("检查证书状态失败: %v", err)
	}

	if !needsRenewal {
		log.Printf("[ACME] 证书仍然有效，无需续期")
		return nil
	}

	if m.CertificateExists() {
		return m.RenewCertificate()
	} else {
		return m.ObtainCertificate()
	}
}

// StartAutoRenewal 启动自动续期任务
func (m *Manager) StartAutoRenewal() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour) // 每天检查一次
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				needsRenewal, err := m.NeedsRenewal()
				if err != nil {
					log.Printf("[ACME] 检查证书续期状态失败: %v", err)
					continue
				}

				if needsRenewal {
					log.Printf("[ACME] 证书需要续期，开始自动续期...")
					if err := m.RenewCertificate(); err != nil {
						log.Printf("[ACME] 自动续期失败: %v", err)
					} else {
						log.Printf("[ACME] 自动续期成功")
					}
				}
			}
		}
	}()

	log.Printf("[ACME] 自动续期任务已启动")
}
