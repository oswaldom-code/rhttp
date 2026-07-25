package rhttp_test

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"syscall"
	"testing"

	"github.com/oswaldom-code/rhttp"
)

func TestClassify_DeadlineExceeded(t *testing.T) {
	classified := rhttp.Classify(context.DeadlineExceeded)

	if classified.Kind != rhttp.ErrKindTimeout {
		t.Errorf("expected ErrKindTimeout, got %v", classified.Kind)
	}
	if !errors.Is(classified, context.DeadlineExceeded) {
		t.Error("expected Unwrap to return original error")
	}
}

func TestClassify_Canceled(t *testing.T) {
	classified := rhttp.Classify(context.Canceled)

	if classified.Kind != rhttp.ErrKindCanceled {
		t.Errorf("expected ErrKindCanceled, got %v", classified.Kind)
	}
}

func TestClassify_DNSError(t *testing.T) {
	dnsErr := &net.DNSError{
		Err:  "no such host",
		Name: "invalid.example.com",
	}
	classified := rhttp.Classify(dnsErr)

	if classified.Kind != rhttp.ErrKindDNS {
		t.Errorf("expected ErrKindDNS, got %v", classified.Kind)
	}
}

func TestClassify_ConnectionRefused(t *testing.T) {
	err := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	classified := rhttp.Classify(err)

	if classified.Kind != rhttp.ErrKindConnection {
		t.Errorf("expected ErrKindConnection, got %v", classified.Kind)
	}
}

func TestClassify_ConnectionReset(t *testing.T) {
	err := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	classified := rhttp.Classify(err)

	if classified.Kind != rhttp.ErrKindConnection {
		t.Errorf("expected ErrKindConnection, got %v", classified.Kind)
	}
}

func TestClassify_TLSError(t *testing.T) {
	err := x509.UnknownAuthorityError{}
	classified := rhttp.Classify(err)

	if classified.Kind != rhttp.ErrKindTLS {
		t.Errorf("expected ErrKindTLS, got %v", classified.Kind)
	}
}

func TestClassify_X509Error(t *testing.T) {
	err := x509.CertificateInvalidError{Reason: x509.Expired}
	classified := rhttp.Classify(err)

	if classified.Kind != rhttp.ErrKindTLS {
		t.Errorf("expected ErrKindTLS, got %v", classified.Kind)
	}
}

func TestClassify_WrappedURLError(t *testing.T) {
	urlErr := &url.Error{
		Op:  "Get",
		URL: "http://example.com",
		Err: context.DeadlineExceeded,
	}
	classified := rhttp.Classify(urlErr)

	if classified.Kind != rhttp.ErrKindTimeout {
		t.Errorf("expected ErrKindTimeout for wrapped deadline, got %v", classified.Kind)
	}
}

func TestClassify_URLErrorTimeout(t *testing.T) {
	urlErr := &url.Error{
		Op:  "Get",
		URL: "http://example.com",
		Err: &timeoutError{},
	}
	classified := rhttp.Classify(urlErr)

	if classified.Kind != rhttp.ErrKindTimeout {
		t.Errorf("expected ErrKindTimeout, got %v", classified.Kind)
	}
}

// timeoutError implements net.Error with Timeout() = true
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestClassify_NilError(t *testing.T) {
	classified := rhttp.Classify(nil)

	if classified != nil {
		t.Error("expected nil for nil error")
	}
}

func TestClassify_UnknownError(t *testing.T) {
	err := errors.New("something completely unexpected")
	classified := rhttp.Classify(err)

	if classified.Kind != rhttp.ErrKindUnknown {
		t.Errorf("expected ErrKindUnknown, got %v", classified.Kind)
	}
}

func TestClassifiedError_Error(t *testing.T) {
	classified := rhttp.Classify(syscall.ECONNREFUSED)

	expected := "connection: " + syscall.ECONNREFUSED.Error()
	if classified.Error() != expected {
		t.Errorf("expected %q, got %q", expected, classified.Error())
	}
}

func TestClassifiedError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	classified := rhttp.Classify(originalErr)

	if !errors.Is(classified, originalErr) {
		t.Error("errors.Is should match original error")
	}
}

func TestErrorKind_String(t *testing.T) {
	tests := []struct {
		kind     rhttp.ErrorKind
		expected string
	}{
		{rhttp.ErrKindTimeout, "timeout"},
		{rhttp.ErrKindCanceled, "canceled"},
		{rhttp.ErrKindConnection, "connection"},
		{rhttp.ErrKindDNS, "dns"},
		{rhttp.ErrKindTLS, "tls"},
		{rhttp.ErrKindUnknown, "unknown"},
	}

	for _, tt := range tests {
		if tt.kind.String() != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, tt.kind.String())
		}
	}
}

func TestErrorKind_IsRetryable(t *testing.T) {
	retryable := []rhttp.ErrorKind{
		rhttp.ErrKindTimeout,
		rhttp.ErrKindConnection,
		rhttp.ErrKindDNS,
	}
	for _, k := range retryable {
		if !k.IsRetryable() {
			t.Errorf("expected %v to be retryable", k)
		}
	}

	notRetryable := []rhttp.ErrorKind{
		rhttp.ErrKindCanceled,
		rhttp.ErrKindTLS,
		rhttp.ErrKindUnknown,
	}
	for _, k := range notRetryable {
		if k.IsRetryable() {
			t.Errorf("expected %v to not be retryable", k)
		}
	}
}

func TestIsTimeout(t *testing.T) {
	if !rhttp.IsTimeout(context.DeadlineExceeded) {
		t.Error("expected IsTimeout to be true for DeadlineExceeded")
	}
	if rhttp.IsTimeout(context.Canceled) {
		t.Error("expected IsTimeout to be false for Canceled")
	}
	if rhttp.IsTimeout(nil) {
		t.Error("expected IsTimeout to be false for nil")
	}
}

func TestIsCanceled(t *testing.T) {
	if !rhttp.IsCanceled(context.Canceled) {
		t.Error("expected IsCanceled to be true for Canceled")
	}
	if rhttp.IsCanceled(context.DeadlineExceeded) {
		t.Error("expected IsCanceled to be false for DeadlineExceeded")
	}
	if rhttp.IsCanceled(nil) {
		t.Error("expected IsCanceled to be false for nil")
	}
}

func TestIsConnection(t *testing.T) {
	if !rhttp.IsConnection(syscall.ECONNREFUSED) {
		t.Error("expected IsConnection to be true for ECONNREFUSED")
	}
	if rhttp.IsConnection(context.Canceled) {
		t.Error("expected IsConnection to be false for Canceled")
	}
}

func TestIsDNS(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "invalid.example.com"}
	if !rhttp.IsDNS(dnsErr) {
		t.Error("expected IsDNS to be true for DNSError")
	}
	if rhttp.IsDNS(context.Canceled) {
		t.Error("expected IsDNS to be false for Canceled")
	}
}

func TestIsTLS(t *testing.T) {
	if !rhttp.IsTLS(x509.UnknownAuthorityError{}) {
		t.Error("expected IsTLS to be true for TLS error")
	}
	if rhttp.IsTLS(context.Canceled) {
		t.Error("expected IsTLS to be false for Canceled")
	}
}

func TestClassify_AllKindsAreReachable(t *testing.T) {
	producers := map[rhttp.ErrorKind]error{
		rhttp.ErrKindUnknown:    errors.New("something completely unexpected"),
		rhttp.ErrKindTimeout:    context.DeadlineExceeded,
		rhttp.ErrKindCanceled:   context.Canceled,
		rhttp.ErrKindConnection: syscall.ECONNREFUSED,
		rhttp.ErrKindDNS:        &net.DNSError{Err: "no such host"},
		rhttp.ErrKindTLS:        x509.UnknownAuthorityError{},
	}

	for kind, err := range producers {
		if got := rhttp.Classify(err).Kind; got != kind {
			t.Errorf("expected %v to classify as %v, got %v", err, kind, got)
		}
	}

	for k := rhttp.ErrKindUnknown; ; k++ {
		if k != rhttp.ErrKindUnknown && k.String() == "unknown" {
			break
		}
		if _, ok := producers[k]; !ok {
			t.Errorf("ErrorKind %d (%s) has no producing error: orphaned kind", k, k)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	// Retryable
	if !rhttp.IsRetryable(context.DeadlineExceeded) {
		t.Error("expected timeout to be retryable")
	}
	if !rhttp.IsRetryable(syscall.ECONNREFUSED) {
		t.Error("expected connection error to be retryable")
	}

	// Not retryable
	if rhttp.IsRetryable(context.Canceled) {
		t.Error("expected canceled to not be retryable")
	}
	if rhttp.IsRetryable(x509.UnknownAuthorityError{}) {
		t.Error("expected TLS error to not be retryable")
	}
	if rhttp.IsRetryable(nil) {
		t.Error("expected nil to not be retryable")
	}
}
