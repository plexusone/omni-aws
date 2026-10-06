# OmniMail: Amazon SES

An [OmniMail](https://github.com/plexusone/omnimail) sender for transactional
email through the Amazon SES v2 API.

```go
import ses "github.com/plexusone/omni-aws/omnimail"
```

## Installation

```bash
go get github.com/plexusone/omni-aws
```

## Features

- **`omnimail.Sender`**: drop-in replacement for the SMTP, log and memory senders
- **Raw MIME delivery**: messages are built with `omnimail.BuildMIME` and sent with SES v2 `SendEmail` (`Content.Raw`), so custom headers, Reply-To, Unicode subjects and display names, and text/HTML bodies arrive exactly as built
- **Bcc kept out of headers**: To, Cc and Bcc are passed as the SES destination
- **Message tags**: `Message.Tags` and `Config.DefaultTags` become SES message tags for event publishing
- **Configuration sets** and **sending authorization** (`FromEmailAddressIdentityArn`)
- **Classified errors**: SES error codes mapped to `omnimail` kinds, with `Retry-After` when SES sends one
- **Conformance tested**: passes the `omnimail/providertest` suite against a fake SES endpoint

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/plexusone/omnimail"
    ses "github.com/plexusone/omni-aws/omnimail"
)

func main() {
    sender, err := ses.New(ses.Config{
        Region:               "us-east-1",
        ConfigurationSetName: "transactional", // optional
        DefaultTags:          map[string]string{"app": "web"},
    })
    if err != nil {
        log.Fatal(err)
    }

    res, err := sender.Send(context.Background(), &omnimail.Message{
        From:    omnimail.Address{Name: "Example", Email: "no-reply@example.com"},
        To:      []omnimail.Address{{Email: "user@example.org"}},
        Subject: "Verify your email",
        Text:    "Open this link to verify your address: https://example.com/verify?t=...",
        HTML:    `<p><a href="https://example.com/verify?t=...">Verify your address</a></p>`,
        Tags:    map[string]string{"purpose": "verify_email"},
    })
    if err != nil {
        if omnimail.IsRetryable(err) {
            log.Printf("temporary failure, retry after %v: %v", omnimail.RetryAfter(err), err)
            return
        }
        log.Fatal(err)
    }
    log.Printf("sent %s via %s", res.MessageID, res.Provider) // SES MessageId, "ses"
}
```

## Configuration

| Field | Description |
|-------|-------------|
| `Client` | Preconfigured `*sesv2.Client`. When set, `AWSConfig`, `Region`, `Profile`, `EndpointURL` and `MaxAttempts` are ignored |
| `AWSConfig` | Preloaded `*aws.Config`. When nil, the default credential chain is loaded |
| `Region` | SES region of your identity. Overrides the region from `AWSConfig` or the environment |
| `Profile` | Shared config profile used when `AWSConfig` is nil |
| `EndpointURL` | Custom SES endpoint (tests, local emulators) |
| `MaxAttempts` | SDK attempts per send including the first; 0 keeps the SDK default (3). Use 1 to leave retries to your application |
| `ConfigurationSetName` | Configuration set applied to every message |
| `FromEmailAddressIdentityArn` | Identity ARN for cross-account sending authorization |
| `FeedbackForwardingEmailAddress` | Address that receives forwarded bounces and complaints |
| `DefaultTags` | Message tags added to every message; message tags with the same name win |

Credentials come from the standard AWS chain: environment variables, shared
config/credentials files, SSO, IAM Roles for Service Accounts on EKS, ECS task
roles and EC2 instance profiles.

## Message Mapping

| `omnimail.Message` | SES v2 `SendEmail` |
|--------------------|--------------------|
| From, To, Cc, Reply-To, Subject, Text, HTML, Headers | `Content.Raw.Data` (RFC 5322 from `omnimail.BuildMIME`) |
| `From.Email` | `FromEmailAddress` |
| To / Cc / Bcc | `Destination.ToAddresses` / `CcAddresses` / `BccAddresses` |
| Tags (merged over `DefaultTags`) | `EmailTags` |
| IdempotencyKey | not transmitted (see below) |

The returned `SendResult.MessageID` is the SES `MessageId`, which is also what
SES reports in bounce, complaint and delivery events.

### Tags

SES message tag names and values must be 1-256 characters of `A-Z`, `a-z`,
`0-9`, `_` and `-`. Tags that break this rule are rejected with
`KindInvalidMessage` (field `tags`) before SES is called; they are not
silently rewritten. Invalid `DefaultTags` make `New` fail.

### Idempotency

SES has no idempotency key. `Message.IdempotencyKey` is accepted but not
sent, and a retried `Send` can deliver the message twice. If duplicates
matter, record the key (or the returned `MessageID`) in your own store before
retrying.

### Internationalized addresses

SES does not support SMTPUTF8, so every address must be 7-bit ASCII. `Send`
returns `KindInvalidAddress` for an address with non-ASCII characters without
calling SES. Encode internationalized domains as Punycode
(`user@xn--bcher-kva.example`) before sending. Non-ASCII display names and
subjects are fine; they are RFC 2047 encoded.

## Error Mapping

Errors are `*omnimail.Error` with `Provider` `"ses"`, `Code` set to the SES
error code, `Message` set to the SES message, and the SDK error in `Err`.
`ses.Classify` exposes the mapping.

| SES signal | `omnimail.Kind` | Retryable |
|------------|-----------------|-----------|
| `TooManyRequestsException`, `LimitExceededException`, `ThrottlingException`, HTTP 429 | `Throttled` (with `RetryAfter` from a `Retry-After` header) | yes |
| `MessageRejected`, `MailFromDomainNotVerifiedException`, `AccountSuspendedException`, `SendingPausedException`, `NotFoundException` | `Rejected` | no |
| `BadRequestException` naming an address, recipient, domain or mailbox | `InvalidAddress` | no |
| other `BadRequestException` | `InvalidMessage` | no |
| `AccessDeniedException`, `UnrecognizedClientException`, signature and expired-token errors, HTTP 401/403 | `Auth` | no |
| `InternalServiceErrorException`, HTTP 5xx, network errors | `Transient` | yes |
| anything else | `Unknown` | no |

A canceled or expired context returns the context error, which is not
retryable. In the SES sandbox, sending to an unverified recipient returns
`MessageRejected` and is classified `Rejected`.

## IAM Policy

Grant send permission scoped to your verified identity (and configuration
set, if you use one):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["ses:SendEmail", "ses:SendRawEmail"],
      "Resource": [
        "arn:aws:ses:us-east-1:123456789012:identity/example.com",
        "arn:aws:ses:us-east-1:123456789012:configuration-set/transactional"
      ]
    }
  ]
}
```

SES v2 `SendEmail` is authorized as `ses:SendEmail`; `ses:SendRawEmail` is
included for tools that still use the v1 raw API. Add
`"Condition": {"StringEquals": {"ses:FromAddress": "no-reply@example.com"}}`
to restrict the From address.

## SES Setup

- **Verify a domain identity** (not just an email address) for the From
  domain, in the same region as the sender.
- **DKIM**: enable Easy DKIM on the domain identity and publish the three
  CNAME records SES provides.
- **SPF**: configure a custom MAIL FROM domain (for example
  `mail.example.com`) with its MX record and an SPF TXT record
  `v=spf1 include:amazonses.com ~all`, so SPF aligns with your domain.
- **DMARC**: publish `_dmarc.example.com` (start with `p=none` and a `rua`
  reporting address, then tighten to `quarantine` or `reject`).
- **Leave the sandbox**: new accounts can only send to verified addresses and
  have low quotas. Request production access in the SES console before
  sending to real users.
- **Handle bounces and complaints**: attach an event destination (SNS,
  EventBridge, Firehose) to the configuration set and feed it into your
  suppression logic; message tags appear on these events.

## Testing

Unit tests run the `omnimail/providertest` conformance suite against an
`httptest` fake of `POST /v2/email/outbound-emails`; no AWS access is needed:

```bash
go test ./omnimail/
```

An opt-in integration test sends to the SES mailbox simulator
(`success@simulator.amazonses.com`) using the default AWS credential chain:

```bash
OMNIAWS_SES_INTEGRATION=1 \
OMNIAWS_SES_FROM=no-reply@example.com \
OMNIAWS_SES_REGION=us-east-1 \
go test -run TestIntegrationMailboxSimulator -v ./omnimail/
```

`OMNIAWS_SES_FROM` must be a verified identity. `OMNIAWS_SES_CONFIGURATION_SET`
is optional.
