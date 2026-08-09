package storage

import (
	"bytes"
	"context"
	"io"
	"mime"
	"path"
	"strings"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// awsEndpoint is used when no endpoint is configured, which means AWS S3
// proper. Any other S3-compatible service (Linode, MinIO, ...) must name its
// endpoint explicitly.
const awsEndpoint = "s3.amazonaws.com"

var _ Interface = &S3{}

// S3 stores receipts as objects in an S3-compatible bucket.
type S3 struct {
	client *minio.Client
	bucket string
}

func NewS3(cfg config.S3) (*S3, error) {
	endpoint, useSSL := normalizeEndpoint(cfg.Endpoint)

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Region: cfg.Region,
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	return &S3{client: client, bucket: cfg.Bucket}, nil
}

// normalizeEndpoint accepts an endpoint with or without a scheme. TLS is on
// unless "http://" is asked for explicitly, which is only sensible for a local
// MinIO.
func normalizeEndpoint(endpoint string) (string, bool) {
	if endpoint == "" {
		return awsEndpoint, true
	}

	if rest, ok := strings.CutPrefix(endpoint, "http://"); ok {
		return rest, false
	}

	return strings.TrimPrefix(endpoint, "https://"), true
}

func (s *S3) Save(ctx context.Context, key string, data []byte) error {
	contentType := mime.TypeByExtension(path.Ext(key))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err := s.client.PutObject(
		ctx,
		s.bucket,
		key,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType},
	)

	return err
}

// Load fetches the object stored under key and returns its bytes. minio defers
// the request until the stream is read, so a missing key surfaces here as a
// read error (which the caller turns into a 404).
func (s *S3) Load(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close() //nolint:errcheck // read-only object stream

	return io.ReadAll(obj)
}
