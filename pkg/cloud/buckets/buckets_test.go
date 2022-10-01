package buckets_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cloud/buckets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "gocloud.dev/blob/fileblob"
)

type HTTPClientMock struct {
	// DoFunc will be executed whenever Do function is executed
	// so we'll be able to create a custom response
	DoFunc func(*http.Request) (*http.Response, error)
}

func (H HTTPClientMock) Do(r *http.Request) (*http.Response, error) {
	return H.DoFunc(r)
}

func TestSplitBucketURL(t *testing.T) {
	assertSplitBucketURL(t, "s3://foo/my/file", "s3://foo", "my/file")
	assertSplitBucketURL(t, "gs://mybucket/beer/cheese.txt?param=1234", "gs://mybucket?param=1234", "beer/cheese.txt")
	assertSplitBucketURL(t,
		"s3://jx3/jenkins-x/logs/org/repo/foo.log?endpoint=minio.minio.svc.cluster.local:9000&disableSSL=true&s3ForcePathStyle=true&region=ignored",
		"s3://jx3?endpoint=minio.minio.svc.cluster.local:9000&disableSSL=true&s3ForcePathStyle=true&region=ignored",
		"jenkins-x/logs/org/repo/foo.log")
}

func assertSplitBucketURL(t *testing.T, inputURL, expectedBucketURL, expectedKey string) {
	u, err := url.Parse(inputURL)
	require.NoError(t, err, "failed to parse URL %s", inputURL)

	bucketURL, key := buckets.SplitBucketURL(u)

	assert.Equal(t, expectedBucketURL, bucketURL, "for URL %s", inputURL)
	assert.Equal(t, expectedKey, key, "for URL %s", inputURL)
}

func TestCreateBucketURL(t *testing.T) {
	testCases := []struct {
		name              string
		kind              string
		cloudProvider     string
		expectedBucketURL string
		expectedError     error
	}{
		{
			name:              "jx-bucket",
			kind:              "storage",
			cloudProvider:     "gcp",
			expectedBucketURL: "storage://jx-bucket",
			expectedError:     nil,
		},
		{
			name:              "jx-bucket2",
			kind:              "storage",
			cloudProvider:     "aws",
			expectedBucketURL: "storage://jx-bucket2",
			expectedError:     nil,
		},
		{
			name:              "jx-bucket3",
			kind:              "",
			cloudProvider:     "",
			expectedBucketURL: "",
			expectedError:     fmt.Errorf("no bucket kind provided nor is a kubernetes provider configured for this team so it could not be defaulted"),
		},
		{
			name:              "jx-bucket4",
			kind:              "",
			cloudProvider:     "aws",
			expectedBucketURL: "s3://jx-bucket4",
			expectedError:     nil,
		},
	}
	for _, tc := range testCases {
		url, err := buckets.CreateBucketURL(tc.name, tc.kind, tc.cloudProvider)
		assert.Equal(t, url, tc.expectedBucketURL)
		if err != nil {
			assert.EqualError(t, err, tc.expectedError.Error())
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestReadURL(t *testing.T) {
	testCases := []struct {
		ctx                  context.Context
		urlText              string
		timeout              time.Duration
		httpFn               func(urlString string) (string, func(*http.Request), error)
		expectedError        error
		httpResponse         *HTTPClientMock
		ExpectedReadResponse []byte
	}{
		{
			ctx:           nil,
			urlText:       "https://jenkins-x.io/",
			httpFn:        nil,
			timeout:       30 * time.Second,
			expectedError: nil,
			httpResponse: &HTTPClientMock{
				DoFunc: func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: 200,
						Body:       ioutil.NopCloser(bytes.NewReader([]byte(`[{"body": "ok"}]`))),
					}, nil
				},
			},
			ExpectedReadResponse: []byte(`[{"body": "ok"}]`),
		},
		{
			ctx:           nil,
			urlText:       "https://jenkins-x-demo.io/",
			httpFn:        nil,
			timeout:       30 * time.Second,
			expectedError: fmt.Errorf("status Not found when performing GET on https://jenkins-x-demo.io/"),
			httpResponse: &HTTPClientMock{
				DoFunc: func(*http.Request) (*http.Response, error) {
					return &http.Response{
						Status:     "Not found",
						StatusCode: 404,
					}, nil
				},
			},
		},
		{
			ctx:           nil,
			urlText:       "https://jenkins-x-error.io/",
			httpFn:        nil,
			timeout:       30 * time.Second,
			expectedError: fmt.Errorf("failed to invoke GET on https://jenkins-x-error.io/: No Internet Connection"),
			httpResponse: &HTTPClientMock{
				DoFunc: func(*http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("No Internet Connection")
				},
			},
		},
	}
	for _, tc := range testCases {
		if tc.httpResponse != nil {
			buckets.HttpClient = tc.httpResponse
			buckets.GetClientWithTimeout = func(timeout time.Duration) buckets.HTTPClient {
				return buckets.HttpClient
			}
		}
		read, err := buckets.ReadURL(tc.ctx, tc.urlText, tc.timeout, tc.httpFn)
		if read != nil {
			readResponse, err := ioutil.ReadAll(read)
			assert.NoError(t, err)
			assert.Equal(t, readResponse, tc.ExpectedReadResponse)
		} else {
			assert.Nil(t, read)
		}
		if err != nil {
			assert.EqualError(t, err, tc.expectedError.Error())
		} else {
			assert.NoError(t, err)
		}
	}

	ctx := context.Background()
	read, err := buckets.ReadURL(ctx, "file:///test_data/foo", 30, nil)
	assert.Nil(t, read)
	assert.Contains(t, err.Error(), "failed to read key")
}

func TestBucketURL(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		url           string
		data          string
		time          time.Duration
		expectedError string
	}{
		{
			url:  "file:///test_data/my-key",
			data: "Hello world",
			time: 30 * time.Second,
		},
		{
			url:           "x:///test_data/fooError",
			data:          "Hello foo2",
			time:          30 * time.Second,
			expectedError: "failed to open bucket",
		},
	}
	dir, err := os.Getwd()
	assert.NoError(t, err)
	defer os.RemoveAll(dir + "/test_data/")
	for _, tc := range testCases {
		u, err := url.Parse(tc.url)
		assert.NoError(t, err)

		err = buckets.WriteBucketURL(ctx, u, strings.NewReader(tc.data), tc.time)
		if tc.expectedError != "" {
			assert.Contains(t, err.Error(), "failed to open bucket")
		} else {
			assert.NoError(t, err)
		}
		read, err := buckets.ReadURL(ctx, tc.url, tc.time, nil)
		if tc.expectedError != "" {
			assert.Nil(t, read)
			assert.Contains(t, err.Error(), "failed to open bucket")
		} else {
			assert.NoError(t, err)
			buf := new(bytes.Buffer)
			buf.ReadFrom(read)
			assert.Equal(t, buf.String(), tc.data)
		}
	}
}
