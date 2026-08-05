package rhttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"syscall"
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

	// ErrKindDNS indicates a transient DNS resolution failure.
	ErrKindDNS

	// ErrKindTLS indicates a TLS/SSL error.
	ErrKindTLS

	// ErrKindDNSNotFound indicates the name does not exist (NXDOMAIN). Unlike
	// ErrKindDNS this is permanent: retrying the same name cannot succeed.
	ErrKindDNSNotFound
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
	case ErrKindDNSNotFound:
		return "dns_not_found"
	case ErrKindTLS:
		return "tls"
	default:
		return "unknown"
	}
}

// IsRetryable returns true if the error kind is typically safe to retry.
func (k ErrorKind) IsRetryable() bool {
	switch k {
	case ErrKindTimeout, ErrKindConnection, ErrKindDNS:
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

func classifyTLS(err error) ErrorKind {
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return ErrKindTLS
	}

	var unknownAuthErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthErr) {
		return ErrKindTLS
	}

	var invalidCertErr x509.CertificateInvalidError
	if errors.As(err, &invalidCertErr) {
		return ErrKindTLS
	}

	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return ErrKindTLS
	}

	return ErrKindUnknown
}

func classifyConnection(err error) ErrorKind {
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) {
		return ErrKindConnection
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" || opErr.Op == "read" || opErr.Op == "write" {
			return ErrKindConnection
		}
	}

	return ErrKindUnknown
}

func classifyError(err error) ErrorKind {
	if err == nil {
		return ErrKindUnknown
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return ErrKindTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrKindCanceled
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return ErrKindTimeout
		}
		if urlErr.Err != nil {
			return classifyError(urlErr.Err)
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrKindTimeout
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return ErrKindDNSNotFound
		}
		return ErrKindDNS
	}

	if kind := classifyTLS(err); kind != ErrKindUnknown {
		return kind
	}

	return classifyConnection(err)
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

// IsDNS returns true if the error is a DNS error, whether transient or
// permanent. Use IsDNSNotFound to single out the permanent case.
func IsDNS(err error) bool {
	if err == nil {
		return false
	}
	classified := Classify(err)
	return classified.Kind == ErrKindDNS || classified.Kind == ErrKindDNSNotFound
}

// IsDNSNotFound returns true if the name does not exist (NXDOMAIN). Such an
// error is permanent and is never retried by DefaultIsRetryable.
func IsDNSNotFound(err error) bool {
	if err == nil {
		return false
	}
	classified := Classify(err)
	return classified.Kind == ErrKindDNSNotFound
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
