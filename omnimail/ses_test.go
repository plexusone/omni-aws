package ses

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/plexusone/omnimail"
	"github.com/plexusone/omnimail/providertest"
)

func TestConformance(t *testing.T) {
	fake := newFakeSES(t)
	providertest.RunAll(t, providertest.Config{
		Harness:      &harness{fake: fake, sender: fake.newSender(t, Config{})},
		Provider:     ProviderName,
		SupportsTags: true,
		// SES has no idempotency mechanism and does not support SMTPUTF8.
		SupportsIdempotencyKey: false,
		SupportsSMTPUTF8:       false,
	})
}

func TestConformanceWithConfigurationSet(t *testing.T) {
	fake := newFakeSES(t)
	providertest.RunAll(t, providertest.Config{
		Harness: &harness{fake: fake, sender: fake.newSender(t, Config{
			ConfigurationSetName: "transactional",
		})},
		Provider:     ProviderName,
		SupportsTags: true,
	})
}

func testMessage() *omnimail.Message {
	return &omnimail.Message{
		From:    omnimail.Address{Name: "Example", Email: "no-reply@example.com"},
		To:      []omnimail.Address{{Name: "To", Email: "to@example.org"}},
		Cc:      []omnimail.Address{{Email: "cc@example.org"}},
		Bcc:     []omnimail.Address{{Email: "bcc@example.org"}},
		Subject: "Verify your email",
		Text:    "Open the link.\n",
	}
}

func TestSendRequest(t *testing.T) {
	fake := newFakeSES(t)
	s := fake.newSender(t, Config{
		ConfigurationSetName:           "transactional",
		FromEmailAddressIdentityArn:    "arn:aws:ses:us-east-1:123456789012:identity/example.com",
		FeedbackForwardingEmailAddress: "bounces@example.com",
		DefaultTags:                    map[string]string{"app": "web", "purpose": "default"},
	})
	msg := testMessage()
	msg.Tags = map[string]string{"purpose": "verify_email"}
	msg.IdempotencyKey = "key-1"

	res, err := s.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Provider != ProviderName || !strings.HasPrefix(res.MessageID, "0100018f-fake-") {
		t.Errorf("result = %+v", res)
	}
	reqs := fake.received()
	if len(reqs) != 1 {
		t.Fatalf("received %d requests, want 1", len(reqs))
	}
	req := reqs[0]
	if req.ConfigurationSetName != "transactional" {
		t.Errorf("ConfigurationSetName = %q", req.ConfigurationSetName)
	}
	if req.FromEmailAddress != "no-reply@example.com" {
		t.Errorf("FromEmailAddress = %q", req.FromEmailAddress)
	}
	if req.FromEmailAddressIdentityArn != "arn:aws:ses:us-east-1:123456789012:identity/example.com" {
		t.Errorf("FromEmailAddressIdentityArn = %q", req.FromEmailAddressIdentityArn)
	}
	if req.FeedbackForwardingEmailAddress != "bounces@example.com" {
		t.Errorf("FeedbackForwardingEmailAddress = %q", req.FeedbackForwardingEmailAddress)
	}
	if !reflect.DeepEqual(req.Destination.ToAddresses, []string{"to@example.org"}) ||
		!reflect.DeepEqual(req.Destination.CcAddresses, []string{"cc@example.org"}) ||
		!reflect.DeepEqual(req.Destination.BccAddresses, []string{"bcc@example.org"}) {
		t.Errorf("Destination = %+v", req.Destination)
	}
	var names, values []string
	for _, tag := range req.EmailTags {
		names = append(names, tag.Name)
		values = append(values, tag.Value)
	}
	if !reflect.DeepEqual(names, []string{"app", "purpose"}) || !reflect.DeepEqual(values, []string{"web", "verify_email"}) {
		t.Errorf("EmailTags = %+v, want sorted defaults overridden by message tags", req.EmailTags)
	}
	raw, err := base64.StdEncoding.DecodeString(req.Content.Raw.Data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(raw)), "bcc") {
		t.Errorf("raw message leaks Bcc:\n%s", raw)
	}
	if strings.Contains(string(raw), "key-1") {
		t.Errorf("idempotency key must not be transmitted:\n%s", raw)
	}
	if msg.Tags["purpose"] != "verify_email" || len(msg.Tags) != 1 {
		t.Errorf("message tags modified: %v", msg.Tags)
	}
}

func TestSendOmitsOptionalFields(t *testing.T) {
	fake := newFakeSES(t)
	s := fake.newSender(t, Config{})
	msg := testMessage()
	msg.Cc, msg.Bcc = nil, nil
	if _, err := s.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	req := fake.received()[0]
	if req.ConfigurationSetName != "" || req.FromEmailAddressIdentityArn != "" || req.FeedbackForwardingEmailAddress != "" {
		t.Errorf("optional fields set: %+v", req)
	}
	if len(req.EmailTags) != 0 || req.Destination.CcAddresses != nil || req.Destination.BccAddresses != nil {
		t.Errorf("unexpected tags or destinations: %+v", req)
	}
}

func TestSendRejectsInvalidTags(t *testing.T) {
	fake := newFakeSES(t)
	s := fake.newSender(t, Config{})
	cases := map[string]map[string]string{
		"NameSpace":   {"bad name": "v"},
		"ValueDot":    {"purpose": "verify.email"},
		"EmptyValue":  {"purpose": ""},
		"NameTooLong": {strings.Repeat("a", 257): "v"},
	}
	for name, tags := range cases {
		t.Run(name, func(t *testing.T) {
			msg := testMessage()
			msg.Tags = tags
			_, err := s.Send(context.Background(), msg)
			if omnimail.KindOf(err) != omnimail.KindInvalidMessage || !errors.Is(err, omnimail.ErrInvalidMessage) {
				t.Fatalf("err = %v, want invalid message", err)
			}
			var e *omnimail.Error
			if !errors.As(err, &e) || e.Provider != ProviderName || e.Field != "tags" {
				t.Errorf("error = %+v", e)
			}
		})
	}
	if n := len(fake.received()); n != 0 {
		t.Errorf("invalid tags reached SES (%d requests)", n)
	}
}

func TestSendRejectsNonASCIIAddresses(t *testing.T) {
	fake := newFakeSES(t)
	s := fake.newSender(t, Config{})
	cases := map[string]struct {
		mutate func(*omnimail.Message)
		field  string
	}{
		"From":      {func(m *omnimail.Message) { m.From.Email = "zoë@example.com" }, "from"},
		"ToLocal":   {func(m *omnimail.Message) { m.To[0].Email = "用户@example.org" }, "to[0]"},
		"CcDomain":  {func(m *omnimail.Message) { m.Cc[0].Email = "user@bücher.example" }, "cc[0]"},
		"BccDomain": {func(m *omnimail.Message) { m.Bcc[0].Email = "user@例子.广告" }, "bcc[0]"},
		"ReplyTo": {func(m *omnimail.Message) {
			m.ReplyTo = []omnimail.Address{{Email: "ok@example.com"}, {Email: "josé@example.com"}}
		}, "replyTo[1]"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			msg := testMessage()
			tc.mutate(msg)
			_, err := s.Send(context.Background(), msg)
			var e *omnimail.Error
			if !errors.As(err, &e) || e.Kind != omnimail.KindInvalidAddress || e.Provider != ProviderName || e.Field != tc.field {
				t.Fatalf("err = %v (%+v), want invalid address on %s", err, e, tc.field)
			}
			if !errors.Is(err, omnimail.ErrInvalidMessage) || omnimail.IsRetryable(err) {
				t.Errorf("err = %v, want non-retryable ErrInvalidMessage match", err)
			}
		})
	}
	if n := len(fake.received()); n != 0 {
		t.Errorf("non-ASCII addresses reached SES (%d requests)", n)
	}
	// Punycode domains are ASCII and accepted.
	msg := testMessage()
	msg.To[0].Email = "user@xn--bcher-kva.example"
	if _, err := s.Send(context.Background(), msg); err != nil {
		t.Errorf("punycode domain: %v", err)
	}
}

func TestSendDeadlineExceeded(t *testing.T) {
	fake := newFakeSES(t)
	s := fake.newSender(t, Config{})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := s.Send(ctx, testMessage())
	if !errors.Is(err, context.DeadlineExceeded) || omnimail.IsRetryable(err) {
		t.Fatalf("err = %v, want non-retryable deadline exceeded", err)
	}
	if n := len(fake.received()); n != 0 {
		t.Errorf("sent despite expired context (%d requests)", n)
	}
}

func TestSendRetryAfter(t *testing.T) {
	fake := newFakeSES(t)
	s := fake.newSender(t, Config{})
	fake.fail(fakeFailure{status: 429, errorType: "TooManyRequestsException", message: "Maximum sending rate exceeded.", retryAfter: "7"})
	_, err := s.Send(context.Background(), testMessage())
	if !errors.Is(err, omnimail.ErrThrottled) || !omnimail.IsRetryable(err) {
		t.Fatalf("err = %v, want retryable throttled", err)
	}
	if got := omnimail.RetryAfter(err); got != 7*time.Second {
		t.Errorf("RetryAfter = %v, want 7s", got)
	}
	var e *omnimail.Error
	if !errors.As(err, &e) || e.Code != "TooManyRequestsException" || e.Message != "Maximum sending rate exceeded." {
		t.Errorf("error = %+v", e)
	}
}

func TestNewConfig(t *testing.T) {
	// Keep the default credential chain away from the developer's files.
	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")

	t.Run("NoRegion", func(t *testing.T) {
		if _, err := New(Config{}); err == nil || !strings.Contains(err.Error(), "region is required") {
			t.Fatalf("err = %v, want region required", err)
		}
	})
	t.Run("Region", func(t *testing.T) {
		s, err := New(Config{Region: "eu-west-1", MaxAttempts: 2})
		if err != nil {
			t.Fatal(err)
		}
		if got := s.client.Options().Region; got != "eu-west-1" {
			t.Errorf("Region = %q", got)
		}
		if got := s.client.Options().RetryMaxAttempts; got != 2 {
			t.Errorf("RetryMaxAttempts = %d", got)
		}
	})
	t.Run("MissingProfile", func(t *testing.T) {
		if _, err := New(Config{Region: "us-east-1", Profile: "does-not-exist"}); err == nil {
			t.Fatal("want error for a missing shared config profile")
		}
	})
	t.Run("NegativeMaxAttempts", func(t *testing.T) {
		if _, err := New(Config{Region: "us-east-1", MaxAttempts: -1}); err == nil {
			t.Fatal("want error for negative MaxAttempts")
		}
	})
	t.Run("InvalidDefaultTags", func(t *testing.T) {
		_, err := New(Config{Region: "us-east-1", DefaultTags: map[string]string{"a": "b c"}})
		if omnimail.KindOf(err) != omnimail.KindInvalidMessage {
			t.Fatalf("err = %v, want invalid default tags", err)
		}
	})
	t.Run("AWSConfigAndEndpoint", func(t *testing.T) {
		fake := newFakeSES(t)
		awsCfg := aws.Config{
			Region:      "us-west-2",
			Credentials: credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "SECRET", ""),
		}
		s, err := New(Config{AWSConfig: &awsCfg, Region: "us-east-2", EndpointURL: fake.URL, MaxAttempts: 1})
		if err != nil {
			t.Fatal(err)
		}
		if got := s.client.Options().Region; got != "us-east-2" {
			t.Errorf("Region = %q, want Config.Region to override AWSConfig", got)
		}
		if awsCfg.Region != "us-west-2" {
			t.Errorf("caller's AWSConfig modified: %q", awsCfg.Region)
		}
		res, err := s.Send(context.Background(), testMessage())
		if err != nil {
			t.Fatalf("Send through EndpointURL: %v", err)
		}
		if res.MessageID == "" {
			t.Error("empty MessageID")
		}
	})
	t.Run("Client", func(t *testing.T) {
		fake := newFakeSES(t)
		c := fake.client()
		s, err := New(Config{Client: c})
		if err != nil {
			t.Fatal(err)
		}
		if s.client != c {
			t.Error("Config.Client not used")
		}
	})
}

// TestIntegrationMailboxSimulator sends through real SES to the mailbox
// simulator. It is skipped unless OMNIAWS_SES_INTEGRATION=1 and
// OMNIAWS_SES_FROM (a verified identity) are set; AWS credentials and
// region come from the default chain.
func TestIntegrationMailboxSimulator(t *testing.T) {
	if os.Getenv("OMNIAWS_SES_INTEGRATION") != "1" {
		t.Skip("set OMNIAWS_SES_INTEGRATION=1 and OMNIAWS_SES_FROM to send through SES")
	}
	from := os.Getenv("OMNIAWS_SES_FROM")
	if from == "" {
		t.Skip("OMNIAWS_SES_FROM (a verified SES identity) is not set")
	}
	s, err := New(Config{
		Region:               os.Getenv("OMNIAWS_SES_REGION"),
		ConfigurationSetName: os.Getenv("OMNIAWS_SES_CONFIGURATION_SET"),
		MaxAttempts:          1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := s.Send(ctx, &omnimail.Message{
		From:    omnimail.Address{Name: "omni-aws integration", Email: from},
		To:      []omnimail.Address{{Email: "success@simulator.amazonses.com"}},
		Subject: "omni-aws SES integration test",
		Text:    "Sent by the omni-aws SES adapter integration test.\n",
		HTML:    "<p>Sent by the omni-aws SES adapter integration test.</p>",
		Tags:    map[string]string{"purpose": "integration_test"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	t.Logf("SES MessageId %s", res.MessageID)
}
