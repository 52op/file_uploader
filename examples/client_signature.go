package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 默认配置
const (
	DEFAULT_SERVER_URL = "http://localhost:8080"
	DEFAULT_SECRET_KEY = "your-secret-key-change-this-in-production"
)

// generateSignature 生成HMAC-SHA256签名
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

// uploadFile 上传文件
func uploadFile(serverURL, filePath, customPath, storageType, secretKey string) error {
	// 生成签名
	apiPath := "/api/v1/upload"
	signature, expires, err := generateSignature(apiPath, time.Hour, secretKey)
	if err != nil {
		return fmt.Errorf("生成签名失败: %v", err)
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 创建multipart表单
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// 添加文件字段
	filename := filepath.Base(filePath)
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("创建表单文件字段失败: %v", err)
	}

	// 复制文件内容
	if _, err := io.Copy(fileWriter, file); err != nil {
		return fmt.Errorf("复制文件内容失败: %v", err)
	}

	// 添加可选参数
	if customPath != "" {
		if err := writer.WriteField("path", customPath); err != nil {
			return fmt.Errorf("添加path字段失败: %v", err)
		}
	}

	if storageType != "" {
		if err := writer.WriteField("storage", storageType); err != nil {
			return fmt.Errorf("添加storage字段失败: %v", err)
		}
	}

	// 关闭writer
	writer.Close()

	// 构建请求URL
	url := fmt.Sprintf("%s%s?expires=%d&signature=%s", serverURL, apiPath, expires, signature)

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, &requestBody)
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 设置Content-Type
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	fmt.Printf("上传响应 (状态码: %d):\n%s\n", resp.StatusCode, string(body))
	return nil
}

// getFileInfo 获取文件信息
func getFileInfo(serverURL, filename, storageType, secretKey string) error {
	// 生成签名
	apiPath := fmt.Sprintf("/api/v1/files/%s", filename)
	signature, expires, err := generateSignature(apiPath, time.Hour, secretKey)
	if err != nil {
		return fmt.Errorf("生成签名失败: %v", err)
	}

	// 构建请求URL
	url := fmt.Sprintf("%s%s?expires=%d&signature=%s", serverURL, apiPath, expires, signature)
	if storageType != "" {
		url += fmt.Sprintf("&storage=%s", storageType)
	}

	// 发送GET请求
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	fmt.Printf("文件信息响应 (状态码: %d):\n%s\n", resp.StatusCode, string(body))
	return nil
}

// deleteFile 删除文件
func deleteFile(serverURL, filename, storageType, secretKey string) error {
	// 生成签名
	apiPath := fmt.Sprintf("/api/v1/files/%s", filename)
	signature, expires, err := generateSignature(apiPath, time.Hour, secretKey)
	if err != nil {
		return fmt.Errorf("生成签名失败: %v", err)
	}

	// 构建请求URL
	url := fmt.Sprintf("%s%s?expires=%d&signature=%s", serverURL, apiPath, expires, signature)
	if storageType != "" {
		url += fmt.Sprintf("&storage=%s", storageType)
	}

	// 创建DELETE请求
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 发送请求
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	fmt.Printf("删除响应 (状态码: %d):\n%s\n", resp.StatusCode, string(body))
	return nil
}

// healthCheck 健康检查
func healthCheck(serverURL string) error {
	url := fmt.Sprintf("%s/health", serverURL)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("健康检查失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	fmt.Printf("健康检查响应 (状态码: %d):\n%s\n", resp.StatusCode, string(body))
	return nil
}

// printUsage 打印使用说明
func printUsage() {
	fmt.Println("文件上传客户端 - 支持命令行参数")
	fmt.Println()
	fmt.Println("使用方法:")
	fmt.Println("  client_signature [选项] <命令> [参数...]")
	fmt.Println()
	fmt.Println("全局选项:")
	fmt.Println("  -u, --url <URL>        服务器地址 (默认: http://localhost:8080)")
	fmt.Println("  -k, --key <KEY>        签名密钥 (默认: your-secret-key-change-this-in-production)")
	fmt.Println("  -s, --storage <NAME>   存储类型 (可选)")
	fmt.Println("  -h, --help             显示帮助信息")
	fmt.Println()
	fmt.Println("命令:")
	fmt.Println("  health                 健康检查")
	fmt.Println("  upload <文件路径>       上传文件")
	fmt.Println("    -p, --path <PATH>    自定义文件路径 (可选)")
	fmt.Println("  info <文件名>          获取文件信息")
	fmt.Println("  delete <文件名>        删除文件")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  # 健康检查")
	fmt.Println("  client_signature -u https://wuhu-cdn.hxljzz.com:8443 health")
	fmt.Println()
	fmt.Println("  # 上传文件到默认存储")
	fmt.Println("  client_signature -u https://wuhu-cdn.hxljzz.com:8443 -k secret-key upload test.txt")
	fmt.Println()
	fmt.Println("  # 上传文件到指定存储和路径")
	fmt.Println("  client_signature -u https://wuhu-cdn.hxljzz.com:8443 -k secret-key -s local_backup upload test.txt -p /backup/2025/test.txt")
	fmt.Println()
	fmt.Println("  # 获取文件信息")
	fmt.Println("  client_signature -u https://wuhu-cdn.hxljzz.com:8443 -k secret-key -s local_backup info test.txt")
	fmt.Println()
	fmt.Println("  # 删除文件")
	fmt.Println("  client_signature -u https://wuhu-cdn.hxljzz.com:8443 -k secret-key -s local_backup delete test.txt")
}

func main() {
	// 定义命令行参数
	var (
		serverURL   = flag.String("u", DEFAULT_SERVER_URL, "服务器地址")
		secretKey   = flag.String("k", DEFAULT_SECRET_KEY, "签名密钥")
		storageType = flag.String("s", "", "存储类型")
		customPath  = flag.String("p", "", "自定义文件路径 (仅用于upload命令)")
		showHelp    = flag.Bool("h", false, "显示帮助信息")
	)

	// 支持长选项
	flag.StringVar(serverURL, "url", DEFAULT_SERVER_URL, "服务器地址")
	flag.StringVar(secretKey, "key", DEFAULT_SECRET_KEY, "签名密钥")
	flag.StringVar(storageType, "storage", "", "存储类型")
	flag.StringVar(customPath, "path", "", "自定义文件路径 (仅用于upload命令)")
	flag.BoolVar(showHelp, "help", false, "显示帮助信息")

	// 解析命令行参数
	flag.Parse()

	// 显示帮助信息
	if *showHelp {
		printUsage()
		return
	}

	// 获取剩余参数（命令和参数）
	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("错误: 请指定命令")
		fmt.Println()
		printUsage()
		os.Exit(1)
	}

	command := args[0]

	// 清理服务器URL（移除末尾的斜杠）
	*serverURL = strings.TrimRight(*serverURL, "/")

	switch command {
	case "health":
		fmt.Printf("正在检查服务器健康状态: %s\n", *serverURL)
		if err := healthCheck(*serverURL); err != nil {
			fmt.Printf("错误: %v\n", err)
			os.Exit(1)
		}

	case "upload":
		if len(args) < 2 {
			fmt.Println("错误: 请提供要上传的文件路径")
			fmt.Println("使用方法: client_signature upload <文件路径>")
			os.Exit(1)
		}
		filePath := args[1]

		fmt.Printf("正在上传文件: %s\n", filePath)
		fmt.Printf("服务器: %s\n", *serverURL)
		if *storageType != "" {
			fmt.Printf("存储类型: %s\n", *storageType)
		}
		if *customPath != "" {
			fmt.Printf("自定义路径: %s\n", *customPath)
		}

		if err := uploadFile(*serverURL, filePath, *customPath, *storageType, *secretKey); err != nil {
			fmt.Printf("上传失败: %v\n", err)
			os.Exit(1)
		}

	case "info":
		if len(args) < 2 {
			fmt.Println("错误: 请提供文件名")
			fmt.Println("使用方法: client_signature info <文件名>")
			os.Exit(1)
		}
		filename := args[1]

		fmt.Printf("正在获取文件信息: %s\n", filename)
		fmt.Printf("服务器: %s\n", *serverURL)
		if *storageType != "" {
			fmt.Printf("存储类型: %s\n", *storageType)
		}

		if err := getFileInfo(*serverURL, filename, *storageType, *secretKey); err != nil {
			fmt.Printf("获取文件信息失败: %v\n", err)
			os.Exit(1)
		}

	case "delete":
		if len(args) < 2 {
			fmt.Println("错误: 请提供要删除的文件名")
			fmt.Println("使用方法: client_signature delete <文件名>")
			os.Exit(1)
		}
		filename := args[1]

		fmt.Printf("正在删除文件: %s\n", filename)
		fmt.Printf("服务器: %s\n", *serverURL)
		if *storageType != "" {
			fmt.Printf("存储类型: %s\n", *storageType)
		}

		if err := deleteFile(*serverURL, filename, *storageType, *secretKey); err != nil {
			fmt.Printf("删除失败: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Printf("错误: 未知命令 '%s'\n", command)
		fmt.Println()
		printUsage()
		os.Exit(1)
	}
}
