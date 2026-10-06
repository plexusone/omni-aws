package ses

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/plexusone/omnimail"
)

// codeKinds maps SES v2 and common AWS error codes to failure kinds.
var codeKinds = map[string]omnimail.Kind{
	// Rate and quota limits.
	"TooManyRequestsException": omnimail.KindThrottled,
	"LimitExceededException":   omnimail.KindThrottled,
	"ThrottlingException":      omnimail.KindThrottled,
	"Throttling":               omnimail.KindThrottled,

	// Policy and account-state rejections.
	"MessageRejected":                    omnimail.KindRejected,
	"MailFromDomainNotVerifiedException": omnimail.KindRejected,
	"AccountSuspendedException":          omnimail.KindRejected,
	"SendingPausedException":             omnimail.KindRejected,
	"NotFoundException":                  omnimail.KindRejected,

	// Malformed requests.
	"BadRequestException":    omnimail.KindInvalidMessage,
	"ValidationException":    omnimail.KindInvalidMessage,
	"SerializationException": omnimail.KindInvalidMessage,

	// Credentials and permissions.
	"AccessDeniedException":       omnimail.KindAuth,
	"AccessDenied":                omnimail.KindAuth,
	"UnrecognizedClientException": omnimail.KindAuth,
	"InvalidClientTokenId":        omnimail.KindAuth,
	"InvalidSignatureException":   omnimail.KindAuth,
	"SignatureDoesNotMatch":       omnimail.KindAuth,
	"IncompleteSignature":         omnimail.KindAuth,
	"MissingAuthenticationToken":  omnimail.KindAuth,
	"ExpiredToken":                omnimail.KindAuth,
	"ExpiredTokenException":       omnimail.KindAuth,
	"NotAuthorized":               omnimail.KindAuth,

	// Service-side faults.
	"InternalServiceErrorException": omnimail.KindTransient,
	"InternalFailure":               omnimail.KindTransient,
	"ServiceUnavailable":            omnimail.KindTransient,
	"ServiceUnavailableException":   omnimail.KindTransient,
	"RequestTimeout":                omnimail.KindTransient,
	"RequestTimeoutException":       omnimail.KindTransient,
}

// addressHints mark a BadRequestException that is about an address rather
// than the message as a whole.
var addressHints = []string{"address", "recipient", "domain", "mailbox"}

// Classify maps an SES v2 SendEmail error to an [*omnimail.Error] with
// Provider "ses", Code set to the SES error code, RetryAfter from a
// Retry-After response header, and err as the cause:
//
//   - TooManyRequestsException, LimitExceededException, throttling: KindThrottled
//   - MessageRejected, MailFromDomainNotVerifiedException,
//     AccountSuspendedException, SendingPausedException, NotFoundException:
//     KindRejected
//   - BadRequestException: KindInvalidAddress when the message names an
//     address, recipient, domain or mailbox, otherwise KindInvalidMessage
//   - AccessDeniedException, UnrecognizedClientException, signature and
//     expired-token errors, HTTP 401/403: KindAuth
//   - InternalServiceErrorException, HTTP 5xx, timeouts and network errors:
//     KindTransient
//   - HTTP 429: KindThrottled
//
// Anything else is KindUnknown. A nil err returns nil.
func Classify(err error) *omnimail.Error {
	if err == nil {
		return nil
	}
	e := &omnimail.Error{Kind: omnimail.KindUnknown, Provider: ProviderName, Err: err}

	status := 0
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.Response != nil && respErr.Response.Response != nil {
		status = respErr.Response.StatusCode
		e.RetryAfter = parseRetryAfter(respErr.Response.Header.Get("Retry-After"), time.Now())
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		e.Code = apiErr.ErrorCode()
		e.Message = apiErr.ErrorMessage()
		if kind, ok := codeKinds[e.Code]; ok {
			e.Kind = kind
			if kind == omnimail.KindInvalidMessage && e.Code == "BadRequestException" && mentionsAddress(e.Message) {
				e.Kind = omnimail.KindInvalidAddress
			}
			return e
		}
	}

	switch {
	case status == http.StatusTooManyRequests:
		e.Kind = omnimail.KindThrottled
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		e.Kind = omnimail.KindAuth
	case status >= 500 && status < 600:
		e.Kind = omnimail.KindTransient
	case status == 0 && isNetworkError(err):
		e.Kind = omnimail.KindTransient
	}
	return e
}

func mentionsAddress(msg string) bool {
	lower := strings.ToLower(msg)
	for _, h := range addressHints {
		if strings.Contains(lower, h) {
			return true
		}
	}
	return false
}

func isNetworkError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed)
}

// parseRetryAfter parses a Retry-After header given as delay seconds or an
// HTTP date. It returns 0 when the header is absent or invalid.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}
