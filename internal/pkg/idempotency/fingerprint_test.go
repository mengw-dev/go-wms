package idempotency

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestFingerprintEncodingContract(t *testing.T) {
	f := Fingerprint("42", "5", "A\x1fB", "")
	const want = "v2:S9s0jxKtNW10KNO1g2LyeHC8fd8rl5QMNgJivxLFX3E"
	if f.String() != want {
		t.Fatalf("fingerprint=%s want=%s", f.String(), want)
	}
	if len(f.String()) != 46 || !f.Matches(want) {
		t.Fatal("v2 must fit VARCHAR(64) and replay itself")
	}
	// The golden value locks byte lengths, field count, version and byte order.
	if Fingerprint("42", "5", "A\x1fB", "").String() != want {
		t.Fatal("fingerprint must be deterministic")
	}
}

func TestFingerprintFieldBoundaries(t *testing.T) {
	// Every tuple below is distinct, including tuples that shared a legacy encoding.
	tuples := [][]string{nil, {""}, {"", ""}, {"a", "b"}, {"a\x1fb"},
		{"a\x1fb", "c"}, {"a", "b\x1fc"}, {"a\x1f", "b"}, {"a", "\x1fb"},
		{"a", ""}, {"", "a"}, {"ab", "c"}, {"a", "bc"}, {"仓库", "批次"},
		{"仓", "库批次"}, {"a\x00b", "c"}, {"a", "\x00bc"}, {"\xff"}, {"\xfe"}, {"\ufffd"}}
	seen := make(map[string]bool)
	for _, parts := range tuples {
		hash := Fingerprint(parts...).String()
		if seen[hash] {
			t.Fatalf("ambiguous tuple: %q", parts)
		}
		seen[hash] = true
	}
}

func TestFingerprintLegacyCompatibility(t *testing.T) {
	for _, parts := range [][]string{{"12", "34", "5", "0", "B1"}, {"12", "34", "5"},
		{"12"}, {"12", "picker"}, {"12", "5", "a01", "b1", "pda"}, {""}} {
		legacy := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
		f := Fingerprint(parts...)
		if !f.Matches(fmt.Sprintf("%x", legacy)) {
			t.Fatalf("safe legacy replay rejected: %q", parts)
		}
		changed := append(append([]string{}, parts...), "changed")
		if Fingerprint(changed...).Matches(fmt.Sprintf("%x", legacy)) {
			t.Fatalf("changed request matched: %q", parts)
		}
	}
	// Legacy hashes cannot prove boundaries when user fields contain the delimiter.
	left, right := []string{"a\x1fb", "c"}, []string{"a", "b\x1fc"}
	legacy := sha256.Sum256([]byte(strings.Join(left, "\x1f")))
	stored := fmt.Sprintf("%x", legacy)
	if Fingerprint(left...).Matches(stored) || Fingerprint(right...).Matches(stored) {
		t.Fatal("ambiguous legacy replay accepted")
	}
	if Fingerprint(left...).Matches(Fingerprint(right...).String()) {
		t.Fatal("distinct v2 requests matched")
	}
	empty := sha256.Sum256(nil)
	if Fingerprint().Matches(fmt.Sprintf("%x", empty)) {
		t.Fatal("empty legacy tuple accepted")
	}
	for _, invalid := range []string{"", "v3:unknown", "v2:broken", strings.Repeat("0", 64)} {
		if Fingerprint("12").Matches(invalid) {
			t.Fatalf("invalid hash matched: %q", invalid)
		}
	}
	if (RequestFingerprint{}).Matches("") {
		t.Fatal("uninitialized fingerprint matched")
	}
}
