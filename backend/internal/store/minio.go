package store

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Objects 封装 MinIO 对象存储,负责原始文件归档。
type Objects struct {
	client *minio.Client
	bucket string
}

// OpenObjects 连接 MinIO 并确保归档桶存在。
func OpenObjects(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Objects, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("store: 连接 MinIO 失败: %w", err)
	}
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("store: 检查 bucket 失败: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("store: 创建 bucket %q 失败: %w", bucket, err)
		}
	}
	return &Objects{client: client, bucket: bucket}, nil
}

// Put 归档一个文件,返回对象键。
func (o *Objects) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := o.client.PutObject(ctx, o.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

// Get 读取归档文件内容。
func (o *Objects) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := o.client.GetObject(ctx, o.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

// Remove 删除归档文件(对象不存在时不报错)。
func (o *Objects) Remove(ctx context.Context, key string) error {
	return o.client.RemoveObject(ctx, o.bucket, key, minio.RemoveObjectOptions{})
}
