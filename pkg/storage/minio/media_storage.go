package minio_storage

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	storage_interfaces "github.com/evolution-foundation/evolution-go/pkg/storage/interfaces"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioMediaStorage struct {
	client     *minio.Client
	bucketName string
	baseURL    string
}

func setBucketPolicy(client *minio.Client, bucketName string) error {
	policy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": "*",
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::` + bucketName + `/*"]
			}
		]
	}`

	return client.SetBucketPolicy(context.Background(), bucketName, policy)
}

// generateFilePath creates a simple media folder structure
// Format: evolution-go-medias/{filename}
func generateFilePath(fileName string) string {
	return fmt.Sprintf("evolution-go-medias/%s", fileName)
}

// resolveFilePath determines if the input is a full path or just a filename
// If it's just a filename, it assumes it's in the evolution-go-medias folder
// If it's a full path, it returns it as-is
func (m *MinioMediaStorage) resolveFilePath(ctx context.Context, fileNameOrPath string) (string, error) {
	// If the input already contains path separators, assume it's a full path
	if strings.Contains(fileNameOrPath, "/") {
		return fileNameOrPath, nil
	}

	// If it's just a filename, assume it's in the evolution-go-medias folder
	return fmt.Sprintf("evolution-go-medias/%s", fileNameOrPath), nil
}

func NewMinioMediaStorage(
	endpoint,
	accessKeyID,
	secretAccessKey,
	bucketName,
	region string,
	useSSL bool,
) (storage_interfaces.MediaStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Try to set bucket policy to allow public access (optional for some providers)
	err = setBucketPolicy(client, bucketName)
	if err != nil {
		// Some providers (like Backblaze B2) don't support SetBucketPolicy
		// Log warning but continue - files can still be accessed via presigned URLs
		fmt.Printf("Warning: Failed to set bucket policy (provider may not support it): %v\n", err)
	}

	baseURL := fmt.Sprintf("https://%s/%s", endpoint, bucketName)
	if !useSSL {
		baseURL = fmt.Sprintf("http://%s/%s", endpoint, bucketName)
	}

	return &MinioMediaStorage{
		client:     client,
		bucketName: bucketName,
		baseURL:    baseURL,
	}, nil
}

func (m *MinioMediaStorage) Store(ctx context.Context, data []byte, fileName string, contentType string) (string, error) {
	// Generate organized file path
	filePath := generateFilePath(fileName)
	reader := bytes.NewReader(data)

	_, err := m.client.PutObject(ctx, m.bucketName, filePath, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to store object: %w", err)
	}

	// Gerando URL assinada com validade de 7 dias
	reqParams := make(url.Values)
	presignedURL, err := m.client.PresignedGetObject(ctx, m.bucketName, filePath, time.Hour*24*7, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}

func (m *MinioMediaStorage) Delete(ctx context.Context, fileName string) error {
	// Resolve the full path for the file
	filePath, err := m.resolveFilePath(ctx, fileName)
	if err != nil {
		return fmt.Errorf("failed to resolve file path: %w", err)
	}

	err = m.client.RemoveObject(ctx, m.bucketName, filePath, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

func (m *MinioMediaStorage) GetURL(ctx context.Context, fileName string) (string, error) {
	// Resolve the full path for the file
	filePath, err := m.resolveFilePath(ctx, fileName)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file path: %w", err)
	}

	// Check if object exists
	_, err = m.client.StatObject(ctx, m.bucketName, filePath, minio.StatObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get object stats: %w", err)
	}

	// Gerando URL assinada com validade de 7 dias
	reqParams := make(url.Values)
	presignedURL, err := m.client.PresignedGetObject(ctx, m.bucketName, filePath, time.Hour*24*7, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}
