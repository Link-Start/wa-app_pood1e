package engine

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// fakeKey builds a deterministic 32-byte fake Curve25519 key (no real secrets).
func fakeKey(fill byte) []byte {
	return bytes.Repeat([]byte{fill}, sixSegmentKeyBytes)
}

func TestBuildSixSegment(t *testing.T) {
	identityPublic := fakeKey(0xA1)
	identityPrivate := fakeKey(0xA2)
	chatStaticPublic := fakeKey(0xB1)
	chatStaticPrivate := fakeKey(0xB2)
	accountIDBytes := bytes.Repeat([]byte{0x7F}, 21) // reference field 6 has ~21 id bytes

	ns := &NativeState{
		CC:    "62",
		Phone: "83129740676",
		Signal: nativeSignalState{
			Identity: nativeCurveKeyPair{Public: b64u(identityPublic), Private: b64u(identityPrivate)},
		},
		ChatStatic: nativeCurveKeyPair{Public: b64u(chatStaticPublic), Private: b64u(chatStaticPrivate)},
		Profile:    nativePhoneProfile{BackupTokenHex: hex.EncodeToString(accountIDBytes)},
	}

	segment, err := buildSixSegment(ns)
	if err != nil {
		t.Fatalf("buildSixSegment returned error: %v", err)
	}

	parts := strings.Split(segment, ",")
	if len(parts) != 6 {
		t.Fatalf("expected 6 comma-separated fields, got %d (%q)", len(parts), segment)
	}

	const wantPhone = "6283129740676"
	if parts[0] != wantPhone {
		t.Fatalf("field 1 = %q, want %q", parts[0], wantPhone)
	}

	// Fields 2–5 must be standard base64 of the exact 32-byte keys, in the
	// documented pairing order: identity public/private, then chatStatic
	// public/private.
	keyCases := []struct {
		index int
		label string
		want  []byte
	}{
		{1, "公钥", identityPublic},
		{2, "私钥", identityPrivate},
		{3, "消息公钥", chatStaticPublic},
		{4, "消息私钥", chatStaticPrivate},
	}
	for _, kc := range keyCases {
		raw, err := base64.StdEncoding.DecodeString(parts[kc.index])
		if err != nil {
			t.Fatalf("field %d (%s) is not standard base64: %v", kc.index+1, kc.label, err)
		}
		if len(raw) != sixSegmentKeyBytes {
			t.Fatalf("field %d (%s) decoded to %d bytes, want %d", kc.index+1, kc.label, len(raw), sixSegmentKeyBytes)
		}
		if !bytes.Equal(raw, kc.want) {
			t.Fatalf("field %d (%s) key bytes mismatch", kc.index+1, kc.label)
		}
	}

	// Field 6 is standard base64 of <phone-ascii> || <account-id bytes>.
	numberID, err := base64.StdEncoding.DecodeString(parts[5])
	if err != nil {
		t.Fatalf("field 6 (号码ID) is not standard base64: %v", err)
	}
	if !bytes.HasPrefix(numberID, []byte(wantPhone)) {
		t.Fatalf("field 6 (号码ID) does not begin with the phone digits")
	}
	if want := len(wantPhone) + len(accountIDBytes); len(numberID) != want {
		t.Fatalf("field 6 (号码ID) decoded to %d bytes, want %d", len(numberID), want)
	}
}

func TestBuildSixSegmentRejectsWrongLengthKey(t *testing.T) {
	ns := &NativeState{
		CC:    "62",
		Phone: "83129740676",
		Signal: nativeSignalState{
			Identity: nativeCurveKeyPair{Public: b64u([]byte{0x01, 0x02, 0x03}), Private: b64u(fakeKey(0xA2))},
		},
		ChatStatic: nativeCurveKeyPair{Public: b64u(fakeKey(0xB1)), Private: b64u(fakeKey(0xB2))},
		Profile:    nativePhoneProfile{BackupTokenHex: hex.EncodeToString(fakeKey(0x7F))},
	}
	if _, err := buildSixSegment(ns); err == nil {
		t.Fatal("expected error for a key that does not decode to 32 bytes")
	}
}
