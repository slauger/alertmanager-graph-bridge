package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIError is a structured error returned by the Microsoft Graph API.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	// RetryAfter is the delay requested by a 429 response, if any.
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("graph: API error %d (%s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("graph: API error %d", e.StatusCode)
}

// sendMailEnvelope is the JSON request body of the sendMail endpoint.
type sendMailEnvelope struct {
	Message         graphMessage `json:"message"`
	SaveToSentItems bool         `json:"saveToSentItems"`
}

type graphMessage struct {
	Subject       string           `json:"subject"`
	Body          graphBody        `json:"body"`
	ToRecipients  []graphRecipient `json:"toRecipients"`
	CcRecipients  []graphRecipient `json:"ccRecipients,omitempty"`
	BccRecipients []graphRecipient `json:"bccRecipients,omitempty"`
}

type graphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type graphRecipient struct {
	EmailAddress graphAddress `json:"emailAddress"`
}

type graphAddress struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

// graphErrorEnvelope mirrors the Graph API error response shape.
type graphErrorEnvelope struct {
	Error struct {
		Code       string          `json:"code"`
		Message    string          `json:"message"`
		InnerError json.RawMessage `json:"innerError"`
	} `json:"error"`
}

// SendMail sends a message through the Graph sendMail endpoint. A 202 response
// is treated as success. On HTTP 429 it performs up to MaxRetries bounded
// retries honouring the Retry-After header.
func (c *Client) SendMail(ctx context.Context, msg Outgoing) error {
	from := msg.From
	if from == "" {
		from = c.from
	}
	if from == "" {
		return errors.New("graph: no sender address configured")
	}

	payload, err := json.Marshal(buildEnvelope(msg))
	if err != nil {
		return fmt.Errorf("graph: marshaling request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/users/%s/sendMail", c.graphBase, url.PathEscape(from))

	for attempt := 0; ; attempt++ {
		sendErr := c.doSend(ctx, endpoint, payload)
		if sendErr == nil {
			return nil
		}

		var apiErr *APIError
		retryable := errors.As(sendErr, &apiErr) && isRetryable(apiErr.StatusCode)
		if !retryable || attempt >= c.maxRetries {
			return sendErr
		}

		delay := apiErr.RetryAfter
		if delay <= 0 {
			delay = time.Second
		}
		if delay > maxRetryDelay {
			delay = maxRetryDelay
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("graph: context cancelled before retry: %w", ctx.Err())
		}
	}
}

// doSend performs a single sendMail HTTP request.
func (c *Client) doSend(ctx context.Context, endpoint string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("graph: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("graph: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusAccepted {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return parseAPIError(resp)
}

// buildEnvelope converts an Outgoing message into the Graph request body.
func buildEnvelope(msg Outgoing) sendMailEnvelope {
	contentType := "Text"
	if msg.HTMLBody != "" {
		contentType = "HTML"
	}
	return sendMailEnvelope{
		Message: graphMessage{
			Subject:       msg.Subject,
			Body:          graphBody{ContentType: contentType, Content: msg.HTMLBody},
			ToRecipients:  toRecipients(msg.To),
			CcRecipients:  toRecipients(msg.Cc),
			BccRecipients: toRecipients(msg.Bcc),
		},
		SaveToSentItems: msg.SaveToSentItems,
	}
}

func toRecipients(addrs []string) []graphRecipient {
	if len(addrs) == 0 {
		return nil
	}
	out := make([]graphRecipient, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, graphRecipient{EmailAddress: graphAddress{Address: a}})
	}
	return out
}

// parseAPIError builds an *APIError from a non-2xx response.
func parseAPIError(resp *http.Response) *APIError {
	apiErr := &APIError{StatusCode: resp.StatusCode}

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env graphErrorEnvelope
	if json.Unmarshal(raw, &env) == nil {
		apiErr.Code = env.Error.Code
		apiErr.Message = env.Error.Message
	}
	if isRetryable(resp.StatusCode) {
		apiErr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	}
	return apiErr
}

// isRetryable reports whether a Microsoft Graph response status indicates a
// transient condition worth retrying: throttling (429) or a temporary service
// failure (503, 504).
func isRetryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// parseRetryAfter interprets a Retry-After header value, which may be either a
// number of seconds or an HTTP date.
func parseRetryAfter(v string) time.Duration {
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
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
