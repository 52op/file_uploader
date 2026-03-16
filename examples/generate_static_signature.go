package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"
)

func generateSignature(path string, expiryDuration time.Duration, secretKey string) (string, int64, error) {
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
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(signContent))
	hmacBytes := h.Sum(nil)

	// 组合最终签名：HMAC + 随机数
	finalSignature := append(hmacBytes, nonce...)

	// 转换为十六进制字符串
	signatureHex := hex.EncodeToString(finalSignature)

	return signatureHex, expires, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: generate_static_signature <文件路径> [密钥]")
		fmt.Println("示例: generate_static_signature /backup/test/2025/custom_path.txt")
		fmt.Println("      generate_static_signature /backup/test/2025/custom_path.txt your-secret-key")
		return
	}

	filePath := os.Args[1]
	secretKey := "your-secret-key-change-this-in-production"
	
	if len(os.Args) > 2 {
		secretKey = os.Args[2]
	}

	signature, expires, err := generateSignature(filePath, time.Hour, secretKey)
	if err != nil {
		fmt.Printf("生成签名失败: %v\n", err)
		return
	}

	baseURL := "https://wuhu-cdn.hxljzz.com:8443"
	signedURL := fmt.Sprintf("%s%s?expires=%d&signature=%s", baseURL, filePath, expires, signature)

	fmt.Printf("文件路径: %s\n", filePath)
	fmt.Printf("过期时间: %d (%s)\n", expires, time.Unix(expires, 0).Format("2006-01-02 15:04:05"))
	fmt.Printf("签名: %s\n", signature)
	fmt.Printf("完整URL: %s\n", signedURL)
	fmt.Printf("\n测试命令:\n")
	fmt.Printf("curl \"%s\"\n", signedURL)
}
