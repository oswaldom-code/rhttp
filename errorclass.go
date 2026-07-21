package rhttp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"
)

// ErrorKind represents the category of an HTTP client error.
type ErrorKind int

const (
	// ErrKindUnknown is an unclassified error.
	ErrKindUnknown ErrorKind = iota

	// ErrKindTimeout indicates the request timed out.
	ErrKindTimeout

	// ErrKindCanceled indicates the request was canceled by the caller.
	ErrKindCanceled

	// ErrKindConnection indicates a connection error (refused, reset, etc.).
	ErrKindConnection

	// ErrKindDNS indicates a DNS resolution failure.
	ErrKindDNS

	// ErrKindTLS indicates a TLS/SSL error.
	ErrKindTLS

	// ErrKindTemporary indicates a temporary error that may resolve on retry.
	ErrKindTemporary
)

// String returns a human-readable name for the error kind.
func (k ErrorKind) String() string {
	switch k {
	case ErrKindTimeout:
		return "timeout"
	case ErrKindCanceled:
		return "canceled"
	case ErrKindConnection:
		return "connection"
	case ErrKindDNS:
		return "dns"
	case ErrKindTLS:
		return "tls"
	case ErrKindTemporary:
		return "temporary"
	default:
		return "unknown"
	}
}

// IsRetryable returns true if the error kind is typically safe to retry.
func (k ErrorKind) IsRetryable() bool {
	switch k {
	case ErrKindTimeout, ErrKindConnection, ErrKindDNS, ErrKindTemporary:
		return true
	default:
		return false
	}
}

// ClassifiedError wraps an error with its classification.
type ClassifiedError struct {
	Kind ErrorKind
	Err  error
}

// Error returns a string representation of the classified error.
func (e *ClassifiedError) Error() string {
	if e.Err == nil {
		return e.Kind.String() + " error"
	}
	return e.Kind.String() + ": " + e.Err.Error()
}

// Unwrap returns the underlying error, allowing use with errors.Is and errors.As.
func (e *ClassifiedError) Unwrap() error {
	return e.Err
}

// Classify analyzes an error and returns its classification.
func Classify(err error) *ClassifiedError {
	if err == nil {
		return nil
	}

	kind := classifyError(err)
	return &ClassifiedError{
		Kind: kind,
		Err:  err,
	}
}

//nolint:gocognit,gocyclo // error classification inherently requires multiple checks
func classifyError(err error) ErrorKind {
	if err == nil {
		return ErrKindUnknown
	}

	// Check for context errors first
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrKindTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrKindCanceled
	}

	// Check for URL errors (often wrap other errors)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return ErrKindTimeout
		}
		// Classify the wrapped error
		if urlErr.Err != nil {
			return classifyError(urlErr.Err)
		}
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrKindTimeout
		}
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ErrKindDNS
	}

	// Check for operation errors (connection refused, etc.)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			return ErrKindTimeout
		}
		// Connection errors
		if opErr.Op == "dial" || opErr.Op == "read" || opErr.Op == "write" {
			return ErrKindConnection
		}
	}

	// Check for TLS errors
	var tlsErr *tls.CertificateVerificationError
	if errors.As(err, &tlsErr) {
		return ErrKindTLS
	}

	// Check error message for common patterns
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "no route to host") ||
		strings.Contains(errMsg, "network is unreachable") {
		return ErrKindConnection
	}
	if strings.Contains(errMsg, "tls") ||
		strings.Contains(errMsg, "certificate") ||
		strings.Contains(errMsg, "x509") {
		return ErrKindTLS
	}
	if strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "lookup") {
		return ErrKindDNS
	}

	return ErrKindUnknown
}

// IsTimeout returns true if the error is a timeout error.
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	classified := Classify(err)
	return classified.Kind == ErrKindTimeout
}

// IsCanceled returns true if the error is a cancellation error.
func IsCanceled(err error) bool {
	if err == nil {
		return false
	}
	classified := Classify(err)
	return classified.Kind == ErrKindCanceled
}

// IsConnection returns true if the error is a connection error.
func IsConnection(err error) bool {
	if err == nil {
		return false
	}
	classified := Classify(err)
	return classified.Kind == ErrKindConnection
}

// IsDNS returns true if the error is a DNS error.
func IsDNS(err error) bool {
	if err == nil {
		return false
	}
	classified := Classify(err)
	return classified.Kind == ErrKindDNS
}

// IsTLS returns true if the error is a TLS error.
func IsTLS(err error) bool {
	if err == nil {
		return false
	}
	classified := Classify(err)
	return classified.Kind == ErrKindTLS
}

// IsRetryable returns true if the error is typically safe to retry.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	classified := Classify(err)
	return classified.Kind.IsRetryable()
}
