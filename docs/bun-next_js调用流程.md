# Bun/Next.js 调用流程指南

本文档详细介绍如何在 Bun 后端和 Next.js 前端项目中集成 File Uploader 服务。

## 🎯 概述

File Uploader 提供了完整的文件上传解决方案，支持：
- 智能缩略图生成
- 多存储后端支持
- 安全签名认证
- 实时访问统计
- 防盗链保护

## 🔧 Bun 后端集成

### 1. 安装依赖

```bash
bun add crypto
```

### 2. 签名生成工具

创建 `utils/signature.ts`：

```typescript
import { createHmac } from 'crypto';

export interface SignatureResult {
  signature: string;
  timestamp: number;
  expires: number;
}

/**
 * 生成文件上传签名
 * @param secretKey 密钥
 * @param method HTTP方法
 * @param path API路径
 * @param expiryMinutes 过期时间（分钟）
 */
export function generateUploadSignature(
  secretKey: string,
  method: string = 'POST',
  path: string = '/api/v1/upload',
  expiryMinutes: number = 60
): SignatureResult {
  const timestamp = Math.floor(Date.now() / 1000);
  const expires = timestamp + (expiryMinutes * 60);
  
  const message = `${method}|${path}|${expires}`;
  const signature = createHmac('sha256', secretKey)
    .update(message)
    .digest('hex');
  
  return {
    signature,
    timestamp,
    expires
  };
}

/**
 * 生成静态文件访问签名
 * @param secretKey 密钥
 * @param filePath 文件路径
 * @param expiryMinutes 过期时间（分钟）
 */
export function generateStaticSignature(
  secretKey: string,
  filePath: string,
  expiryMinutes: number = 60
): SignatureResult {
  const timestamp = Math.floor(Date.now() / 1000);
  const expires = timestamp + (expiryMinutes * 60);
  
  const message = `${filePath}|${expires}`;
  const signature = createHmac('sha256', secretKey)
    .update(message)
    .digest('hex');
  
  return {
    signature,
    timestamp,
    expires
  };
}
```

### 3. 文件上传服务

创建 `services/fileUpload.ts`：

```typescript
import { generateUploadSignature } from '../utils/signature';

export interface UploadConfig {
  fileUploaderUrl: string;
  secretKey: string;
  storage?: string;
  path?: string;
}

export interface UploadResult {
  success: boolean;
  url: string;
  thumbnail_url?: string;
  filename: string;
  size: number;
  upload_time: number;
  storage_type: string;
}

export class FileUploadService {
  private config: UploadConfig;

  constructor(config: UploadConfig) {
    this.config = config;
  }

  /**
   * 上传单个文件
   */
  async uploadFile(file: File, customPath?: string): Promise<UploadResult> {
    const { signature, expires } = generateUploadSignature(
      this.config.secretKey,
      'POST',
      '/api/v1/upload'
    );

    const formData = new FormData();
    formData.append('file', file);
    
    if (customPath || this.config.path) {
      formData.append('path', customPath || this.config.path!);
    }
    
    if (this.config.storage) {
      formData.append('storage', this.config.storage);
    }

    const url = `${this.config.fileUploaderUrl}/api/v1/upload?expires=${expires}&signature=${signature}`;
    
    const response = await fetch(url, {
      method: 'POST',
      body: formData,
    });

    if (!response.ok) {
      throw new Error(`Upload failed: ${response.statusText}`);
    }

    return await response.json();
  }

  /**
   * 批量上传文件
   */
  async uploadFiles(files: File[], storage?: string): Promise<any> {
    const { signature, expires } = generateUploadSignature(
      this.config.secretKey,
      'POST',
      '/api/v1/batch/upload'
    );

    const formData = new FormData();
    files.forEach(file => {
      formData.append('files', file);
    });
    
    if (storage || this.config.storage) {
      formData.append('storage', storage || this.config.storage!);
    }

    const url = `${this.config.fileUploaderUrl}/api/v1/batch/upload?expires=${expires}&signature=${signature}`;
    
    const response = await fetch(url, {
      method: 'POST',
      body: formData,
    });

    if (!response.ok) {
      throw new Error(`Batch upload failed: ${response.statusText}`);
    }

    return await response.json();
  }

  /**
   * 删除文件
   */
  async deleteFile(filename: string, storage?: string): Promise<any> {
    const { signature, expires } = generateUploadSignature(
      this.config.secretKey,
      'DELETE',
      `/api/v1/files/${filename}`
    );

    let url = `${this.config.fileUploaderUrl}/api/v1/files/${filename}?expires=${expires}&signature=${signature}`;
    
    if (storage || this.config.storage) {
      url += `&storage=${storage || this.config.storage}`;
    }
    
    const response = await fetch(url, {
      method: 'DELETE',
    });

    if (!response.ok) {
      throw new Error(`Delete failed: ${response.statusText}`);
    }

    return await response.json();
  }
}
```

### 4. API 路由示例

创建 `routes/upload.ts`：

```typescript
import { Hono } from 'hono';
import { FileUploadService } from '../services/fileUpload';

const app = new Hono();

// 初始化文件上传服务
const uploadService = new FileUploadService({
  fileUploaderUrl: process.env.FILE_UPLOADER_URL || 'http://localhost:8080',
  secretKey: process.env.FILE_UPLOADER_SECRET || 'your-secret-key',
  storage: 'protected_images', // 可选：默认存储
});

// 单文件上传
app.post('/upload', async (c) => {
  try {
    const formData = await c.req.formData();
    const file = formData.get('file') as File;
    const path = formData.get('path') as string;
    
    if (!file) {
      return c.json({ error: 'No file provided' }, 400);
    }

    const result = await uploadService.uploadFile(file, path);
    return c.json(result);
  } catch (error) {
    console.error('Upload error:', error);
    return c.json({ error: 'Upload failed' }, 500);
  }
});

// 批量上传
app.post('/upload/batch', async (c) => {
  try {
    const formData = await c.req.formData();
    const files = formData.getAll('files') as File[];
    const storage = formData.get('storage') as string;
    
    if (!files.length) {
      return c.json({ error: 'No files provided' }, 400);
    }

    const result = await uploadService.uploadFiles(files, storage);
    return c.json(result);
  } catch (error) {
    console.error('Batch upload error:', error);
    return c.json({ error: 'Batch upload failed' }, 500);
  }
});

// 删除文件
app.delete('/files/:filename', async (c) => {
  try {
    const filename = c.req.param('filename');
    const storage = c.req.query('storage');
    
    const result = await uploadService.deleteFile(filename, storage);
    return c.json(result);
  } catch (error) {
    console.error('Delete error:', error);
    return c.json({ error: 'Delete failed' }, 500);
  }
});

export default app;
```

## ⚛️ Next.js 前端集成

### 1. 安装依赖

```bash
npm install axios
# 或
bun add axios
```

### 2. 上传组件

创建 `components/FileUploader.tsx`：

```tsx
'use client';

import React, { useState, useCallback } from 'react';
import axios from 'axios';

interface UploadResult {
  success: boolean;
  url: string;
  thumbnail_url?: string;
  filename: string;
  size: number;
}

interface FileUploaderProps {
  onUploadSuccess?: (result: UploadResult) => void;
  onUploadError?: (error: string) => void;
  accept?: string;
  maxSize?: number; // MB
  storage?: string;
  path?: string;
}

export default function FileUploader({
  onUploadSuccess,
  onUploadError,
  accept = "image/*",
  maxSize = 10,
  storage,
  path
}: FileUploaderProps) {
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);

  const uploadFile = useCallback(async (file: File) => {
    if (file.size > maxSize * 1024 * 1024) {
      onUploadError?.(`文件大小不能超过 ${maxSize}MB`);
      return;
    }

    setUploading(true);

    try {
      const formData = new FormData();
      formData.append('file', file);

      if (storage) {
        formData.append('storage', storage);
      }

      if (path) {
        formData.append('path', path);
      }

      const response = await axios.post('/api/upload', formData, {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      });

      onUploadSuccess?.(response.data);
    } catch (error) {
      console.error('Upload error:', error);
      onUploadError?.('上传失败，请重试');
    } finally {
      setUploading(false);
    }
  }, [maxSize, storage, path, onUploadSuccess, onUploadError]);

  const handleFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      uploadFile(file);
    }
  };

  const handleDrop = (event: React.DragEvent) => {
    event.preventDefault();
    setDragOver(false);

    const file = event.dataTransfer.files[0];
    if (file) {
      uploadFile(file);
    }
  };

  const handleDragOver = (event: React.DragEvent) => {
    event.preventDefault();
    setDragOver(true);
  };

  const handleDragLeave = () => {
    setDragOver(false);
  };

  return (
    <div
      className={`border-2 border-dashed rounded-lg p-8 text-center transition-colors ${
        dragOver
          ? 'border-blue-500 bg-blue-50'
          : 'border-gray-300 hover:border-gray-400'
      }`}
      onDrop={handleDrop}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
    >
      {uploading ? (
        <div className="flex flex-col items-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
          <p className="mt-2 text-gray-600">上传中...</p>
        </div>
      ) : (
        <div>
          <input
            type="file"
            accept={accept}
            onChange={handleFileSelect}
            className="hidden"
            id="file-upload"
          />
          <label
            htmlFor="file-upload"
            className="cursor-pointer text-blue-500 hover:text-blue-600"
          >
            点击选择文件或拖拽文件到此处
          </label>
          <p className="mt-2 text-sm text-gray-500">
            支持的格式：{accept}，最大 {maxSize}MB
          </p>
        </div>
      )}
    </div>
  );
}
```

### 3. 图片预览组件

创建 `components/ImagePreview.tsx`：

```tsx
'use client';

import React, { useState } from 'react';
import Image from 'next/image';

interface ImagePreviewProps {
  src: string;
  thumbnailSrc?: string;
  alt: string;
  width?: number;
  height?: number;
  className?: string;
}

export default function ImagePreview({
  src,
  thumbnailSrc,
  alt,
  width = 300,
  height = 200,
  className = ""
}: ImagePreviewProps) {
  const [showFullSize, setShowFullSize] = useState(false);
  const [imageError, setImageError] = useState(false);

  // 优先使用缩略图，如果没有则使用原图
  const displaySrc = thumbnailSrc || src;

  if (imageError) {
    return (
      <div
        className={`flex items-center justify-center bg-gray-200 rounded ${className}`}
        style={{ width, height }}
      >
        <span className="text-gray-500">图片加载失败</span>
      </div>
    );
  }

  return (
    <>
      <div className={`relative cursor-pointer ${className}`}>
        <Image
          src={displaySrc}
          alt={alt}
          width={width}
          height={height}
          className="rounded object-cover hover:opacity-90 transition-opacity"
          onClick={() => setShowFullSize(true)}
          onError={() => setImageError(true)}
        />
        {thumbnailSrc && (
          <div className="absolute top-2 right-2 bg-black bg-opacity-50 text-white text-xs px-2 py-1 rounded">
            缩略图
          </div>
        )}
      </div>

      {/* 全尺寸预览模态框 */}
      {showFullSize && (
        <div
          className="fixed inset-0 bg-black bg-opacity-75 flex items-center justify-center z-50"
          onClick={() => setShowFullSize(false)}
        >
          <div className="relative max-w-4xl max-h-4xl">
            <Image
              src={src}
              alt={alt}
              width={800}
              height={600}
              className="max-w-full max-h-full object-contain"
              onClick={(e) => e.stopPropagation()}
            />
            <button
              className="absolute top-4 right-4 text-white text-2xl hover:text-gray-300"
              onClick={() => setShowFullSize(false)}
            >
              ×
            </button>
          </div>
        </div>
      )}
    </>
  );
}
```

### 4. 使用示例

创建 `app/upload/page.tsx`：

```tsx
'use client';

import React, { useState } from 'react';
import FileUploader from '@/components/FileUploader';
import ImagePreview from '@/components/ImagePreview';

interface UploadedFile {
  url: string;
  thumbnail_url?: string;
  filename: string;
  size: number;
}

export default function UploadPage() {
  const [uploadedFiles, setUploadedFiles] = useState<UploadedFile[]>([]);
  const [error, setError] = useState<string>('');

  const handleUploadSuccess = (result: any) => {
    setUploadedFiles(prev => [...prev, result]);
    setError('');
  };

  const handleUploadError = (errorMsg: string) => {
    setError(errorMsg);
  };

  return (
    <div className="container mx-auto p-6">
      <h1 className="text-3xl font-bold mb-6">文件上传</h1>

      {error && (
        <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded mb-4">
          {error}
        </div>
      )}

      <div className="mb-8">
        <FileUploader
          onUploadSuccess={handleUploadSuccess}
          onUploadError={handleUploadError}
          accept="image/*"
          maxSize={10}
          storage="protected_images"
        />
      </div>

      {uploadedFiles.length > 0 && (
        <div>
          <h2 className="text-2xl font-semibold mb-4">已上传的文件</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {uploadedFiles.map((file, index) => (
              <div key={index} className="border rounded-lg p-4">
                <ImagePreview
                  src={file.url}
                  thumbnailSrc={file.thumbnail_url}
                  alt={file.filename}
                  width={250}
                  height={200}
                  className="mb-2"
                />
                <p className="text-sm font-medium truncate">{file.filename}</p>
                <p className="text-xs text-gray-500">
                  {(file.size / 1024).toFixed(1)} KB
                </p>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
```

## 🔧 环境配置

### Bun 环境变量

创建 `.env`：

```env
FILE_UPLOADER_URL=http://localhost:8080
FILE_UPLOADER_SECRET=your-secret-key-change-this-in-production
```

### Next.js 环境变量

创建 `.env.local`：

```env
NEXT_PUBLIC_API_URL=http://localhost:3000/api
FILE_UPLOADER_URL=http://localhost:8080
FILE_UPLOADER_SECRET=your-secret-key-change-this-in-production
```

## 🎯 最佳实践

### 1. 错误处理
- 实现完整的错误处理机制
- 提供用户友好的错误提示
- 记录详细的错误日志

### 2. 性能优化
- 使用缩略图提升加载速度
- 实现图片懒加载
- 压缩上传前的图片

### 3. 安全考虑
- 验证文件类型和大小
- 使用HTTPS传输
- 定期更新签名密钥

### 4. 用户体验
- 显示上传进度
- 支持拖拽上传
- 提供预览功能

## 🔗 相关链接

- [API调用说明](调用说明.md)
- [Referer防盗链配置](Referer防盗链配置说明.md)
- [项目示例代码](../examples/bun-backend/)

这个集成方案提供了完整的文件上传解决方案，包括智能缩略图、安全认证和用户友好的界面。
