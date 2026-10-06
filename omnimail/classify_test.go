package ses

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/smithy-go"

	"github.com/plexusone/omnimail"
)

// TestClassifyResponses drives real SES v2 SDK errors through the fake
// endpoint, so the SDK's error deserialization is part of the test.
func TestClassifyResponses(t *testing.T) {
	cases := []struct {
		name       string
		fail       fakeFailure
		kind       omnimail.Kind
		retryAfter time.Duration
	}{
		{"TooManyRequests", fakeFailure{429, "TooManyRequestsException", "Maximum sending rate exceeded.", ""}, omnimail.KindThrottled, 0},
		{"TooManyRequestsRetryAfter", fakeFailure{429, "TooManyRequestsException", "slow down", "3"}, omnimail.KindThrottled, 3 * time.Second},
		{"LimitExceeded", fakeFailure{400, "LimitExceededException", "Daily message quota exceeded.", ""}, omnimail.KindThrottled, 0},
		{"Throttling", fakeFailure{400, "ThrottlingException", "Rate exceeded", ""}, omnimail.KindThrottled, 0},
		{"MessageRejected", fakeFailure{400, "MessageRejected", "Email address is not verified.", ""}, omnimail.KindRejected, 0},
		{"MailFromNotVerified", fakeFailure{400, "MailFromDomainNotVerifiedException", "MAIL FROM domain not verified", ""}, omnimail.KindRejected, 0},
		{"AccountSuspended", fakeFailure{400, "AccountSuspendedException", "account suspended", ""}, omnimail.KindRejected, 0},
		{"SendingPaused", fakeFailure{400, "SendingPausedException", "sending paused", ""}, omnimail.KindRejected, 0},
		{"ConfigurationSetNotFound", fakeFailure{404, "NotFoundException", "Configuration set does not exist", ""}, omnimail.KindRejected, 0},
		{"BadRequest", fakeFailure{400, "BadRequestException", "Message length is more than 40 MB", ""}, omnimail.KindInvalidMessage, 0},
		{"BadRequestAddress", fakeFailure{400, "BadRequestException", "Illegal address", ""}, omnimail.KindInvalidAddress, 0},
		{"BadRequestDomain", fakeFailure{400, "BadRequestException", "Missing final '@domain'", ""}, omnimail.KindInvalidAddress, 0},
		{"AccessDenied", fakeFailure{403, "AccessDeniedException", "not authorized", ""}, omnimail.KindAuth, 0},
		{"UnrecognizedClient", fakeFailure{403, "UnrecognizedClientException", "The security token included in the request is invalid.", ""}, omnimail.KindAuth, 0},
		{"ExpiredToken", fakeFailure{400, "ExpiredTokenException", "The security token included in the request is expired", ""}, omnimail.KindAuth, 0},
		{"SignatureMismatch", fakeFailure{403, "InvalidSignatureException", "signature mismatch", ""}, omnimail.KindAuth, 0},
		{"InternalServiceError", fakeFailure{500, "InternalServiceErrorException", "internal error", ""}, omnimail.KindTransient, 0},
		{"ServiceUnavailable", fakeFailure{503, "ServiceUnavailable", "unavailable", ""}, omnimail.KindTransient, 0},
		{"Unknown5xx", fakeFailure{502, "SomethingNew", "bad gateway", ""}, omnimail.KindTransient, 0},
		{"Unknown429", fakeFailure{429, "SomethingNew", "slow", "1"}, omnimail.KindThrottled, time.Second},
		{"Unknown401", fakeFailure{401, "SomethingNew", "who are you", ""}, omnimail.KindAuth, 0},
		{"Unknown4xx", fakeFailure{409, "ConflictException", "conflict", ""}, omnimail.KindUnknown, 0},
	}
	fake := newFakeSES(t)
	s := fake.newSender(t, Config{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake.reset()
			fake.fail(tc.fail)
			_, err := s.Send(context.Background(), testMessage())
			var e *omnimail.Error
			if !errors.As(err, &e) {
				t.Fatalf("err = %v, want *omnimail.Error", err)
			}
			if e.Kind != tc.kind {
				t.Errorf("Kind = %s, want %s (err %v)", e.Kind, tc.kind, err)
			}
			if e.Provider != ProviderName {
				t.Errorf("Provider = %q", e.Provider)
			}
			if e.Code != tc.fail.errorType {
				t.Errorf("Code = %q, want %q", e.Code, tc.fail.errorType)
			}
			if e.Message != tc.fail.message {
				t.Errorf("Message = %q, want %q", e.Message, tc.fail.message)
			}
			if e.RetryAfter != tc.retryAfter {
				t.Errorf("RetryAfter = %v, want %v", e.RetryAfter, tc.retryAfter)
			}
			wantRetry := tc.kind == omnimail.KindThrottled || tc.kind == omnimail.KindTransient
			if omnimail.IsRetryable(err) != wantRetry {
				t.Errorf("IsRetryable = %v, want %v", omnimail.IsRetryable(err), wantRetry)
			}
			var apiErr smithy.APIError
			if !errors.As(err, &apiErr) {
				t.Error("SDK error not preserved in the chain")
			}
		})
	}
}

func TestClassifyErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		kind omnimail.Kind
		code string
	}{
		{"GenericAPIError", &smithy.GenericAPIError{Code: "SendingPausedException", Message: "paused"}, omnimail.KindRejected, "SendingPausedException"},
		{"WrappedAPIError", fmt.Errorf("operation error: %w", &smithy.GenericAPIError{Code: "TooManyRequestsException"}), omnimail.KindThrottled, "TooManyRequestsException"},
		{"UnknownAPIError", &smithy.GenericAPIError{Code: "Mystery"}, omnimail.KindUnknown, "Mystery"},
		{"NetOpError", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, omnimail.KindTransient, ""},
		{"DNSError", &net.DNSError{Err: "no such host", Name: "email.us-east-1.amazonaws.com", IsTimeout: true}, omnimail.KindTransient, ""},
		{"UnexpectedEOF", fmt.Errorf("read: %w", io.ErrUnexpectedEOF), omnimail.KindTransient, ""},
		{"Plain", errors.New("something odd"), omnimail.KindUnknown, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Classify(tc.err)
			if e.Kind != tc.kind || e.Code != tc.code || e.Provider != ProviderName || !errors.Is(e, tc.err) {
				t.Errorf("Classify = %+v, want kind %s code %q wrapping the cause", e, tc.kind, tc.code)
			}
		})
	}
	if Classify(nil) != nil {
		t.Error("Classify(nil) != nil")
	}
}

func TestClassifyConnectionRefused(t *testing.T) {
	fake := newFakeSES(t)
	url := fake.URL
	fake.Close()
	s, err := New(Config{Client: sesv2.New(sesv2.Options{
		BaseEndpoint:     aws.String(url),
		Region:           "us-east-1",
		Credentials:      credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "SECRET", ""),
		RetryMaxAttempts: 1,
	})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Send(context.Background(), testMessage())
	if omnimail.KindOf(err) != omnimail.KindTransient || !omnimail.IsRetryable(err) {
		t.Fatalf("err = %v, want retryable transient", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := map[string]time.Duration{
		"":    0,
		"5":   5 * time.Second,
		" 12": 12 * time.Second,
		"-1":  0,
		"abc": 0,
		now.Add(30 * time.Second).Format(http.TimeFormat): 30 * time.Second,
		now.Add(-time.Minute).Format(http.TimeFormat):     0,
	}
	for in, want := range cases {
		if got := parseRetryAfter(in, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}
