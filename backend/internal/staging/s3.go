// backend/internal/staging/s3.go
package staging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// ErrUnavailable tags connection-level staging failures (endpoint unreachable,
// timeout before any S3 response). Handlers map it to ERR_STORAGE_UNAVAILABLE.
var ErrUnavailable = errors.New("chunk storage unavailable")

// s3API is the subset of the S3 client used by S3Store; tests inject a fake.
type s3API interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	HeadObject(ctx context.Context, in *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObjects(ctx context.Context, in *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	HeadBucket(ctx context.Context, in *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

// S3Options configures an S3Store. All values come from GFTP_S3_* env vars.
type S3Options struct {
	Endpoint     string // full URL incl. scheme, e.g. "http://minio:9000"
	Bucket       string
	Region       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool // true for MinIO, false for AWS virtual-hosted addressing
	Prefix       string
	Timeout      time.Duration // per S3 call; streaming reads are bound by the request context instead
}

// S3Store stages upload chunks in an S3-compatible bucket under
// {prefix}/{uploadID}/{index:%04d} — the same zero-padded scheme as LocalStore.
type S3Store struct {
	api     s3API
	bucket  string
	prefix  string
	timeout time.Duration
}

// NewS3Store creates an S3Store. The bucket must already exist; call Ping to
// probe reachability.
func NewS3Store(opts S3Options) *S3Store {
	cfg := aws.Config{
		Region:      opts.Region,
		Credentials: credentials.NewStaticCredentialsProvider(opts.AccessKey, opts.SecretKey, ""),
	}
	cl := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(opts.Endpoint)
		o.UsePathStyle = opts.UsePathStyle
	})
	return &S3Store{
		api:     cl,
		bucket:  opts.Bucket,
		prefix:  opts.Prefix,
		timeout: opts.Timeout,
	}
}

// Ping checks that the bucket is reachable with the configured credentials.
func (s *S3Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	_, err := s.api.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	return err
}

func (s *S3Store) uploadPrefix(uploadID string) string {
	if s.prefix == "" {
		return uploadID + "/"
	}
	return s.prefix + "/" + uploadID + "/"
}

func (s *S3Store) chunkKey(uploadID string, index int) string {
	return s.uploadPrefix(uploadID) + fmt.Sprintf("%04d", index)
}

func (s *S3Store) NewUpload(_ context.Context, destination string, totalChunks int, chunkSize int64) (*transfer.UploadMeta, error) {
	// No S3 call: buckets have no directories, so there is nothing to create
	// until the first chunk arrives.
	return &transfer.UploadMeta{
		ID:          uuid.NewString(),
		Destination: destination,
		TotalChunks: totalChunks,
		ChunkSize:   chunkSize,
	}, nil
}

func (s *S3Store) WriteChunk(ctx context.Context, uploadID string, index int, size int64, r io.Reader) error {
	if err := transfer.ValidateUploadID(uploadID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	// Re-PUT of the same key on a frontend retry overwrites idempotently.
	_, err := s.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(s.chunkKey(uploadID, index)),
		Body:          r,
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return wrapUnavailable(fmt.Errorf("s3 put chunk %d: %w", index, err))
	}
	return nil
}

func (s *S3Store) AssembleReader(ctx context.Context, uploadID string, totalChunks int) (io.ReadCloser, error) {
	if err := transfer.ValidateUploadID(uploadID); err != nil {
		return nil, err
	}
	// Verify all chunks exist before streaming any (mirrors LocalStore).
	for i := range totalChunks {
		hctx, cancel := context.WithTimeout(ctx, s.timeout)
		_, err := s.api.HeadObject(hctx, &s3.HeadObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(s.chunkKey(uploadID, i)),
		})
		cancel()
		if err != nil {
			return nil, wrapUnavailable(fmt.Errorf("chunk %d missing: %w", i, err))
		}
	}
	return &s3SequentialReader{ctx: ctx, store: s, uploadID: uploadID, totalChunks: totalChunks}, nil
}

func (s *S3Store) Cleanup(ctx context.Context, uploadID string) error {
	if err := transfer.ValidateUploadID(uploadID); err != nil {
		return err
	}
	prefix := s.uploadPrefix(uploadID)
	var token *string
	for {
		lctx, cancel := context.WithTimeout(ctx, s.timeout)
		out, err := s.api.ListObjectsV2(lctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		cancel()
		if err != nil {
			return wrapUnavailable(fmt.Errorf("s3 list chunks: %w", err))
		}
		if len(out.Contents) > 0 {
			ids := make([]types.ObjectIdentifier, 0, len(out.Contents))
			for _, obj := range out.Contents {
				ids = append(ids, types.ObjectIdentifier{Key: obj.Key})
			}
			dctx, cancel := context.WithTimeout(ctx, s.timeout)
			_, err = s.api.DeleteObjects(dctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(s.bucket),
				Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
			})
			cancel()
			if err != nil {
				return wrapUnavailable(fmt.Errorf("s3 delete chunks: %w", err))
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return nil
		}
		token = out.NextContinuationToken
	}
}

// s3SequentialReader streams chunks in index order with at most one GetObject
// body open at a time (constant memory). Reads are bound by the parent request
// context, not the per-call timeout, so slow FTP/SFTP sinks don't abort
// mid-stream — matching LocalStore, whose disk reads have no deadline either.
type s3SequentialReader struct {
	ctx         context.Context
	store       *S3Store
	uploadID    string
	totalChunks int
	index       int           // next chunk index to open
	cur         io.ReadCloser // currently open chunk body, nil if none
}

func (r *s3SequentialReader) Read(p []byte) (int, error) {
	for {
		if r.cur == nil {
			if r.index >= r.totalChunks {
				return 0, io.EOF
			}
			out, err := r.store.api.GetObject(r.ctx, &s3.GetObjectInput{
				Bucket: aws.String(r.store.bucket),
				Key:    aws.String(r.store.chunkKey(r.uploadID, r.index)),
			})
			if err != nil {
				return 0, wrapUnavailable(fmt.Errorf("s3 get chunk %d: %w", r.index, err))
			}
			r.cur = out.Body
			r.index++
		}
		n, err := r.cur.Read(p)
		if err == io.EOF {
			closeErr := r.cur.Close()
			r.cur = nil
			if closeErr != nil {
				return n, closeErr
			}
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (r *s3SequentialReader) Close() error {
	if r.cur == nil {
		return nil
	}
	err := r.cur.Close()
	r.cur = nil
	r.index = r.totalChunks
	return err
}

// wrapUnavailable tags connection-level failures with ErrUnavailable. Errors
// that carry an S3 API response (NoSuchKey, AccessDenied, ...) pass through —
// the service responded, so it is not an outage.
func wrapUnavailable(err error) error {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}
