package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestBuildMarkdownKeepsStatementSections(t *testing.T) {
	input := StatementRequest{CreatorID: "c-17", Period: "2026-09", Updates: []string{"tier changed"}, ContentJobs: []string{"clip-4 processed"}}
	got := buildMarkdown(input, UsageData{Points: []UsagePoint{{Timestamp: "2026-09-01", Amount: 12.5}}})
	want := []string{"Creator: c-17", "2026-09-01: 12.50", "tier changed", "clip-4 processed"}
	for _, part := range want {
		if !contains(got, part) {
			t.Fatalf("statement missing %q: %s", part, got)
		}
	}
}

func TestVerifyWebhook(t *testing.T) {
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte("statement.ready"))
	if !verifyWebhook("statement.ready", hex.EncodeToString(mac.Sum(nil)), "secret") {
		t.Fatal("signature should verify")
	}
}

func contains(s, part string) bool { return len(s) >= len(part) && stringIndex(s, part) >= 0 }
func stringIndex(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
