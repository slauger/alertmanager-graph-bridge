package graph

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBuildEnvelope(t *testing.T) {
	t.Run("html body", func(t *testing.T) {
		env := buildEnvelope(Outgoing{
			To:              []string{"a@example.com", "b@example.com"},
			Cc:              []string{"c@example.com"},
			Bcc:             []string{"d@example.com"},
			Subject:         "Subject",
			HTMLBody:        "<p>body</p>",
			SaveToSentItems: true,
		})
		if env.Message.Body.ContentType != "HTML" {
			t.Errorf("ContentType = %q, want HTML", env.Message.Body.ContentType)
		}
		if env.Message.Body.Content != "<p>body</p>" {
			t.Errorf("Content = %q", env.Message.Body.Content)
		}
		if len(env.Message.ToRecipients) != 2 {
			t.Errorf("ToRecipients = %d, want 2", len(env.Message.ToRecipients))
		}
		if len(env.Message.CcRecipients) != 1 || len(env.Message.BccRecipients) != 1 {
			t.Errorf("cc/bcc recipients not mapped: %+v", env.Message)
		}
		if !env.SaveToSentItems {
			t.Error("SaveToSentItems = false, want true")
		}
	})

	t.Run("empty body falls back to text", func(t *testing.T) {
		env := buildEnvelope(Outgoing{Subject: "Subject", To: []string{"a@example.com"}})
		if env.Message.Body.ContentType != "Text" {
			t.Errorf("ContentType = %q, want Text", env.Message.Body.ContentType)
		}
		if env.Message.CcRecipients != nil {
			t.Errorf("CcRecipients = %v, want nil", env.Message.CcRecipients)
		}
	})
}

func TestParseRetryAfter(t *testing.T) {
	future := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	past := time.Now().Add(-2 * time.Minute).UTC().Format(http.TimeFormat)

	tests := []struct {
		name     string
		header   string
		wantPos  bool
		wantZero bool
	}{
		{name: "empty", header: "", wantZero: true},
		{name: "seconds", header: "5", wantPos: true},
		{name: "zero seconds", header: "0", wantZero: true},
		{name: "negative seconds", header: "-3", wantZero: true},
		{name: "garbage", header: "soon", wantZero: true},
		{name: "future http date", header: future, wantPos: true},
		{name: "past http date", header: past, wantZero: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRetryAfter(tt.header)
			if tt.wantZero && got != 0 {
				t.Errorf("parseRetryAfter(%q) = %v, want 0", tt.header, got)
			}
			if tt.wantPos && got <= 0 {
				t.Errorf("parseRetryAfter(%q) = %v, want > 0", tt.header, got)
			}
		})
	}
}

func TestAPIErrorMessage(t *testing.T) {
	withCode := &APIError{StatusCode: 400, Code: "ErrorInvalidRecipients", Message: "bad"}
	if msg := withCode.Error(); !strings.Contains(msg, "400") || !strings.Contains(msg, "ErrorInvalidRecipients") {
		t.Errorf("Error() = %q", msg)
	}

	bare := &APIError{StatusCode: 503}
	if msg := bare.Error(); !strings.Contains(msg, "503") {
		t.Errorf("Error() = %q", msg)
	}
}
