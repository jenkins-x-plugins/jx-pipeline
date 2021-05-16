package buckets_test

import (
	"net/url"
	"testing"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cloud/buckets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitBucketURL(t *testing.T) {
	assertSplitBucketURL(t, "s3://foo/my/file", "s3://foo", "my/file")
	assertSplitBucketURL(t, "gs://mybucket/beer/cheese.txt?param=1234", "gs://mybucket?param=1234", "beer/cheese.txt")
	assertSplitBucketURL(t,
		"s3://jx3/jenkins-x/logs/org/repo/foo.log?endpoint=minio.minio.svc.cluster.local:9000&disableSSL=true&s3ForcePathStyle=true&region=ignored",
		"s3://jx3?endpoint=minio.minio.svc.cluster.local:9000&disableSSL=true&s3ForcePathStyle=true&region=ignored",
		"jenkins-x/logs/org/repo/foo.log")
}

func assertSplitBucketURL(t *testing.T, inputURL string, expectedBucketURL string, expectedKey string) {
	u, err := url.Parse(inputURL)
	require.NoError(t, err, "failed to parse URL %s", inputURL)

	bucketURL, key := buckets.SplitBucketURL(u)

	assert.Equal(t, expectedBucketURL, bucketURL, "for URL %s", inputURL)
	assert.Equal(t, expectedKey, key, "for URL %s", inputURL)
}
