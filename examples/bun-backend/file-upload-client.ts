import crypto from 'crypto';
import FormData from 'form-data';
import axios from 'axios';
import fs from 'fs';

export interface FileUploadConfig {
  serverUrl: string;
  secretKey: string;
  defaultStorage?: string;
}

export interface UploadOptions {
  storage?: string;
  customPath?: string;
}

export interface FileInfo {
  filename: string;
  size: number;
  content_type: string;
  url: string;
  upload_time: number;
}

export interface UploadResponse {
  success: boolean;
  file_info: FileInfo;
  message: string;
  storage_type: string;
}

export class FileUploadClient {
  private config: FileUploadConfig;

  constructor(config: FileUploadConfig) {
    this.config = config;
  }

  /**
   * 生成HMAC-SHA256签名
   */
  private generateSignature(path: string, expiryDuration: number = 3600): { signature: string; expires: number } {
    const expires = Math.floor(Date.now() / 1000) + expiryDuration;
    const expiresStr = expires.toString();

    // 生成8字节随机数
    const nonce = crypto.randomBytes(8);
    const nonceHex = nonce.toString('hex');

    // 构建签名内容：路径 + 过期时间 + 随机数
    const message = path + expiresStr + nonceHex;

    // 计算HMAC-SHA256
    const hmac = crypto.createHmac('sha256', this.config.secretKey);
    hmac.update(message);
    const hmacBytes = hmac.digest();

    // 组合最终签名：HMAC + 随机数
    const finalSignature = Buffer.concat([hmacBytes, nonce]);
    const signature = finalSignature.toString('hex');

    return { signature, expires };
  }

  /**
   * 上传文件
   */
  async uploadFile(filePath: string, options: UploadOptions = {}): Promise<UploadResponse> {
    const apiPath = '/api/v1/upload';
    const { signature, expires } = this.generateSignature(apiPath);

    // 创建表单数据
    const formData = new FormData();
    formData.append('file', fs.createReadStream(filePath));

    // 添加可选参数
    if (options.customPath) {
      formData.append('path', options.customPath);
    }
    if (options.storage) {
      formData.append('storage', options.storage);
    }

    // 构建请求URL
    const url = `${this.config.serverUrl}${apiPath}?expires=${expires}&signature=${signature}`;

    try {
      const response = await axios.post<UploadResponse>(url, formData, {
        headers: {
          ...formData.getHeaders(),
        },
        timeout: 30000,
      });

      return response.data;
    } catch (error) {
      if (axios.isAxiosError(error)) {
        throw new Error(`Upload failed: ${error.response?.data?.error || error.message}`);
      }
      throw error;
    }
  }

  /**
   * 上传Buffer数据
   */
  async uploadBuffer(buffer: Buffer, filename: string, options: UploadOptions = {}): Promise<UploadResponse> {
    const apiPath = '/api/v1/upload';
    const { signature, expires } = this.generateSignature(apiPath);

    // 创建表单数据
    const formData = new FormData();
    formData.append('file', buffer, filename);

    // 添加可选参数
    if (options.customPath) {
      formData.append('path', options.customPath);
    }
    if (options.storage) {
      formData.append('storage', options.storage);
    }

    // 构建请求URL
    const url = `${this.config.serverUrl}${apiPath}?expires=${expires}&signature=${signature}`;

    try {
      const response = await axios.post<UploadResponse>(url, formData, {
        headers: {
          ...formData.getHeaders(),
        },
        timeout: 30000,
      });

      return response.data;
    } catch (error) {
      if (axios.isAxiosError(error)) {
        throw new Error(`Upload failed: ${error.response?.data?.error || error.message}`);
      }
      throw error;
    }
  }

  /**
   * 生成静态文件访问签名URL
   */
  generateStaticFileUrl(filePath: string, expiryDuration: number = 3600): string {
    const { signature, expires } = this.generateSignature(filePath, expiryDuration);
    return `${this.config.serverUrl}${filePath}?expires=${expires}&signature=${signature}`;
  }

  /**
   * 获取文件信息
   */
  async getFileInfo(filename: string, storage?: string): Promise<any> {
    const apiPath = `/api/v1/files/${filename}`;
    const { signature, expires } = this.generateSignature(apiPath);

    let url = `${this.config.serverUrl}${apiPath}?expires=${expires}&signature=${signature}`;
    if (storage) {
      url += `&storage=${storage}`;
    }

    try {
      const response = await axios.get(url);
      return response.data;
    } catch (error) {
      if (axios.isAxiosError(error)) {
        throw new Error(`Get file info failed: ${error.response?.data?.error || error.message}`);
      }
      throw error;
    }
  }

  /**
   * 删除文件
   */
  async deleteFile(filename: string, storage?: string): Promise<any> {
    const apiPath = `/api/v1/files/${filename}`;
    const { signature, expires } = this.generateSignature(apiPath);

    let url = `${this.config.serverUrl}${apiPath}?expires=${expires}&signature=${signature}`;
    if (storage) {
      url += `&storage=${storage}`;
    }

    try {
      const response = await axios.delete(url);
      return response.data;
    } catch (error) {
      if (axios.isAxiosError(error)) {
        throw new Error(`Delete file failed: ${error.response?.data?.error || error.message}`);
      }
      throw error;
    }
  }

  /**
   * 健康检查
   */
  async healthCheck(): Promise<any> {
    const url = `${this.config.serverUrl}/health`;
    try {
      const response = await axios.get(url, { timeout: 5000 });
      return response.data;
    } catch (error) {
      if (axios.isAxiosError(error)) {
        throw new Error(`Health check failed: ${error.message}`);
      }
      throw error;
    }
  }
}

// 导出单例实例
export const fileUploadClient = new FileUploadClient({
  serverUrl: process.env.FILE_UPLOAD_SERVER_URL || 'https://wuhu-cdn.hxljzz.com:8443',
  secretKey: process.env.FILE_UPLOAD_SECRET_KEY || 'your-secret-key-change-this-in-production',
  defaultStorage: process.env.FILE_UPLOAD_DEFAULT_STORAGE,
});
