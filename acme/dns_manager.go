package acme

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"file_uploader/config"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
	"github.com/cloudflare/cloudflare-go"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	dnspod "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

// DNSManager DNS记录管理器
type DNSManager struct {
	config      *config.DNSConfig
	provider    string
	ipDetector  *IPDetector
}

// DNSRecord DNS记录结构
type DNSRecord struct {
	Domain string
	Type   string
	Value  string
	TTL    int
}

// NewDNSManager 创建DNS管理器
func NewDNSManager(cfg *config.DNSConfig, networkCfg *config.NetworkConfig) (*DNSManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("DNS配置不能为空")
	}

	// 解析网络超时配置
	var timeout time.Duration = 10 * time.Second // 默认值
	if networkCfg != nil && networkCfg.RequestTimeout != "" {
		if parsedTimeout, err := time.ParseDuration(networkCfg.RequestTimeout); err == nil {
			timeout = parsedTimeout
		}
	}

	ipDetector := NewIPDetector(cfg.ExternalIPAPIs, timeout)

	return &DNSManager{
		config:     cfg,
		provider:   cfg.Provider,
		ipDetector: ipDetector,
	}, nil
}

// CheckAndUpdateDNSRecords 检查并更新DNS记录
func (dm *DNSManager) CheckAndUpdateDNSRecords(domains []string) error {
	if !dm.config.AutoDNSRecord {
		log.Printf("[DNS管理] 自动DNS记录管理已禁用")
		return nil
	}
	
	log.Printf("[DNS管理] 开始检查DNS记录，域名数量: %d", len(domains))
	
	// 获取当前外网IP
	var currentIP string
	var err error
	
	if dm.config.ExternalIP != "" {
		currentIP = dm.config.ExternalIP
		log.Printf("[DNS管理] 使用手动指定的外网IP: %s", currentIP)
	} else {
		currentIP, err = dm.ipDetector.DetectExternalIP()
		if err != nil {
			return fmt.Errorf("检测外网IP失败: %v", err)
		}
	}
	
	// 验证IP格式
	if !IsValidIP(currentIP) {
		return fmt.Errorf("无效的IP地址: %s", currentIP)
	}
	
	if IsPrivateIP(currentIP) {
		log.Printf("[DNS管理] 警告: 检测到私有IP地址 %s，这可能不是外网IP", currentIP)
	}
	
	// 检查每个域名的DNS记录
	for _, domain := range domains {
		if err := dm.checkAndUpdateDomainRecord(domain, currentIP); err != nil {
			log.Printf("[DNS管理] 处理域名 %s 失败: %v", domain, err)
			// 继续处理其他域名，不中断整个流程
		}
	}
	
	return nil
}

// checkAndUpdateDomainRecord 检查并更新单个域名的DNS记录
func (dm *DNSManager) checkAndUpdateDomainRecord(domain, targetIP string) error {
	log.Printf("[DNS管理] 检查域名: %s", domain)
	
	// 查询当前DNS记录
	currentIPs, err := dm.lookupDomainIP(domain)
	if err != nil {
		log.Printf("[DNS管理] 查询域名 %s 的DNS记录失败: %v", domain, err)
		// DNS查询失败可能意味着记录不存在，尝试创建
		return dm.createDNSRecord(domain, targetIP)
	}
	
	// 检查是否已经指向正确的IP
	for _, ip := range currentIPs {
		if ip == targetIP {
			log.Printf("[DNS管理] 域名 %s 已正确解析到 %s", domain, targetIP)
			return nil
		}
	}
	
	// 需要更新DNS记录
	log.Printf("[DNS管理] 域名 %s 当前解析到 %v，需要更新为 %s", domain, currentIPs, targetIP)
	return dm.updateDNSRecord(domain, targetIP)
}

// lookupDomainIP 查询域名的IP地址
func (dm *DNSManager) lookupDomainIP(domain string) ([]string, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return nil, err
	}
	
	var ipStrings []string
	for _, ip := range ips {
		if ip.To4() != nil { // 只处理IPv4
			ipStrings = append(ipStrings, ip.String())
		}
	}
	
	return ipStrings, nil
}

// createDNSRecord 创建DNS记录
func (dm *DNSManager) createDNSRecord(domain, ip string) error {
	log.Printf("[DNS管理] 创建DNS记录: %s -> %s", domain, ip)
	
	switch dm.provider {
	case "alidns":
		return dm.createAliDNSRecord(domain, ip)
	case "tencentcloud":
		return dm.createTencentDNSRecord(domain, ip)
	case "cloudflare":
		return dm.createCloudflareDNSRecord(domain, ip)
	default:
		return fmt.Errorf("不支持的DNS提供商: %s", dm.provider)
	}
}

// updateDNSRecord 更新DNS记录
func (dm *DNSManager) updateDNSRecord(domain, ip string) error {
	log.Printf("[DNS管理] 更新DNS记录: %s -> %s", domain, ip)
	
	switch dm.provider {
	case "alidns":
		return dm.updateAliDNSRecord(domain, ip)
	case "tencentcloud":
		return dm.updateTencentDNSRecord(domain, ip)
	case "cloudflare":
		return dm.updateCloudflareDNSRecord(domain, ip)
	default:
		return fmt.Errorf("不支持的DNS提供商: %s", dm.provider)
	}
}

// StartDNSCheckTask 启动DNS检查任务
func (dm *DNSManager) StartDNSCheckTask(domains []string) {
	if !dm.config.AutoDNSRecord || dm.config.DNSCheckInterval <= 0 {
		return
	}
	
	interval := time.Duration(dm.config.DNSCheckInterval) * time.Minute
	log.Printf("[DNS管理] 启动DNS检查任务，检查间隔: %v", interval)
	
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			log.Printf("[DNS管理] 执行定期DNS检查")
			if err := dm.CheckAndUpdateDNSRecords(domains); err != nil {
				log.Printf("[DNS管理] 定期DNS检查失败: %v", err)
			}
		}
	}()
}

// extractRootDomain 提取根域名
func (dm *DNSManager) extractRootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return domain
}

// extractSubDomain 提取子域名
func (dm *DNSManager) extractSubDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) > 2 {
		return strings.Join(parts[:len(parts)-2], ".")
	}
	return "@" // 根域名用@表示
}

// createAliDNSRecord 创建阿里云DNS记录
func (dm *DNSManager) createAliDNSRecord(domain, ip string) error {
	log.Printf("[阿里云DNS] 创建A记录: %s -> %s", domain, ip)

	client, err := dm.createAliDNSClient()
	if err != nil {
		return fmt.Errorf("创建阿里云DNS客户端失败: %v", err)
	}

	rootDomain := dm.extractRootDomain(domain)
	subDomain := dm.extractSubDomain(domain)

	// 创建DNS记录请求
	request := alidns.CreateAddDomainRecordRequest()
	request.Scheme = "https"
	request.DomainName = rootDomain
	request.RR = subDomain
	request.Type = "A"
	request.Value = ip
	request.TTL = "600" // 10分钟TTL

	response, err := client.AddDomainRecord(request)
	if err != nil {
		return fmt.Errorf("添加DNS记录失败: %v", err)
	}

	log.Printf("[阿里云DNS] 成功创建DNS记录，记录ID: %s", response.RecordId)
	return nil
}

// updateAliDNSRecord 更新阿里云DNS记录
func (dm *DNSManager) updateAliDNSRecord(domain, ip string) error {
	log.Printf("[阿里云DNS] 更新A记录: %s -> %s", domain, ip)

	client, err := dm.createAliDNSClient()
	if err != nil {
		return fmt.Errorf("创建阿里云DNS客户端失败: %v", err)
	}

	// 首先查找现有记录
	recordId, err := dm.findAliDNSRecord(client, domain)
	if err != nil {
		log.Printf("[阿里云DNS] 未找到现有记录，尝试创建新记录")
		return dm.createAliDNSRecord(domain, ip)
	}

	// 更新现有记录
	request := alidns.CreateUpdateDomainRecordRequest()
	request.Scheme = "https"
	request.RecordId = recordId
	request.RR = dm.extractSubDomain(domain)
	request.Type = "A"
	request.Value = ip
	request.TTL = "600"

	_, err = client.UpdateDomainRecord(request)
	if err != nil {
		return fmt.Errorf("更新DNS记录失败: %v", err)
	}

	log.Printf("[阿里云DNS] 成功更新DNS记录，记录ID: %s", recordId)
	return nil
}

// createTencentDNSRecord 创建腾讯云DNS记录
func (dm *DNSManager) createTencentDNSRecord(domain, ip string) error {
	log.Printf("[腾讯云DNS] 创建A记录: %s -> %s", domain, ip)

	client, err := dm.createTencentDNSClient()
	if err != nil {
		return fmt.Errorf("创建腾讯云DNS客户端失败: %v", err)
	}

	rootDomain := dm.extractRootDomain(domain)
	subDomain := dm.extractSubDomain(domain)

	// 创建DNS记录请求
	request := dnspod.NewCreateRecordRequest()
	request.Domain = common.StringPtr(rootDomain)
	request.SubDomain = common.StringPtr(subDomain)
	request.RecordType = common.StringPtr("A")
	request.RecordLine = common.StringPtr("默认")
	request.Value = common.StringPtr(ip)
	request.TTL = common.Uint64Ptr(600) // 10分钟TTL

	response, err := client.CreateRecord(request)
	if err != nil {
		return fmt.Errorf("添加DNS记录失败: %v", err)
	}

	log.Printf("[腾讯云DNS] 成功创建DNS记录，记录ID: %d", *response.Response.RecordId)
	return nil
}

// updateTencentDNSRecord 更新腾讯云DNS记录
func (dm *DNSManager) updateTencentDNSRecord(domain, ip string) error {
	log.Printf("[腾讯云DNS] 更新A记录: %s -> %s", domain, ip)

	client, err := dm.createTencentDNSClient()
	if err != nil {
		return fmt.Errorf("创建腾讯云DNS客户端失败: %v", err)
	}

	// 首先查找现有记录
	recordId, err := dm.findTencentDNSRecord(client, domain)
	if err != nil {
		log.Printf("[腾讯云DNS] 未找到现有记录，尝试创建新记录")
		return dm.createTencentDNSRecord(domain, ip)
	}

	rootDomain := dm.extractRootDomain(domain)
	subDomain := dm.extractSubDomain(domain)

	// 更新现有记录
	request := dnspod.NewModifyRecordRequest()
	request.Domain = common.StringPtr(rootDomain)
	request.RecordId = common.Uint64Ptr(recordId)
	request.SubDomain = common.StringPtr(subDomain)
	request.RecordType = common.StringPtr("A")
	request.RecordLine = common.StringPtr("默认")
	request.Value = common.StringPtr(ip)
	request.TTL = common.Uint64Ptr(600)

	_, err = client.ModifyRecord(request)
	if err != nil {
		return fmt.Errorf("更新DNS记录失败: %v", err)
	}

	log.Printf("[腾讯云DNS] 成功更新DNS记录，记录ID: %d", recordId)
	return nil
}

// createCloudflareDNSRecord 创建Cloudflare DNS记录
func (dm *DNSManager) createCloudflareDNSRecord(domain, ip string) error {
	log.Printf("[Cloudflare DNS] 创建A记录: %s -> %s", domain, ip)

	api, zoneID, err := dm.createCloudflareClient(domain)
	if err != nil {
		return fmt.Errorf("创建Cloudflare客户端失败: %v", err)
	}

	subDomain := dm.extractSubDomain(domain)
	if subDomain == "@" {
		subDomain = dm.extractRootDomain(domain)
	} else {
		subDomain = subDomain + "." + dm.extractRootDomain(domain)
	}

	// 创建DNS记录
	params := cloudflare.CreateDNSRecordParams{
		Type:    "A",
		Name:    subDomain,
		Content: ip,
		TTL:     600, // 10分钟TTL
	}

	response, err := api.CreateDNSRecord(context.Background(), cloudflare.ZoneIdentifier(zoneID), params)
	if err != nil {
		return fmt.Errorf("添加DNS记录失败: %v", err)
	}

	log.Printf("[Cloudflare DNS] 成功创建DNS记录，记录ID: %s", response.ID)
	return nil
}

// updateCloudflareDNSRecord 更新Cloudflare DNS记录
func (dm *DNSManager) updateCloudflareDNSRecord(domain, ip string) error {
	log.Printf("[Cloudflare DNS] 更新A记录: %s -> %s", domain, ip)

	api, zoneID, err := dm.createCloudflareClient(domain)
	if err != nil {
		return fmt.Errorf("创建Cloudflare客户端失败: %v", err)
	}

	// 首先查找现有记录
	recordID, err := dm.findCloudflareRecord(api, zoneID, domain)
	if err != nil {
		log.Printf("[Cloudflare DNS] 未找到现有记录，尝试创建新记录")
		return dm.createCloudflareDNSRecord(domain, ip)
	}

	subDomain := dm.extractSubDomain(domain)
	if subDomain == "@" {
		subDomain = dm.extractRootDomain(domain)
	} else {
		subDomain = subDomain + "." + dm.extractRootDomain(domain)
	}

	// 更新现有记录
	params := cloudflare.UpdateDNSRecordParams{
		ID:      recordID,
		Type:    "A",
		Name:    subDomain,
		Content: ip,
		TTL:     600,
	}

	_, err = api.UpdateDNSRecord(context.Background(), cloudflare.ZoneIdentifier(zoneID), params)
	if err != nil {
		return fmt.Errorf("更新DNS记录失败: %v", err)
	}

	log.Printf("[Cloudflare DNS] 成功更新DNS记录，记录ID: %s", recordID)
	return nil
}

// createAliDNSClient 创建阿里云DNS客户端
func (dm *DNSManager) createAliDNSClient() (*alidns.Client, error) {
	accessKeyId, ok := dm.config.Config["access_key_id"].(string)
	if !ok || accessKeyId == "" {
		return nil, fmt.Errorf("阿里云DNS缺少access_key_id配置")
	}

	accessKeySecret, ok := dm.config.Config["access_key_secret"].(string)
	if !ok || accessKeySecret == "" {
		return nil, fmt.Errorf("阿里云DNS缺少access_key_secret配置")
	}

	region, ok := dm.config.Config["region"].(string)
	if !ok || region == "" {
		region = "cn-hangzhou" // 默认区域
	}

	client, err := alidns.NewClientWithAccessKey(region, accessKeyId, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("创建阿里云DNS客户端失败: %v", err)
	}

	return client, nil
}

// findAliDNSRecord 查找阿里云DNS记录
func (dm *DNSManager) findAliDNSRecord(client *alidns.Client, domain string) (string, error) {
	rootDomain := dm.extractRootDomain(domain)
	subDomain := dm.extractSubDomain(domain)

	request := alidns.CreateDescribeDomainRecordsRequest()
	request.Scheme = "https"
	request.DomainName = rootDomain
	request.RRKeyWord = subDomain
	request.Type = "A"

	response, err := client.DescribeDomainRecords(request)
	if err != nil {
		return "", fmt.Errorf("查询DNS记录失败: %v", err)
	}

	// 查找匹配的记录
	for _, record := range response.DomainRecords.Record {
		if record.RR == subDomain && record.Type == "A" {
			return record.RecordId, nil
		}
	}

	return "", fmt.Errorf("未找到匹配的DNS记录")
}

// createTencentDNSClient 创建腾讯云DNS客户端
func (dm *DNSManager) createTencentDNSClient() (*dnspod.Client, error) {
	secretId, ok := dm.config.Config["secret_id"].(string)
	if !ok || secretId == "" {
		return nil, fmt.Errorf("腾讯云DNS缺少secret_id配置")
	}

	secretKey, ok := dm.config.Config["secret_key"].(string)
	if !ok || secretKey == "" {
		return nil, fmt.Errorf("腾讯云DNS缺少secret_key配置")
	}

	// 创建认证对象
	credential := common.NewCredential(secretId, secretKey)

	// 实例化一个client选项
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "dnspod.tencentcloudapi.com"

	// 实例化要请求产品的client对象
	client, err := dnspod.NewClient(credential, "", cpf)
	if err != nil {
		return nil, fmt.Errorf("创建腾讯云DNS客户端失败: %v", err)
	}

	return client, nil
}

// findTencentDNSRecord 查找腾讯云DNS记录
func (dm *DNSManager) findTencentDNSRecord(client *dnspod.Client, domain string) (uint64, error) {
	rootDomain := dm.extractRootDomain(domain)
	subDomain := dm.extractSubDomain(domain)

	// 实例化一个请求对象
	request := dnspod.NewDescribeRecordListRequest()
	request.Domain = common.StringPtr(rootDomain)
	request.RecordType = common.StringPtr("A")
	request.Offset = common.Uint64Ptr(0)
	request.Limit = common.Uint64Ptr(100)

	// 返回的resp是一个DescribeRecordListResponse的实例，与请求对象对应
	response, err := client.DescribeRecordList(request)
	if err != nil {
		return 0, fmt.Errorf("查询DNS记录失败: %v", err)
	}

	// 查找匹配的记录
	for _, record := range response.Response.RecordList {
		if *record.Name == subDomain && *record.Type == "A" {
			return *record.RecordId, nil
		}
	}

	return 0, fmt.Errorf("未找到匹配的DNS记录")
}

// createCloudflareClient 创建Cloudflare客户端
func (dm *DNSManager) createCloudflareClient(domain string) (*cloudflare.API, string, error) {
	var api *cloudflare.API
	var err error

	// 支持两种认证方式：API Token（推荐）或 Email + API Key
	if apiToken, ok := dm.config.Config["api_token"].(string); ok && apiToken != "" {
		// 使用API Token认证
		api, err = cloudflare.NewWithAPIToken(apiToken)
	} else {
		// 使用Email + API Key认证
		email, emailOk := dm.config.Config["email"].(string)
		apiKey, keyOk := dm.config.Config["api_key"].(string)

		if !emailOk || !keyOk || email == "" || apiKey == "" {
			return nil, "", fmt.Errorf("Cloudflare DNS缺少认证配置（需要api_token或email+api_key）")
		}

		api, err = cloudflare.New(apiKey, email)
	}

	if err != nil {
		return nil, "", fmt.Errorf("创建Cloudflare客户端失败: %v", err)
	}

	// 获取Zone ID
	rootDomain := dm.extractRootDomain(domain)
	zoneID, err := api.ZoneIDByName(rootDomain)
	if err != nil {
		return nil, "", fmt.Errorf("获取Zone ID失败: %v", err)
	}

	return api, zoneID, nil
}

// findCloudflareRecord 查找Cloudflare DNS记录
func (dm *DNSManager) findCloudflareRecord(api *cloudflare.API, zoneID, domain string) (string, error) {
	subDomain := dm.extractSubDomain(domain)
	rootDomain := dm.extractRootDomain(domain)

	var recordName string
	if subDomain == "@" {
		recordName = rootDomain
	} else {
		recordName = subDomain + "." + rootDomain
	}

	// 查询DNS记录
	params := cloudflare.ListDNSRecordsParams{
		Type: "A",
		Name: recordName,
	}
	records, _, err := api.ListDNSRecords(context.Background(), cloudflare.ZoneIdentifier(zoneID), params)
	if err != nil {
		return "", fmt.Errorf("查询DNS记录失败: %v", err)
	}

	// 查找匹配的记录
	for _, record := range records {
		if record.Name == recordName && record.Type == "A" {
			return record.ID, nil
		}
	}

	return "", fmt.Errorf("未找到匹配的DNS记录")
}
