package util

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const fakeGCSImage = "fsouza/fake-gcs-server:1.52"

// GCSContainer is a local fake GCS server and unauthenticated client for integration tests.
type GCSContainer struct {
	container testcontainers.Container
	Client    *storage.Client
}

// StartGCSContainer starts an HTTP GCS emulator for integration tests.
func StartGCSContainer(ctx context.Context) (*GCSContainer, error) {
	const port = "4443/tcp"
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        fakeGCSImage,
			ExposedPorts: []string{port},
			Cmd:          []string{"-scheme", "http", "-port", "4443"},
			WaitingFor: wait.ForHTTP("/storage/v1/b").
				WithPort(port).
				WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		return nil, fmt.Errorf("starting fake GCS container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("getting fake GCS host: %w", err)
	}
	mappedPort, err := container.MappedPort(ctx, port)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("getting fake GCS port: %w", err)
	}

	client, err := storage.NewClient(ctx,
		option.WithEndpoint(fmt.Sprintf("http://%s:%s/storage/v1/", host, mappedPort.Port())),
		option.WithoutAuthentication(),
		storage.WithJSONReads(),
	)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("creating fake GCS client: %w", err)
	}
	return &GCSContainer{container: container, Client: client}, nil
}

// NewTestBucket creates a bucket isolated to one integration test and removes
// all of its objects and the bucket when the test completes.
func NewTestBucket(t *testing.T, gc *GCSContainer) string {
	t.Helper()
	bucketName := fmt.Sprintf("sippy-integration-%d", time.Now().UnixNano())
	ctx := context.Background()
	if err := gc.Client.Bucket(bucketName).Create(ctx, "test-project", nil); err != nil {
		t.Fatalf("creating test GCS bucket %q: %v", bucketName, err)
	}
	t.Cleanup(func() {
		if err := emptyBucket(ctx, gc.Client.Bucket(bucketName)); err != nil {
			t.Logf("warning: failed to empty test GCS bucket %s: %v", bucketName, err)
			return
		}
		if err := gc.Client.Bucket(bucketName).Delete(ctx); err != nil {
			t.Logf("warning: failed to delete test GCS bucket %s: %v", bucketName, err)
		}
	})
	return bucketName
}

func emptyBucket(ctx context.Context, bucket *storage.BucketHandle) error {
	objects := bucket.Objects(ctx, nil)
	for {
		attrs, err := objects.Next()
		if err == iterator.Done {
			return nil
		}
		if err != nil {
			return fmt.Errorf("listing objects: %w", err)
		}
		if err := bucket.Object(attrs.Name).Delete(ctx); err != nil {
			return fmt.Errorf("deleting object %q: %w", attrs.Name, err)
		}
	}
}

// Terminate closes the client and stops the fake GCS server.
func (gc *GCSContainer) Terminate(ctx context.Context) error {
	if gc.Client != nil {
		if err := gc.Client.Close(); err != nil {
			return fmt.Errorf("closing fake GCS client: %w", err)
		}
	}
	if gc.container != nil {
		if err := gc.container.Terminate(ctx); err != nil {
			return fmt.Errorf("terminating fake GCS container: %w", err)
		}
	}
	return nil
}
