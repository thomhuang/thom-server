package r2

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// emptySHA256 is the SHA-256 of the empty string, which is the payload hash
// SigV4 requires for a request with no body.
const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

var testTime = time.Date(2026, 9, 16, 12, 36, 0, 0, time.UTC)

func newTestClient(endpoint string) *Client {
	client := New(Config{
		AccountID:       "test-account",
		Bucket:          "my-bucket",
		AccessKeyID:     "AKIDEXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		Endpoint:        endpoint,
	})
	client.now = func() time.Time { return testTime }

	return client
}

func TestEmptyPayloadHashMatchesKnownValue(t *testing.T) {
	if got := hexSHA256(""); got != emptySHA256 {
		t.Fatalf("hexSHA256(\"\") = %q, want %q", got, emptySHA256)
	}
}

func TestAwsEscapeFollowsRFC3986(t *testing.T) {
	testCases := []struct {
		name        string
		value       string
		encodeSlash bool
		want        string
	}{
		{name: "unreserved characters are untouched", value: "aZ09-_.~", want: "aZ09-_.~"},
		{name: "space becomes percent twenty", value: "a b", want: "a%20b"},
		{name: "slash is preserved when allowed", value: "a/b", want: "a/b"},
		{name: "slash is encoded when required", value: "a/b", encodeSlash: true, want: "a%2Fb"},
		{name: "hex is uppercase", value: "&", want: "%26"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := awsEscape(testCase.value, testCase.encodeSlash); got != testCase.want {
				t.Fatalf("awsEscape(%q) = %q, want %q", testCase.value, got, testCase.want)
			}
		})
	}
}

func TestEscapePathPreservesKeySeparators(t *testing.T) {
	got := escapePath("shop/12/photo 1.jpg")
	want := "shop/12/photo%201.jpg"

	if got != want {
		t.Fatalf("escapePath = %q, want %q", got, want)
	}
}

func TestCanonicalQueryStringSortsAndEncodes(t *testing.T) {
	got := canonicalQueryString(map[string]string{
		"b":                "2",
		"a":                "1",
		"X-Amz-Credential": "AKID/20260916/auto/s3/aws4_request",
	})
	want := "X-Amz-Credential=AKID%2F20260916%2Fauto%2Fs3%2Faws4_request&a=1&b=2"

	if got != want {
		t.Fatalf("canonicalQueryString = %q, want %q", got, want)
	}
}

func TestPresignPutBuildsExpectedURL(t *testing.T) {
	client := newTestClient("")

	presigned, err := client.PresignPut("shop/1/abc.jpg", "image/png", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := "https://test-account.r2.cloudflarestorage.com/my-bucket/shop/1/abc.jpg?" +
		"X-Amz-Algorithm=AWS4-HMAC-SHA256" +
		"&X-Amz-Credential=AKIDEXAMPLE%2F20260916%2Fauto%2Fs3%2Faws4_request" +
		"&X-Amz-Date=20260916T123600Z" +
		"&X-Amz-Expires=600" +
		"&X-Amz-SignedHeaders=content-type%3Bhost" +
		"&X-Amz-Signature="

	if !strings.HasPrefix(presigned, wantPrefix) {
		t.Fatalf("presigned URL =\n%s\nwant prefix\n%s", presigned, wantPrefix)
	}

	signature := strings.TrimPrefix(presigned, wantPrefix)
	if len(signature) != 64 {
		t.Fatalf("signature length = %d, want 64 (%q)", len(signature), signature)
	}
	for _, character := range signature {
		if !strings.ContainsRune("0123456789abcdef", character) {
			t.Fatalf("signature %q contains non-hex character %q", signature, character)
		}
	}
}

func TestPresignPutIsDeterministicForFixedClock(t *testing.T) {
	client := newTestClient("")

	first, err := client.PresignPut("shop/1/abc.jpg", "image/png", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.PresignPut("shop/1/abc.jpg", "image/png", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatalf("presign is not deterministic:\n%s\n%s", first, second)
	}
}

func TestPresignPutVariesWithEverySignedInput(t *testing.T) {
	client := newTestClient("")

	baseline, err := client.PresignPut("shop/1/abc.jpg", "image/png", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	variants := map[string]func() (string, error){
		"expiry": func() (string, error) {
			return client.PresignPut("shop/1/abc.jpg", "image/png", 5*time.Minute)
		},
		"object key": func() (string, error) {
			return client.PresignPut("shop/1/other.jpg", "image/png", 10*time.Minute)
		},
		"content type": func() (string, error) {
			return client.PresignPut("shop/1/abc.jpg", "image/jpeg", 10*time.Minute)
		},
	}

	for name, variant := range variants {
		t.Run(name, func(t *testing.T) {
			signed, err := variant()
			if err != nil {
				t.Fatal(err)
			}
			if signatureOf(t, signed) == signatureOf(t, baseline) {
				t.Fatalf("changing the %s did not change the signature", name)
			}
		})
	}
}

func TestPresignPutRequiresCredentials(t *testing.T) {
	client := New(Config{Bucket: "my-bucket"})

	if _, err := client.PresignPut("shop/1/abc.jpg", "image/png", time.Minute); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestDeleteSendsSignedRequest(t *testing.T) {
	var captured *http.Request
	var authorization string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestClient(server.URL)

	if err := client.Delete("shop/7/photo.jpg"); err != nil {
		t.Fatal(err)
	}

	if captured.Method != http.MethodDelete {
		t.Fatalf("method = %s, want DELETE", captured.Method)
	}
	if captured.URL.Path != "/my-bucket/shop/7/photo.jpg" {
		t.Fatalf("path = %q, want /my-bucket/shop/7/photo.jpg", captured.URL.Path)
	}
	if got := captured.Header.Get("x-amz-content-sha256"); got != emptySHA256 {
		t.Fatalf("x-amz-content-sha256 = %q, want %q", got, emptySHA256)
	}
	if got := captured.Header.Get("x-amz-date"); got != "20260916T123600Z" {
		t.Fatalf("x-amz-date = %q, want 20260916T123600Z", got)
	}

	wantAuthorizationPrefix := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20260916/auto/s3/aws4_request, " +
		"SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature="
	if !strings.HasPrefix(authorization, wantAuthorizationPrefix) {
		t.Fatalf("Authorization =\n%s\nwant prefix\n%s", authorization, wantAuthorizationPrefix)
	}
}

func TestDeleteTreatsMissingObjectAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server.URL)

	if err := client.Delete("shop/7/missing.jpg"); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestDeleteReportsUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(server.URL)

	err := client.Delete("shop/7/photo.jpg")
	if err == nil {
		t.Fatal("err = nil, want an error for status 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want it to mention status 403", err)
	}
}

func signatureOf(t *testing.T, presigned string) string {
	t.Helper()

	parsed, err := url.Parse(presigned)
	if err != nil {
		t.Fatal(err)
	}

	signature := parsed.Query().Get("X-Amz-Signature")
	if signature == "" {
		t.Fatalf("no signature in %q", presigned)
	}

	return signature
}

func TestValidateAcceptsWellFormedCredentials(t *testing.T) {
	config := Config{
		AccountID:       "test-account",
		Bucket:          "my-bucket",
		AccessKeyID:     strings.Repeat("a1", 16),
		SecretAccessKey: strings.Repeat("b", 64),
	}

	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidateAllowsUnconfiguredConfig(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil because image uploads are optional", err)
	}
}

func TestValidateRejectsMalformedCredentials(t *testing.T) {
	valid := Config{
		AccountID:       "test-account",
		Bucket:          "my-bucket",
		AccessKeyID:     strings.Repeat("a1", 16),
		SecretAccessKey: strings.Repeat("b", 64),
	}

	testCases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name:   "truncated access key",
			mutate: func(config *Config) { config.AccessKeyID = strings.Repeat("a", 31) },
			want:   "access key ID",
		},
		{
			name:   "non-hex access key",
			mutate: func(config *Config) { config.AccessKeyID = strings.Repeat("z", 32) },
			want:   "access key ID",
		},
		{
			name:   "truncated secret",
			mutate: func(config *Config) { config.SecretAccessKey = strings.Repeat("b", 63) },
			want:   "secret access key",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := valid
			testCase.mutate(&config)

			err := config.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to mention %q", err, testCase.want)
			}
		})
	}
}
