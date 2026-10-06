package ses

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	"github.com/plexusone/omnimail"
	"github.com/plexusone/omnimail/providertest"
)

// sendEmailRequest is the subset of the SES v2 SendEmail JSON body the fake
// inspects.
type sendEmailRequest struct {
	FromEmailAddress               string `json:"FromEmailAddress"`
	FromEmailAddressIdentityArn    string `json:"FromEmailAddressIdentityArn"`
	FeedbackForwardingEmailAddress string `json:"FeedbackForwardingEmailAddress"`
	ConfigurationSetName           string `json:"ConfigurationSetName"`
	Destination                    struct {
		ToAddresses  []string `json:"ToAddresses"`
		CcAddresses  []string `json:"CcAddresses"`
		BccAddresses []string `json:"BccAddresses"`
	} `json:"Destination"`
	Content struct {
		Raw struct {
			Data string `json:"Data"`
		} `json:"Raw"`
	} `json:"Content"`
	EmailTags []struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
	} `json:"EmailTags"`
}

// fakeFailure is an SES error response.
type fakeFailure struct {
	status     int
	errorType  string
	message    string
	retryAfter string
}

// fakeSES is an httptest server that implements POST
// /v2/email/outbound-emails like SES v2.
type fakeSES struct {
	*httptest.Server
	t testing.TB

	mu       sync.Mutex
	requests []sendEmailRequest
	failures []fakeFailure
	seq      atomic.Int64
}

func newFakeSES(t *testing.T) *fakeSES {
	t.Helper()
	f := &fakeSES{t: t}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeSES) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v2/email/outbound-emails" {
		f.writeError(w, fakeFailure{status: http.StatusNotFound, errorType: "UnknownOperationException", message: r.Method + " " + r.URL.Path})
		return
	}
	if r.Header.Get("Authorization") == "" {
		f.writeError(w, fakeFailure{status: http.StatusForbidden, errorType: "MissingAuthenticationToken", message: "unsigned request"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.writeError(w, fakeFailure{status: http.StatusBadRequest, errorType: "SerializationException", message: err.Error()})
		return
	}
	var req sendEmailRequest
	if err := json.Unmarshal(body, &req); err != nil {
		f.writeError(w, fakeFailure{status: http.StatusBadRequest, errorType: "SerializationException", message: err.Error()})
		return
	}

	f.mu.Lock()
	if len(f.failures) > 0 {
		fail := f.failures[0]
		f.failures = f.failures[1:]
		f.mu.Unlock()
		f.writeError(w, fail)
		return
	}
	f.requests = append(f.requests, req)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"MessageId": fmt.Sprintf("0100018f-fake-%06d-000000", f.seq.Add(1)),
	}); err != nil {
		f.t.Errorf("fake SES: write response: %v", err)
	}
}

func (f *fakeSES) writeError(w http.ResponseWriter, fail fakeFailure) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Amzn-ErrorType", fail.errorType)
	if fail.retryAfter != "" {
		w.Header().Set("Retry-After", fail.retryAfter)
	}
	w.WriteHeader(fail.status)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": fail.message}); err != nil {
		f.t.Errorf("fake SES: write error response: %v", err)
	}
}

// fail queues an error response for the next request.
func (f *fakeSES) fail(ff fakeFailure) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, ff)
}

func (f *fakeSES) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
	f.failures = nil
}

func (f *fakeSES) received() []sendEmailRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sendEmailRequest(nil), f.requests...)
}

// client returns an SES v2 client for the fake with SDK retries disabled.
func (f *fakeSES) client() *sesv2.Client {
	return sesv2.New(sesv2.Options{
		BaseEndpoint:     aws.String(f.URL),
		Region:           "us-east-1",
		Credentials:      credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "SECRET", ""),
		RetryMaxAttempts: 1,
	})
}

// newSender returns a Sender wired to the fake.
func (f *fakeSES) newSender(t *testing.T, cfg Config) *Sender {
	t.Helper()
	cfg.Client = f.client()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// harness adapts the fake to providertest.Harness.
type harness struct {
	fake   *fakeSES
	sender *Sender
}

func (h *harness) Sender() omnimail.Sender { return h.sender }

func (h *harness) Reset() { h.fake.reset() }

func (h *harness) Captured(t testing.TB) []*omnimail.Message {
	t.Helper()
	reqs := h.fake.received()
	out := make([]*omnimail.Message, 0, len(reqs))
	for _, req := range reqs {
		out = append(out, decodeRequest(t, req))
	}
	return out
}

func (h *harness) InjectFailure(kind omnimail.Kind) bool {
	switch kind {
	case omnimail.KindInvalidAddress:
		h.fake.fail(fakeFailure{status: 400, errorType: "BadRequestException", message: "Illegal address: missing final '@domain'"})
	case omnimail.KindRejected:
		h.fake.fail(fakeFailure{status: 400, errorType: "MessageRejected", message: "Email address is not verified."})
	case omnimail.KindThrottled:
		h.fake.fail(fakeFailure{status: 429, errorType: "TooManyRequestsException", message: "Maximum sending rate exceeded.", retryAfter: "2"})
	case omnimail.KindTransient:
		h.fake.fail(fakeFailure{status: 500, errorType: "InternalServiceErrorException", message: "internal error"})
	case omnimail.KindAuth:
		h.fake.fail(fakeFailure{status: 403, errorType: "AccessDeniedException", message: "not authorized to perform ses:SendEmail"})
	default:
		return false
	}
	return true
}

// decodeRequest turns a captured SendEmail body back into a message.
func decodeRequest(t testing.TB, req sendEmailRequest) *omnimail.Message {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(req.Content.Raw.Data)
	if err != nil {
		t.Fatalf("decode Content.Raw.Data: %v", err)
	}
	var rcpts []string
	rcpts = append(rcpts, req.Destination.ToAddresses...)
	rcpts = append(rcpts, req.Destination.CcAddresses...)
	rcpts = append(rcpts, req.Destination.BccAddresses...)
	msg, err := providertest.DecodeMIME(raw, rcpts)
	if err != nil {
		t.Fatalf("DecodeMIME: %v", err)
	}
	for _, tag := range req.EmailTags {
		if msg.Tags == nil {
			msg.Tags = map[string]string{}
		}
		msg.Tags[tag.Name] = tag.Value
	}
	return msg
}
