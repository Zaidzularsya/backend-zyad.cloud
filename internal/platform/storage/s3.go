package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
)

// S3Config adalah parameter koneksi ke object storage S3-compatible (MinIO).
type S3Config struct {
	Endpoint      string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	UsePathStyle  bool
	PresignExpiry time.Duration
}

// S3Provider menyimpan object di bucket S3-compatible. Object key dipetakan 1:1
// ke S3 key (hasil BuildObjectKey), tanpa prefix tambahan.
type S3Provider struct {
	client        *s3.Client
	presign       *s3.PresignClient
	bucket        string
	presignExpiry time.Duration
}

var (
	_ ObjectStorage      = (*S3Provider)(nil)
	_ PublicObjectReader = (*S3Provider)(nil)
	_ PresignedStorage   = (*S3Provider)(nil)
)

const defaultPresignExpiry = 15 * time.Minute

func NewS3Provider(cfg S3Config) (*S3Provider, error) {
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.AccessKey = strings.TrimSpace(cfg.AccessKey)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("s3 storage requires endpoint, bucket, access key, and secret key")
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	expiry := cfg.PresignExpiry
	if expiry <= 0 {
		expiry = defaultPresignExpiry
	}

	awsCfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})

	return &S3Provider{
		client:        client,
		presign:       s3.NewPresignClient(client),
		bucket:        cfg.Bucket,
		presignExpiry: expiry,
	}, nil
}

func (p *S3Provider) Put(
	ctx context.Context,
	key string,
	content io.Reader,
	contentType string,
	size int64,
) error {
	cleaned := cleanObjectKey(key)
	if cleaned == "" {
		return ErrInvalidTenantObject
	}
	// aws-sdk-go-v2 butuh body yang seekable untuk menghitung payload hash saat
	// SigV4 signing. Caller (media service) mengirim io.LimitReader yang tidak
	// seekable, jadi buffer dulu di memory. Aman karena object media dibatasi
	// MaxMediaSizeBytes (10MB) di layer service.
	body, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("buffer s3 object body: %w", err)
	}
	input := &s3.PutObjectInput{
		Bucket:        aws.String(p.bucket),
		Key:           aws.String(cleaned),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	if _, err := p.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("put s3 object: %w", err)
	}
	return nil
}

func (p *S3Provider) Delete(ctx context.Context, key string) error {
	cleaned := cleanObjectKey(key)
	if cleaned == "" {
		return ErrInvalidTenantObject
	}
	// DeleteObject di S3 idempotent (tidak error untuk key yang tidak ada),
	// jadi cek dulu lewat HeadObject supaya semantik ErrObjectNotFound tetap
	// konsisten dengan LocalProvider.
	if _, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(cleaned),
	}); err != nil {
		if isS3NotFound(err) {
			return ErrObjectNotFound
		}
		return fmt.Errorf("head s3 object: %w", err)
	}
	if _, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(cleaned),
	}); err != nil {
		return fmt.Errorf("delete s3 object: %w", err)
	}
	return nil
}

func (p *S3Provider) OpenPublic(ctx context.Context, key string) (io.ReadCloser, string, error) {
	cleaned := cleanObjectKey(key)
	if cleaned == "" || !isPublicObjectKey(cleaned) {
		return nil, "", ErrInvalidTenantObject
	}
	out, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(cleaned),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, "", ErrObjectNotFound
		}
		return nil, "", fmt.Errorf("get s3 object: %w", err)
	}
	contentType := "application/octet-stream"
	if out.ContentType != nil && *out.ContentType != "" {
		contentType = *out.ContentType
	}
	return out.Body, contentType, nil
}

func (p *S3Provider) PresignPut(
	ctx context.Context,
	key string,
	contentType string,
	_ int64,
) (PresignedResult, error) {
	cleaned := cleanObjectKey(key)
	if cleaned == "" {
		return PresignedResult{}, ErrInvalidTenantObject
	}
	input := &s3.PutObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(cleaned),
	}
	headers := map[string]string{}
	if contentType != "" {
		// Content-Type ikut ditandatangani; client wajib mengirim header yang
		// sama persis saat PUT.
		input.ContentType = aws.String(contentType)
		headers["Content-Type"] = contentType
	}
	req, err := p.presign.PresignPutObject(ctx, input, s3.WithPresignExpires(p.presignExpiry))
	if err != nil {
		return PresignedResult{}, fmt.Errorf("presign put s3 object: %w", err)
	}
	return PresignedResult{
		URL:       req.URL,
		Method:    req.Method,
		Headers:   headers,
		ExpiresAt: time.Now().Add(p.presignExpiry),
	}, nil
}

func (p *S3Provider) PresignGet(ctx context.Context, key string) (PresignedResult, error) {
	cleaned := cleanObjectKey(key)
	if cleaned == "" {
		return PresignedResult{}, ErrInvalidTenantObject
	}
	req, err := p.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(cleaned),
	}, s3.WithPresignExpires(p.presignExpiry))
	if err != nil {
		return PresignedResult{}, fmt.Errorf("presign get s3 object: %w", err)
	}
	return PresignedResult{
		URL:       req.URL,
		Method:    req.Method,
		ExpiresAt: time.Now().Add(p.presignExpiry),
	}, nil
}

func isS3NotFound(err error) bool {
	var noSuchKey *s3types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var notFound *s3types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}
