package acme

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// IPDetector 外网IP检测器
type IPDetector struct {
	apis    []string
	timeout time.Duration
}

// NewIPDetector 创建IP检测器
func NewIPDetector(apis []string, timeout time.Duration) *IPDetector {
	if len(apis) == 0 {
		// 默认API列表
		apis = []string{
			"https://api.ipify.org",
			"https://ipv4.icanhazip.com",
			"https://checkip.amazonaws.com",
			"https://ipinfo.io/ip",
			"https://api.myip.com",
		}
	}

	if timeout <= 0 {
		timeout = 10 * time.Second // 默认超时时间
	}

	return &IPDetector{
		apis:    apis,
		timeout: timeout,
	}
}

// DetectExternalIP 检测外网IP地址
func (d *IPDetector) DetectExternalIP() (string, error) {
	var lastErr error
	
	for i, api := range d.apis {
		log.Printf("[IP检测] 尝试API %d/%d: %s", i+1, len(d.apis), api)
		
		ip, err := d.getIPFromAPI(api)
		if err != nil {
			log.Printf("[IP检测] API %s 失败: %v", api, err)
			lastErr = err
			continue
		}
		
		// 验证IP格式
		if net.ParseIP(ip) == nil {
			log.Printf("[IP检测] API %s 返回无效IP: %s", api, ip)
			lastErr = fmt.Errorf("无效IP格式: %s", ip)
			continue
		}
		
		log.Printf("[IP检测] 成功检测到外网IP: %s (来源: %s)", ip, api)
		return ip, nil
	}
	
	if lastErr != nil {
		return "", fmt.Errorf("所有IP检测API都失败，最后错误: %v", lastErr)
	}
	
	return "", fmt.Errorf("无法检测外网IP")
}

// getIPFromAPI 从指定API获取IP
func (d *IPDetector) getIPFromAPI(apiURL string) (string, error) {
	client := &http.Client{
		Timeout: d.timeout,
	}
	
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP状态码: %d", resp.StatusCode)
	}
	
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}
	
	ip := strings.TrimSpace(string(body))
	
	// 处理一些API返回JSON格式的情况
	if strings.Contains(ip, "{") && strings.Contains(ip, "}") {
		// 简单的JSON解析，提取IP字段
		if strings.Contains(ip, `"ip"`) {
			parts := strings.Split(ip, `"ip"`)
			if len(parts) > 1 {
				ipPart := strings.Split(parts[1], `"`)[2]
				if ipPart != "" {
					ip = ipPart
				}
			}
		}
	}
	
	return ip, nil
}

// IsValidIP 验证IP地址格式
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsPrivateIP 检查是否为私有IP
func IsPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	
	// 检查是否为私有IP段
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
	}
	
	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsedIP) {
			return true
		}
	}
	
	return false
}
