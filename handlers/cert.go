package handlers

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"file_uploader/acme"
)

// CertHandler 证书管理处理器
type CertHandler struct {
	acmeManager *acme.Manager
}

// NewCertHandler 创建证书管理处理器
func NewCertHandler(acmeManager *acme.Manager) *CertHandler {
	return &CertHandler{
		acmeManager: acmeManager,
	}
}

// CertInfo 证书信息结构
type CertInfo struct {
	Domain     string    `json:"domain"`
	Issuer     string    `json:"issuer"`
	NotBefore  time.Time `json:"not_before"`
	NotAfter   time.Time `json:"not_after"`
	DaysLeft   int       `json:"days_left"`
	NeedsRenewal bool    `json:"needs_renewal"`
	Exists     bool      `json:"exists"`
}

// GetCertInfo 获取证书信息
func (h *CertHandler) GetCertInfo(c *gin.Context) {
	if h.acmeManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "ACME未启用",
		})
		return
	}

	certPath, _ := h.acmeManager.GetCertificatePaths()
	
	info := CertInfo{
		Exists: h.acmeManager.CertificateExists(),
	}

	if !info.Exists {
		c.JSON(http.StatusOK, gin.H{
			"certificate": info,
		})
		return
	}

	// 读取证书信息
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "读取证书文件失败",
		})
		return
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "无效的证书格式",
		})
		return
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "解析证书失败",
		})
		return
	}

	// 计算剩余天数
	daysLeft := int(cert.NotAfter.Sub(time.Now()).Hours() / 24)
	
	// 检查是否需要续期
	needsRenewal, _ := h.acmeManager.NeedsRenewal()

	info.Domain = cert.Subject.CommonName
	info.Issuer = cert.Issuer.CommonName
	info.NotBefore = cert.NotBefore
	info.NotAfter = cert.NotAfter
	info.DaysLeft = daysLeft
	info.NeedsRenewal = needsRenewal

	c.JSON(http.StatusOK, gin.H{
		"certificate": info,
	})
}

// ObtainCertificate 申请证书
func (h *CertHandler) ObtainCertificate(c *gin.Context) {
	if h.acmeManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "ACME未启用",
		})
		return
	}

	// 检查证书是否已存在
	if h.acmeManager.CertificateExists() {
		needsRenewal, err := h.acmeManager.NeedsRenewal()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "检查证书状态失败",
			})
			return
		}

		if !needsRenewal {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "证书已存在且仍然有效",
			})
			return
		}
	}

	// 申请证书
	err := h.acmeManager.ObtainCertificate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "申请证书失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "证书申请成功",
	})
}

// RenewCertificate 续期证书
func (h *CertHandler) RenewCertificate(c *gin.Context) {
	if h.acmeManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "ACME未启用",
		})
		return
	}

	// 检查证书是否存在
	if !h.acmeManager.CertificateExists() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "证书不存在，请先申请证书",
		})
		return
	}

	// 续期证书
	err := h.acmeManager.RenewCertificate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "续期证书失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "证书续期成功",
	})
}

// EnsureCertificate 确保证书可用（自动申请或续期）
func (h *CertHandler) EnsureCertificate(c *gin.Context) {
	if h.acmeManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "ACME未启用",
		})
		return
	}

	err := h.acmeManager.EnsureCertificate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "确保证书可用失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "证书已确保可用",
	})
}

// GetACMEStatus 获取ACME状态
func (h *CertHandler) GetACMEStatus(c *gin.Context) {
	if h.acmeManager == nil {
		c.JSON(http.StatusOK, gin.H{
			"acme_enabled": false,
			"message": "ACME未启用",
		})
		return
	}

	certPath, keyPath := h.acmeManager.GetCertificatePaths()
	
	status := gin.H{
		"acme_enabled": true,
		"cert_path": certPath,
		"key_path": keyPath,
		"certificate_exists": h.acmeManager.CertificateExists(),
	}

	if h.acmeManager.CertificateExists() {
		needsRenewal, err := h.acmeManager.NeedsRenewal()
		if err == nil {
			status["needs_renewal"] = needsRenewal
		}
	}

	c.JSON(http.StatusOK, status)
}
