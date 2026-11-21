package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3 is an S3-compatible adapter (Cloudflare R2). Objects are private; all
// access goes through presigned URLs.
type S3 struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
}

// S3Config configures the adapter.
type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

// NewS3 constructs an R2-compatible S3 store.
func NewS3(cfg S3Config) (*S3, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("incomplete S3 storage configuration")
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	client := s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	})
	return &S3{client: client, presigner: s3.NewPresignClient(client), bucket: cfg.Bucket}, nil
}

// PresignPut signs an upload that enforces content type and SHA-256.
func (s *S3) PresignPut(ctx context.Context, key, contentType string, size int64, sha256 string, ttl time.Duration) (Request, error) {
	if ttl <= 0 || ttl > 15*time.Minute {
		ttl = 15 * time.Minute
	}
	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	}
	headers := map[string]string{"Content-Type": contentType}
	if sha256 != "" {
		input.ChecksumSHA256 = aws.String(base64SHA256(sha256))
		headers["x-amz-checksum-sha256"] = base64SHA256(sha256)
	}
	presigned, err := s.presigner.PresignPutObject(ctx, input, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return Request{}, fmt.Errorf("presign upload: %w", err)
	}
	for name, values := range presigned.SignedHeader {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}
	return Request{URL: presigned.URL, Method: "PUT", Headers: headers}, nil
}

// PresignGet signs a short-lived download.
func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (Request, error) {
	if ttl <= 0 || ttl > 15*time.Minute {
		ttl = 15 * time.Minute
	}
	presigned, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) { options.Expires = ttl })
	if err != nil {
		return Request{}, fmt.Errorf("presign download: %w", err)
	}
	return Request{URL: presigned.URL, Method: "GET", Headers: map[string]string{}}, nil
}

// Head reads object metadata and checksum.
func (s *S3) Head(ctx context.Context, key string) (Object, error) {
	output, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(key),
		ChecksumMode: types.ChecksumModeEnabled,
	})
	if err != nil {
		return Object{}, fmt.Errorf("head object: %w", err)
	}
	return Object{
		ContentType: aws.ToString(output.ContentType),
		Size:        aws.ToInt64(output.ContentLength),
		SHA256:      hexFromBase64(aws.ToString(output.ChecksumSHA256)),
	}, nil
}

// Delete removes an object.
func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}
