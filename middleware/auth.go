package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"file_uploader/config"
)

// SignatureAuth HMAC-SHA256签名验证中间件
func SignatureAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetConfig()
		if cfg == nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "服务器配置错误",
			})
			c.Abort()
			return
		}

		// 获取签名参数
		expiresStr := c.Query("expires")
		signature := c.Query("signature")

		// 检查必需参数
		if expiresStr == "" || signature == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "缺少签名参数",
				"details": "需要提供expires和signature参数",
			})
			c.Abort()
			return
		}

		// 解析过期时间
		expires, err := strconv.ParseInt(expiresStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "无效的过期时间格式",
			})
			c.Abort()
			return
		}

		// 检查签名是否过期
		if time.Now().Unix() > expires {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "签名已过期",
			})
			c.Abort()
			return
		}

		// 标准化路径处理：使用URL解码后的路径进行签名验证
		// 规则：客户端生成签名时必须使用未编码的原始路径
		requestPath := c.Request.URL.Path

		// 如果路径包含URL编码字符，先解码
		signaturePath := requestPath
		if decodedPath, err := url.QueryUnescape(requestPath); err == nil {
			signaturePath = decodedPath
		}

		// 验证签名
		if !verifySignature(signaturePath, expiresStr, signature, cfg.Security.SecretKey) {
			log.Printf("[签名验证失败] 使用路径: %s", signaturePath)
			log.Printf("[签名验证失败] 原始请求路径: %s", requestPath)
			log.Printf("[签名验证失败] Expires: %s, Signature: %s", expiresStr, signature)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "签名验证失败",
				"details": "请确保使用未编码的原始路径生成签名",
			})
			c.Abort()
			return
		}

		// 签名验证通过，继续处理请求
		c.Next()
	}
}

// verifySignature 验证HMAC-SHA256签名
// 签名格式：HMAC(32字节) + 随机数(8字节) = 40字节总长度
// 签名内容：文件路径 + 过期时间 + 随机数（十六进制）
func verifySignature(path, expires, signature, secretKey string) bool {
	// 解码签名
	signatureBytes, err := hex.DecodeString(signature)
	if err != nil {
		log.Printf("[签名验证] 签名解码失败: %v", err)
		return false
	}

	// 检查签名长度：必须是40字节
	if len(signatureBytes) != 40 {
		log.Printf("[签名验证] 签名长度错误: %d, 期望: 40", len(signatureBytes))
		return false
	}

	// 提取HMAC和随机数
	hmacBytes := signatureBytes[:32]
	nonceBytes := signatureBytes[32:]

	// 构建签名内容：路径 + 过期时间 + 随机数
	signContent := path + expires + hex.EncodeToString(nonceBytes)
	log.Printf("[签名验证] 签名内容: %s", signContent)

	// 计算期望的HMAC
	expectedHMAC := computeHMAC(signContent, secretKey)
	log.Printf("[签名验证] 期望HMAC: %x", expectedHMAC)
	log.Printf("[签名验证] 实际HMAC: %x", hmacBytes)

	// 使用恒定时间比较防止时序攻击
	result := hmac.Equal(hmacBytes, expectedHMAC)
	log.Printf("[签名验证] 验证结果: %v", result)
	return result
}



// computeHMAC 计算HMAC-SHA256
func computeHMAC(message, key string) []byte {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(message))
	return h.Sum(nil)
}

// GenerateSignature 生成签名（供客户端使用的辅助函数）
// 这个函数通常在客户端（如网站后端）中使用
func GenerateSignature(path string, expiryDuration time.Duration, secretKey string) (string, int64, error) {
	// 计算过期时间戳
	expires := time.Now().Add(expiryDuration).Unix()
	expiresStr := strconv.FormatInt(expires, 10)

	// 生成8字节随机数
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return "", 0, fmt.Errorf("生成随机数失败: %v", err)
	}

	// 构建签名内容：路径 + 过期时间 + 随机数
	signContent := path + expiresStr + hex.EncodeToString(nonce)

	// 计算HMAC
	hmacBytes := computeHMAC(signContent, secretKey)

	// 组合最终签名：HMAC + 随机数
	finalSignature := append(hmacBytes, nonce...)

	// 转换为十六进制字符串
	signatureHex := hex.EncodeToString(finalSignature)

	return signatureHex, expires, nil
}

// ValidateSignatureParams 验证签名参数的辅助函数
func ValidateSignatureParams(expires, signature string) error {
	if expires == "" {
		return fmt.Errorf("过期时间不能为空")
	}

	if signature == "" {
		return fmt.Errorf("签名不能为空")
	}

	// 验证过期时间格式
	expiresInt, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return fmt.Errorf("无效的过期时间格式: %v", err)
	}

	// 检查过期时间是否合理（不能是过去的时间，也不能太远的未来）
	now := time.Now().Unix()
	if expiresInt <= now {
		return fmt.Errorf("过期时间不能是过去的时间")
	}

	// 限制最大有效期为24小时
	maxExpiry := now + 24*3600
	if expiresInt > maxExpiry {
		return fmt.Errorf("过期时间不能超过24小时")
	}

	// 验证签名格式（应该是80个十六进制字符）
	if len(signature) != 80 {
		return fmt.Errorf("签名长度无效")
	}

	// 验证签名是否为有效的十六进制字符串
	if _, err := hex.DecodeString(signature); err != nil {
		return fmt.Errorf("签名格式无效: %v", err)
	}

	return nil
}

// SignatureInfo 签名信息结构（用于调试和日志）
type SignatureInfo struct {
	Path      string `json:"path"`       // 请求路径
	Expires   int64  `json:"expires"`    // 过期时间戳
	Signature string `json:"signature"`  // 签名值
	Valid     bool   `json:"valid"`      // 是否有效
	Error     string `json:"error,omitempty"` // 错误信息
}

// GetSignatureInfo 获取签名信息（用于调试）
func GetSignatureInfo(c *gin.Context) *SignatureInfo {
	info := &SignatureInfo{
		Path: c.Request.URL.Path,
	}

	expiresStr := c.Query("expires")
	signature := c.Query("signature")

	info.Signature = signature

	if expires, err := strconv.ParseInt(expiresStr, 10, 64); err == nil {
		info.Expires = expires
	}

	// 验证签名参数
	if err := ValidateSignatureParams(expiresStr, signature); err != nil {
		info.Valid = false
		info.Error = err.Error()
		return info
	}

	// 验证签名
	cfg := config.GetConfig()
	if cfg != nil {
		info.Valid = verifySignature(info.Path, expiresStr, signature, cfg.Security.SecretKey)
		if !info.Valid {
			info.Error = "签名验证失败"
		}
	} else {
		info.Valid = false
		info.Error = "服务器配置错误"
	}

	return info
}
