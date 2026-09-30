package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// S3 talks to Amazon S3 or a compatible service (Cloudflare R2, Backblaze
// B2, MinIO, Wasabi) with signed plain HTTP requests.
type S3 struct {
	// Endpoint is empty for Amazon, or the service's address, such as
	// https://<account>.r2.cloudflarestorage.com.
	Endpoint  string
	Region    string
	Bucket    string
	Prefix    string
	AccessKey string
	SecretKey string
	HTTP      *http.Client
}

func (s *S3) region() string {
	if s.Region == "" {
		if s.Endpoint == "" {
			return "us-east-1"
		}
		return "auto"
	}
	return s.Region
}

// Bucket names follow Amazon's rules: 3 to 63 lowercase letters, digits,
// dots and hyphens, starting and ending with a letter or digit. Other
// services also take capitals and underscores. Either way the name cannot
// change the address a signed request goes to.
var (
	amazonBucket = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	otherBucket  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,61}[A-Za-z0-9]$`)
	ipLike       = regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`)
)

func (s *S3) checkBucket() error {
	if s.Bucket == "" {
		return errors.New("the bucket name is missing")
	}
	rule := otherBucket
	if s.Endpoint == "" {
		rule = amazonBucket
	}
	if !rule.MatchString(s.Bucket) || strings.Contains(s.Bucket, "..") || ipLike.MatchString(s.Bucket) {
		return errors.New("the bucket name is not valid: use 3 to 63 lowercase letters, digits, dots and hyphens")
	}
	return nil
}

// loopback is an endpoint on this machine, which may use plain http
// (MinIO for tests).
func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// url addresses the bucket by host on Amazon and by path elsewhere, which
// every compatible service accepts.
func (s *S3) url(key string, q url.Values) (string, error) {
	if err := s.checkBucket(); err != nil {
		return "", err
	}
	var base string
	if s.Endpoint == "" {
		base = "https://" + s.Bucket + ".s3." + s.region() + ".amazonaws.com/"
	} else {
		u, err := url.Parse(strings.TrimRight(s.Endpoint, "/"))
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return "", errors.New("the endpoint must be an address like https://storage.example.com")
		}
		if u.Scheme != "https" && !loopback(u.Hostname()) {
			return "", errors.New("the endpoint must use https")
		}
		base = u.Scheme + "://" + u.Host + "/" + s.Bucket + "/"
	}
	full := base + (&url.URL{Path: key}).EscapedPath()
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	return full, nil
}

func (s *S3) key(name string) string {
	p := strings.Trim(s.Prefix, "/")
	if p == "" {
		return name
	}
	return p + "/" + name
}

func (s *S3) do(ctx context.Context, method, key string, q url.Values, data []byte) (*http.Response, error) {
	u, err := s.url(key, q)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	req.Header.Set("X-Amz-Content-Sha256", hash)
	req.ContentLength = int64(len(data))
	creds := aws.Credentials{AccessKeyID: s.AccessKey, SecretAccessKey: s.SecretKey}
	if err := v4.NewSigner().SignHTTP(ctx, creds, req, hash, "s3", s.region(), time.Now()); err != nil {
		return nil, err
	}
	resp, err := client(s.HTTP).Do(req)
	if err != nil {
		return nil, fmt.Errorf("the storage is unreachable: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		var e struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		}
		xml.Unmarshal(body(resp.Body, 64<<10), &e)
		return nil, s3Error(resp.StatusCode, e.Code, e.Message)
	}
	return resp, nil
}

func s3Error(status int, code, msg string) error {
	switch code {
	case "InvalidAccessKeyId", "SignatureDoesNotMatch":
		return errors.New("the storage refused the access key or secret; check both")
	case "NoSuchBucket":
		return errors.New("that bucket does not exist; check its name and region")
	case "AccessDenied":
		return errors.New("the key has no permission on this bucket; it needs to read, write, list and delete")
	case "AuthorizationHeaderMalformed", "PermanentRedirect":
		return errors.New("the bucket is in another region; check the region")
	}
	if code == "" {
		return fmt.Errorf("the storage answered %d", status)
	}
	return fmt.Errorf("the storage refused: %s %s", code, msg)
}

func (s *S3) Put(ctx context.Context, name string, data []byte) error {
	resp, err := s.do(ctx, http.MethodPut, s.key(name), nil, data)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *S3) Get(ctx context.Context, name string) ([]byte, error) {
	resp, err := s.do(ctx, http.MethodGet, s.key(name), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return body(resp.Body, 4<<30), nil
}

func (s *S3) Delete(ctx context.Context, name string) error {
	resp, err := s.do(ctx, http.MethodDelete, s.key(name), nil, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *S3) List(ctx context.Context) ([]Object, error) {
	prefix := s.key("")
	var out []Object
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := s.do(ctx, http.MethodGet, "", q, nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Contents []struct {
				Key          string    `xml:"Key"`
				Size         int64     `xml:"Size"`
				LastModified time.Time `xml:"LastModified"`
			} `xml:"Contents"`
			Truncated bool   `xml:"IsTruncated"`
			Next      string `xml:"NextContinuationToken"`
		}
		err = xml.Unmarshal(body(resp.Body, 32<<20), &page)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("unexpected answer from the storage: %w", err)
		}
		for _, c := range page.Contents {
			name := strings.TrimPrefix(c.Key, prefix)
			if name != "" && !strings.Contains(name, "/") {
				out = append(out, Object{Name: name, Size: c.Size, Modified: c.LastModified})
			}
		}
		if !page.Truncated || page.Next == "" {
			return out, nil
		}
		token = page.Next
	}
}
