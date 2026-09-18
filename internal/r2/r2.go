// Package r2 signs Cloudflare R2 requests against the S3-compatible API.
//
// The server runs in a Container, so it cannot use R2 bindings (those are
// Worker-only). Instead it signs browser uploads with a presigned PUT URL and
// deletes objects itself, both with AWS Signature Version 4.
package r2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	signingAlgorithm = "AWS4-HMAC-SHA256"
	unsignedPayload  = "UNSIGNED-PAYLOAD"
	region           = "auto"
	service          = "s3"
	requestTimeout   = 15 * time.Second

	// R2 access key IDs are 32 hex characters; secrets are 64 characters.
	accessKeyIDLength     = 32
	secretAccessKeyLength = 64
)

// ErrNotConfigured is returned when a signing credential is missing.
var ErrNotConfigured = errors.New("r2: credentials are not configured")

// Config holds the R2 S3-compatible settings. Endpoint overrides the public
// Cloudflare endpoint so tests can point at a local server.
type Config struct {
	AccountID       string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string
}

// Configured reports whether every value needed to sign a request is present.
func (c Config) Configured() bool {
	return c.AccountID != "" &&
		c.Bucket != "" &&
		c.AccessKeyID != "" &&
		c.SecretAccessKey != ""
}

// Validate reports whether the credentials are well-formed. Presigning happens
// locally, so a wrong-length key is not caught until the browser's PUT reaches
// R2 and fails with an opaque error; checking at startup surfaces it clearly.
// An unconfigured Config is valid because image uploads are optional.
func (c Config) Validate() error {
	if !c.Configured() {
		return nil
	}

	if len(c.AccessKeyID) != accessKeyIDLength || !isHex(c.AccessKeyID) {
		return fmt.Errorf("r2: access key ID must be %d hex characters, got %d", accessKeyIDLength, len(c.AccessKeyID))
	}

	if len(c.SecretAccessKey) != secretAccessKeyLength {
		return fmt.Errorf("r2: secret access key must be %d characters, got %d", secretAccessKeyLength, len(c.SecretAccessKey))
	}

	return nil
}

func isHex(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') &&
			(character < 'A' || character > 'F') {
			return false
		}
	}

	return true
}

func (c Config) baseURL() string {
	if c.Endpoint != "" {
		return strings.TrimSuffix(c.Endpoint, "/")
	}

	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", c.AccountID)
}

type Client struct {
	config Config
	now    func() time.Time
	doer   *http.Client
}

// New returns a signing client. Its methods report ErrNotConfigured when
// Config is incomplete, so callers can always build one.
func New(config Config) *Client {
	return &Client{
		config: config,
		now:    time.Now,
		doer:   &http.Client{Timeout: requestTimeout},
	}
}

// PresignPut returns a URL the browser can PUT to directly. The content type is
// part of the signature, so an object is stored with the type the server
// validated rather than whatever the client claims at upload time.
func (c *Client) PresignPut(objectKey, contentType string, expires time.Duration) (string, error) {
	if !c.config.Configured() {
		return "", ErrNotConfigured
	}

	amzDate, dateStamp := c.timestamps()
	scope := credentialScope(dateStamp)
	canonicalURI := c.canonicalURI(objectKey)

	signedHeaders := "content-type;host"
	canonicalHeaders := "content-type:" + strings.TrimSpace(contentType) + "\n" +
		"host:" + hostOf(c.config.baseURL()) + "\n"

	canonicalQuery := canonicalQueryString(map[string]string{
		"X-Amz-Algorithm":     signingAlgorithm,
		"X-Amz-Credential":    c.config.AccessKeyID + "/" + scope,
		"X-Amz-Date":          amzDate,
		"X-Amz-Expires":       strconv.Itoa(int(expires.Seconds())),
		"X-Amz-SignedHeaders": signedHeaders,
	})

	signature := c.signature(canonicalRequest(
		http.MethodPut, canonicalURI, canonicalQuery, canonicalHeaders, signedHeaders, unsignedPayload,
	), dateStamp, amzDate, scope)

	return c.config.baseURL() + canonicalURI + "?" + canonicalQuery + "&X-Amz-Signature=" + signature, nil
}

// Delete removes an object. A missing object is not an error, so repeated
// deletes stay safe.
func (c *Client) Delete(objectKey string) error {
	if !c.config.Configured() {
		return ErrNotConfigured
	}

	amzDate, dateStamp := c.timestamps()
	scope := credentialScope(dateStamp)
	canonicalURI := c.canonicalURI(objectKey)

	request, err := http.NewRequest(http.MethodDelete, c.config.baseURL()+canonicalURI, nil)
	if err != nil {
		return err
	}

	payloadHash := hexSHA256("")
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + request.URL.Host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"

	signature := c.signature(canonicalRequest(
		http.MethodDelete, canonicalURI, "", canonicalHeaders, signedHeaders, payloadHash,
	), dateStamp, amzDate, scope)

	request.Header.Set("x-amz-content-sha256", payloadHash)
	request.Header.Set("x-amz-date", amzDate)
	request.Header.Set("Authorization", fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		signingAlgorithm, c.config.AccessKeyID, scope, signedHeaders, signature,
	))

	response, err := c.doer.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return nil
	}
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		return fmt.Errorf("r2: delete %q returned status %d", objectKey, response.StatusCode)
	}

	return nil
}

func (c *Client) timestamps() (amzDate, dateStamp string) {
	now := c.now().UTC()
	return now.Format("20060102T150405Z"), now.Format("20060102")
}

func (c *Client) canonicalURI(objectKey string) string {
	return "/" + awsEscape(c.config.Bucket, false) + "/" + escapePath(objectKey)
}

func (c *Client) signature(canonicalRequest, dateStamp, amzDate, scope string) string {
	stringToSign := strings.Join([]string{
		signingAlgorithm,
		amzDate,
		scope,
		hexSHA256(canonicalRequest),
	}, "\n")

	return hex.EncodeToString(hmacSHA256(c.signingKey(dateStamp), stringToSign))
}

func (c *Client) signingKey(dateStamp string) []byte {
	dateKey := hmacSHA256([]byte("AWS4"+c.config.SecretAccessKey), dateStamp)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, service)

	return hmacSHA256(serviceKey, "aws4_request")
}

func credentialScope(dateStamp string) string {
	return strings.Join([]string{dateStamp, region, service, "aws4_request"}, "/")
}

func canonicalRequest(method, canonicalURI, canonicalQuery, canonicalHeaders, signedHeaders, payloadHash string) string {
	return strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
}

// canonicalQueryString sorts by key and RFC 3986 encodes both halves, which is
// what SigV4 expects for query-string authentication.
func canonicalQueryString(query map[string]string) string {
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, awsEscape(key, true)+"="+awsEscape(query[key], true))
	}

	return strings.Join(parts, "&")
}

func escapePath(objectKey string) string {
	segments := strings.Split(objectKey, "/")
	for index, segment := range segments {
		segments[index] = awsEscape(segment, false)
	}

	return strings.Join(segments, "/")
}

// awsEscape applies the RFC 3986 encoding SigV4 requires. Unlike
// url.QueryEscape it leaves unreserved characters alone and encodes spaces as
// %20 rather than +.
func awsEscape(value string, encodeSlash bool) string {
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		character := value[index]

		switch {
		case character >= 'A' && character <= 'Z',
			character >= 'a' && character <= 'z',
			character >= '0' && character <= '9',
			character == '-', character == '_', character == '.', character == '~':
			builder.WriteByte(character)
		case character == '/' && !encodeSlash:
			builder.WriteByte(character)
		default:
			fmt.Fprintf(&builder, "%%%02X", character)
		}
	}

	return builder.String()
}

func hostOf(baseURL string) string {
	return strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
}

func hexSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	return mac.Sum(nil)
}
