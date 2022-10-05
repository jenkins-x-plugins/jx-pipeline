package buckets

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cloud"
	"github.com/jenkins-x/jx-helpers/v3/pkg/httphelpers"
	"github.com/pkg/errors"
	"gocloud.dev/blob"

	// support azure blobs
	_ "gocloud.dev/blob/azureblob"
	// support file blobs
	_ "gocloud.dev/blob/fileblob"
	// support GCS blobs
	_ "gocloud.dev/blob/gcsblob"
	// support memory blobs
	_ "gocloud.dev/blob/memblob"
	// support s3 blobs
	_ "gocloud.dev/blob/s3blob"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

var (
	HttpClient HTTPClient
)

// CreateBucketURL creates a go-cloud URL to a bucket
func CreateBucketURL(name, kind, cloudProvider string) (string, error) {
	if kind == "" {
		if cloudProvider == "" {
			return "", fmt.Errorf("no bucket kind provided nor is a kubernetes provider configured for this team so it could not be defaulted")
		}
		kind = KubeProviderToBucketScheme(cloudProvider)
		if kind == "" {
			return "", fmt.Errorf("no bucket kind is associated with kubernetes provider %s", cloudProvider)
		}
	}
	return kind + "://" + name, nil
}

// KubeProviderToBucketScheme returns the bucket scheme for the cloud provider
func KubeProviderToBucketScheme(provider string) string {
	switch provider {
	case cloud.AKS:
		return "azblob"
	case cloud.AWS, cloud.EKS:
		return "s3"
	case cloud.GKE:
		return "gs"
	default:
		return ""
	}
}

// ReadURL reads the given URL from either a http/https endpoint or a bucket URL path.
// if specified the httpFn is a function which can append the user/password or token and/or add a header with the token if using a git provider
func ReadURL(ctx context.Context, urlText string, timeout time.Duration, httpFn func(urlString string) (string, func(*http.Request), error)) (io.ReadCloser, error) {
	u, err := url.Parse(urlText)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse URL %s", urlText)
	}
	var headerFunc func(*http.Request)
	switch u.Scheme {
	case "http", "https":
		if httpFn != nil {
			urlText, headerFunc, err = httpFn(urlText)
			if err != nil {
				return nil, err
			}
		}
		return ReadHTTPURL(urlText, headerFunc, timeout)
	default:
		return ReadBucketURL(ctx, u, timeout)
	}
}

var GetClientWithTimeout = func(timeout time.Duration) HTTPClient {
	return httphelpers.GetClientWithTimeout(timeout)
}

// ReadHTTPURL reads the HTTP based URL, modifying the headers as needed, and returns the data or returning an error if a 2xx status is not returned
func ReadHTTPURL(u string, headerFunc func(*http.Request), timeout time.Duration) (io.ReadCloser, error) {
	HttpClient = GetClientWithTimeout(timeout)

	req, err := http.NewRequest("GET", u, http.NoBody)
	if err != nil {
		return nil, err
	}
	if headerFunc != nil {
		headerFunc(req)
	}
	resp, err := HttpClient.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to invoke GET on %s", u)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %s when performing GET on %s", resp.Status, u)
	}
	return resp.Body, nil
}

// ReadBucketURL reads the content of a bucket URL of the for 's3://bucketName/foo/bar/whatnot.txt?param=123'
// where any of the query arguments are applied to the underlying Bucket URL and the path is extracted and resolved
// within the bucket
func ReadBucketURL(ctx context.Context, u *url.URL, timeout time.Duration) (io.ReadCloser, error) {
	bucketURL, key := SplitBucketURL(u)

	bucket, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open bucket %s", bucketURL)
	}
	data, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return data, errors.Wrapf(err, "failed to read key %s in bucket %s", key, bucketURL)
	}
	return data, nil
}

// WriteBucketURL writes the data to a bucket URL of the for 's3://bucketName/foo/bar/whatnot.txt?param=123'
// with the given timeout
func WriteBucketURL(ctx context.Context, u *url.URL, data io.Reader, timeout time.Duration) error {
	bucketURL, key := SplitBucketURL(u)
	return WriteBucket(ctx, bucketURL, key, data, timeout)
}

// WriteBucket writes the data to a bucket URL and key of the for 's3://bucketName' and key 'foo/bar/whatnot.txt'
// with the given timeout
func WriteBucket(ctx context.Context, bucketURL, key string, reader io.Reader, timeout time.Duration) (err error) {
	bucket, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		return errors.Wrapf(err, "failed to open bucket %s", bucketURL)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return errors.Wrapf(err, "failed to read data for key %s in bucket %s", key, bucketURL)
	}
	err = bucket.WriteAll(ctx, key, data, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to write key %s in bucket %s", key, bucketURL)
	}
	return nil
}

// SplitBucketURL splits the full bucket URL into the URL to open the bucket and the file name to refer to
// within the bucket
func SplitBucketURL(u *url.URL) (string, string) {
	u2 := *u
	u2.Path = ""
	return u2.String(), strings.TrimPrefix(u.Path, "/")
}
