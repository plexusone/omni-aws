// Package ses provides an [omnimail.Sender] that delivers transactional
// email through the Amazon SES v2 API.
//
// Messages are assembled with [omnimail.BuildMIME] and sent as raw MIME
// (SendEmail with Content.Raw), so custom headers, Reply-To, Unicode
// subjects and display names, and multipart text/HTML bodies arrive exactly
// as built. To, Cc and Bcc recipients are passed as the SES destination;
// Bcc never appears in a header.
//
//	s, err := ses.New(ses.Config{
//	    Region:               "us-east-1",
//	    ConfigurationSetName: "transactional",
//	})
//	if err != nil {
//	    return err
//	}
//	res, err := s.Send(ctx, msg)
//
// Message tags become SES message tags (EmailTags), which flow to event
// destinations configured on the configuration set. SES has no idempotency
// mechanism, so [omnimail.Message.IdempotencyKey] is accepted but not
// transmitted; deduplicate before calling Send if a retry must not deliver
// twice.
//
// SES does not support the SMTPUTF8 extension: every address must be 7-bit
// ASCII. Send rejects addresses with non-ASCII characters as
// [omnimail.KindInvalidAddress] without contacting SES. Encode
// internationalized domains as Punycode (xn--...) before sending.
//
// Failures are returned as [*omnimail.Error] with Provider "ses" and Code
// set to the SES error code; see [Classify] for the mapping.
package ses

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/plexusone/omnimail"
)

// ProviderName is reported in SendResult.Provider and Error.Provider.
const ProviderName = "ses"

// Config configures a Sender.
type Config struct {
	// Client is a preconfigured SES v2 client. When set, Region, Profile,
	// EndpointURL, AWSConfig and MaxAttempts are ignored.
	Client *sesv2.Client

	// AWSConfig is a preloaded AWS SDK configuration used to build the
	// client. When nil, the default credential chain is loaded with Region
	// and Profile.
	AWSConfig *aws.Config

	// Region is the AWS region of the SES identity (for example
	// "us-east-1"). It overrides the region in AWSConfig and defaults to the
	// region from the environment or shared config.
	Region string

	// Profile is the shared config profile used when AWSConfig is nil.
	Profile string

	// EndpointURL overrides the SES endpoint (for tests or a local
	// emulator).
	EndpointURL string

	// MaxAttempts is the SDK's maximum number of attempts per send,
	// including the first. Zero keeps the SDK default (3). Set 1 to disable
	// SDK retries and let the application's retry policy, driven by
	// omnimail.IsRetryable, decide.
	MaxAttempts int

	// ConfigurationSetName is the SES configuration set applied to every
	// message (event publishing, dedicated IPs, suppression overrides).
	// Optional.
	ConfigurationSetName string

	// FromEmailAddressIdentityArn is the ARN of the identity authorized to
	// send for the From address, for cross-account sending authorization.
	// Optional.
	FromEmailAddressIdentityArn string

	// FeedbackForwardingEmailAddress receives bounce and complaint
	// notifications forwarded by SES. Optional; defaults to the From
	// address.
	FeedbackForwardingEmailAddress string

	// DefaultTags are SES message tags added to every message. Tags set on
	// the message override defaults with the same name.
	DefaultTags map[string]string
}

// Sender delivers messages through Amazon SES v2. It is safe for concurrent
// use.
type Sender struct {
	client                 *sesv2.Client
	configurationSet       string
	fromIdentityArn        string
	feedbackForwardingAddr string
	defaultTags            map[string]string
}

var _ omnimail.Sender = (*Sender)(nil)

// New returns a Sender for cfg. It loads AWS configuration when cfg.Client
// is nil but does not contact SES.
func New(cfg Config) (*Sender, error) {
	return NewWithContext(context.Background(), cfg)
}

// NewWithContext is New with a context for loading the AWS configuration.
func NewWithContext(ctx context.Context, cfg Config) (*Sender, error) {
	if err := validateTags("defaultTags", cfg.DefaultTags); err != nil {
		return nil, err
	}
	if cfg.MaxAttempts < 0 {
		return nil, fmt.Errorf("ses: MaxAttempts must not be negative, got %d", cfg.MaxAttempts)
	}
	client := cfg.Client
	if client == nil {
		var err error
		if client, err = newClient(ctx, cfg); err != nil {
			return nil, err
		}
	}
	return &Sender{
		client:                 client,
		configurationSet:       cfg.ConfigurationSetName,
		fromIdentityArn:        cfg.FromEmailAddressIdentityArn,
		feedbackForwardingAddr: cfg.FeedbackForwardingEmailAddress,
		defaultTags:            maps.Clone(cfg.DefaultTags),
	}, nil
}

func newClient(ctx context.Context, cfg Config) (*sesv2.Client, error) {
	var awsCfg aws.Config
	if cfg.AWSConfig != nil {
		awsCfg = cfg.AWSConfig.Copy()
	} else {
		var opts []func(*config.LoadOptions) error
		if cfg.Region != "" {
			opts = append(opts, config.WithRegion(cfg.Region))
		}
		if cfg.Profile != "" {
			opts = append(opts, config.WithSharedConfigProfile(cfg.Profile))
		}
		loaded, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("ses: load AWS config: %w", err)
		}
		awsCfg = loaded
	}
	if cfg.Region != "" {
		awsCfg.Region = cfg.Region
	}
	if awsCfg.Region == "" {
		return nil, errors.New("ses: region is required (set Config.Region, AWS_REGION or a profile region)")
	}
	return sesv2.NewFromConfig(awsCfg, func(o *sesv2.Options) {
		if cfg.EndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.EndpointURL)
		}
		if cfg.MaxAttempts > 0 {
			o.RetryMaxAttempts = cfg.MaxAttempts
		}
	}), nil
}

// Send validates msg, builds it as raw MIME and sends it with SES v2
// SendEmail. It does not modify msg.
func (s *Sender) Send(ctx context.Context, msg *omnimail.Message) (*omnimail.SendResult, error) {
	if err := omnimail.ValidateMessage(msg); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkASCIIAddresses(msg); err != nil {
		return nil, err
	}
	tags, err := s.messageTags(msg.Tags)
	if err != nil {
		return nil, err
	}
	raw, err := omnimail.BuildMIME(msg, omnimail.BuildOptions{})
	if err != nil {
		return nil, err
	}

	in := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(msg.From.Email),
		Destination: &types.Destination{
			ToAddresses:  emails(msg.To),
			CcAddresses:  emails(msg.Cc),
			BccAddresses: emails(msg.Bcc),
		},
		Content:   &types.EmailContent{Raw: &types.RawMessage{Data: raw.Data}},
		EmailTags: tags,
	}
	if s.configurationSet != "" {
		in.ConfigurationSetName = aws.String(s.configurationSet)
	}
	if s.fromIdentityArn != "" {
		in.FromEmailAddressIdentityArn = aws.String(s.fromIdentityArn)
	}
	if s.feedbackForwardingAddr != "" {
		in.FeedbackForwardingEmailAddress = aws.String(s.feedbackForwardingAddr)
	}

	out, err := s.client.SendEmail(ctx, in)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, Classify(err)
	}
	id := aws.ToString(out.MessageId)
	if id == "" {
		id = raw.MessageID
	}
	return &omnimail.SendResult{MessageID: id, Provider: ProviderName}, nil
}

func emails(addrs []omnimail.Address) []string {
	if len(addrs) == 0 {
		return nil
	}
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = a.Email
	}
	return out
}

// checkASCIIAddresses rejects internationalized addresses, which SES cannot
// deliver because it does not support SMTPUTF8.
func checkASCIIAddresses(msg *omnimail.Message) error {
	lists := []struct {
		field string
		addrs []omnimail.Address
	}{
		{"from", []omnimail.Address{msg.From}},
		{"to", msg.To}, {"cc", msg.Cc}, {"bcc", msg.Bcc}, {"replyTo", msg.ReplyTo},
	}
	for _, l := range lists {
		for i, a := range l.addrs {
			if a.IsASCII() {
				continue
			}
			field := l.field
			if l.field != "from" {
				field = fmt.Sprintf("%s[%d]", l.field, i)
			}
			return &omnimail.Error{
				Kind:     omnimail.KindInvalidAddress,
				Provider: ProviderName,
				Field:    field,
				Message:  fmt.Sprintf("address %q is not ASCII; SES does not support SMTPUTF8 (encode the domain as Punycode)", a.Email),
			}
		}
	}
	return nil
}

// tagPattern is the SES message tag name and value character set.
var tagPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

// validateTags checks tags against the SES message tag rules: names and
// values of 1-256 ASCII letters, digits, underscores and dashes.
func validateTags(field string, tags map[string]string) error {
	for _, name := range slices.Sorted(maps.Keys(tags)) {
		if !tagPattern.MatchString(name) {
			return &omnimail.Error{
				Kind:     omnimail.KindInvalidMessage,
				Provider: ProviderName,
				Field:    field,
				Message:  fmt.Sprintf("tag name %q must be 1-256 characters of A-Z, a-z, 0-9, _ or -", name),
			}
		}
		if v := tags[name]; !tagPattern.MatchString(v) {
			return &omnimail.Error{
				Kind:     omnimail.KindInvalidMessage,
				Provider: ProviderName,
				Field:    field,
				Message:  fmt.Sprintf("tag %q value %q must be 1-256 characters of A-Z, a-z, 0-9, _ or -", name, v),
			}
		}
	}
	return nil
}

// messageTags merges the default tags with the message tags (message wins)
// and returns them sorted by name.
func (s *Sender) messageTags(msgTags map[string]string) ([]types.MessageTag, error) {
	if err := validateTags("tags", msgTags); err != nil {
		return nil, err
	}
	if len(s.defaultTags) == 0 && len(msgTags) == 0 {
		return nil, nil
	}
	merged := maps.Clone(s.defaultTags)
	if merged == nil {
		merged = make(map[string]string, len(msgTags))
	}
	maps.Copy(merged, msgTags)
	out := make([]types.MessageTag, 0, len(merged))
	for _, name := range slices.Sorted(maps.Keys(merged)) {
		out = append(out, types.MessageTag{Name: aws.String(name), Value: aws.String(merged[name])})
	}
	return out, nil
}
