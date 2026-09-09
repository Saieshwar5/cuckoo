// Package push sends notifications to the phones of people who are not
// looking.
//
// The chain is longer than it seems: the hub asks Expo, Expo asks Google, and
// Google wakes the phone. Only Google may wake an Android phone, so there is
// no shorter path — and Expo is what saves us implementing Google's protocol
// and holding its credentials.
//
// What matters here is restraint. A notification is the most intrusive thing
// this software can do: it interrupts somebody who did not ask, on a device in
// their pocket, possibly at night. Every rule about when *not* to send is in
// Notifier; this file only knows how to send.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// endpoint is Expo's send API. Overridden in tests.
const endpoint = "https://exp.host/--/api/v2/push/send"

// batchSize is what Expo accepts in one request.
const batchSize = 100

// Message is one notification, addressed to one device.
type Message struct {
	// To is an Expo token, "ExponentPushToken[…]".
	To    string `json:"to"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
	// Data travels with the notification and comes back when it is tapped:
	// which conversation to open.
	Data map[string]string `json:"data,omitempty"`
	// Sound is "default" or empty for silent.
	Sound string `json:"sound,omitempty"`
	// ChannelID names the Android channel, which decides how it behaves —
	// importance, sound, whether it appears on a lock screen.
	ChannelID string `json:"channelId,omitempty"`
	// CollapseKey folds several notifications for one chat into the latest,
	// so ten messages are one line rather than ten.
	CollapseKey string `json:"collapseKey,omitempty"`
}

// Result is what became of one message.
type Result struct {
	Token string
	// Gone means the app is no longer on that device, so the token is wrong
	// and must be forgotten. Anything else is a failure to retry or ignore.
	Gone  bool
	Error string
}

// Sender delivers notifications. Swapped in tests.
type Sender interface {
	Send(ctx context.Context, messages []Message) ([]Result, error)
}

// ExpoSender is the real one.
type ExpoSender struct {
	client   *http.Client
	endpoint string
	// accessToken is optional; Expo requires it only for accounts that have
	// turned on enhanced security.
	accessToken string
}

type Option func(*ExpoSender)

func WithEndpoint(url string) Option       { return func(s *ExpoSender) { s.endpoint = url } }
func WithAccessToken(t string) Option      { return func(s *ExpoSender) { s.accessToken = t } }
func WithHTTPClient(c *http.Client) Option { return func(s *ExpoSender) { s.client = c } }

func NewExpoSender(opts ...Option) *ExpoSender {
	s := &ExpoSender{
		client:   &http.Client{Timeout: 20 * time.Second},
		endpoint: endpoint,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ticket is Expo's answer for one message: accepted, or refused with a reason.
type ticket struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

// Send delivers a batch and reports what became of each message.
//
// A failure to reach Expo at all is an error; a message Expo refuses is a
// Result, because one dead token among a hundred is not a failure of the
// batch.
func (s *ExpoSender) Send(ctx context.Context, messages []Message) ([]Result, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	results := make([]Result, 0, len(messages))
	for start := 0; start < len(messages); start += batchSize {
		end := min(start+batchSize, len(messages))
		batch := messages[start:end]

		out, err := s.sendBatch(ctx, batch)
		if err != nil {
			return results, err
		}
		results = append(results, out...)
	}
	return results, nil
}

func (s *ExpoSender) sendBatch(ctx context.Context, batch []Message) ([]Result, error) {
	body, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("encode notifications: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.accessToken)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send notifications: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read reply: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("expo answered %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var envelope struct {
		Data []ticket `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode reply: %w", err)
	}

	results := make([]Result, 0, len(batch))
	for i, m := range batch {
		r := Result{Token: m.To}
		if i < len(envelope.Data) {
			t := envelope.Data[i]
			if t.Status != "ok" {
				r.Error = t.Message
				// The app is gone from that device. Keeping the token means
				// sending there for as long as the account exists.
				r.Gone = t.Details.Error == "DeviceNotRegistered"
			}
		}
		results = append(results, r)
	}
	return results, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
