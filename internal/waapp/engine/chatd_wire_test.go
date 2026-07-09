package engine

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/byte-v-forge/wa-app/internal/waapp/wacore"
)

// fakeLoginIdentity is a deterministic, non-secret login identity for wire tests.
func fakeLoginIdentity() loginIdentity {
	return loginIdentity{jid: "15551230000@s.whatsapp.net", username: 15551230000}
}

// buildTestLoginPayload exercises the real long-connection payload path
// (longConnectionLoginPayloadBuilder -> loginPayload -> buildLoginPayload) with a fake
// identity and empty (default) device state, so no real secrets are involved.
func buildTestLoginPayload(t *testing.T, continuity wacore.LoginContinuity) []byte {
	t.Helper()
	builder := longConnectionLoginPayloadBuilder(continuity)
	return builder(fakeLoginIdentity(), NativeState{}, "2.26.26.72")
}

func loginPayloadField(t *testing.T, payload []byte, fieldNo int) pbValue {
	t.Helper()
	fields, err := parsePBFields(payload)
	if err != nil {
		t.Fatalf("parse login payload: %v", err)
	}
	values := fields[fieldNo]
	if len(values) != 1 {
		t.Fatalf("expected exactly one field %d, got %d", fieldNo, len(values))
	}
	return values[0]
}

// TestLoginPayloadSessionIDIsFixed32 asserts field 9 (sessionId) is encoded as sfixed32:
// tag byte (9<<3)|5 = 0x4d followed by exactly 4 little-endian bytes.
func TestLoginPayloadSessionIDIsFixed32(t *testing.T) {
	const sessionID uint32 = 0x11223344
	payload := buildTestLoginPayload(t, wacore.LoginContinuity{SessionID: sessionID, ConnectReason: wacore.WAConnectReasonUserActivated})

	field9 := loginPayloadField(t, payload, 9)
	if field9.wireType != 5 {
		t.Fatalf("field 9 wiretype = %d, want 5 (fixed32)", field9.wireType)
	}
	if len(field9.bytes) != 4 {
		t.Fatalf("field 9 payload length = %d, want 4", len(field9.bytes))
	}
	if got := binary.LittleEndian.Uint32(field9.bytes); got != sessionID {
		t.Fatalf("field 9 decoded value = %#x, want %#x", got, sessionID)
	}

	// Exact on-the-wire encoding: 0x4d tag then 4 LE bytes of the session id.
	wantTagAndValue := []byte{0x4d, 0x44, 0x33, 0x22, 0x11}
	if !bytes.Contains(payload, wantTagAndValue) {
		t.Fatalf("login payload does not contain sfixed32 field-9 encoding %x", wantTagAndValue)
	}
}

// TestLoginPayloadSessionIDStability asserts the same threaded session id yields identical
// field-9 bytes across builds (so reconnects look like the same session), while a different
// session id yields different bytes.
func TestLoginPayloadSessionIDStability(t *testing.T) {
	first := loginPayloadField(t, buildTestLoginPayload(t, wacore.LoginContinuity{SessionID: 0x0A0B0C0D}), 9).bytes
	same := loginPayloadField(t, buildTestLoginPayload(t, wacore.LoginContinuity{SessionID: 0x0A0B0C0D}), 9).bytes
	if !bytes.Equal(first, same) {
		t.Fatalf("same session id produced different field-9 bytes: %x vs %x", first, same)
	}

	other := loginPayloadField(t, buildTestLoginPayload(t, wacore.LoginContinuity{SessionID: 0x0A0B0C0E}), 9).bytes
	if bytes.Equal(first, other) {
		t.Fatalf("different session ids produced identical field-9 bytes: %x", first)
	}
}

// TestLoginPayloadConnectReason asserts connectReason (field 13) is USER_ACTIVATED on a
// first connect and ERROR_RECONNECT on a reconnect.
func TestLoginPayloadConnectReason(t *testing.T) {
	firstConnect := buildTestLoginPayload(t, wacore.LoginContinuity{SessionID: 1, ConnectReason: wacore.WAConnectReasonUserActivated})
	reconnect := buildTestLoginPayload(t, wacore.LoginContinuity{SessionID: 1, ConnectReason: wacore.WAConnectReasonErrorReconnect})

	firstReason := loginPayloadField(t, firstConnect, 13)
	if firstReason.wireType != 0 || firstReason.varint != uint64(wacore.WAConnectReasonUserActivated) {
		t.Fatalf("first-connect connectReason = %d (wt %d), want %d", firstReason.varint, firstReason.wireType, wacore.WAConnectReasonUserActivated)
	}
	reconnectReason := loginPayloadField(t, reconnect, 13)
	if reconnectReason.wireType != 0 || reconnectReason.varint != uint64(wacore.WAConnectReasonErrorReconnect) {
		t.Fatalf("reconnect connectReason = %d (wt %d), want %d", reconnectReason.varint, reconnectReason.wireType, wacore.WAConnectReasonErrorReconnect)
	}
	if firstReason.varint == reconnectReason.varint {
		t.Fatalf("connectReason did not differ between first connect and reconnect")
	}
}

// TestLoginPayloadAttemptCount asserts connectAttemptCount (field 16) reflects the threaded
// value.
func TestLoginPayloadAttemptCount(t *testing.T) {
	for _, count := range []uint32{0, 1, 5} {
		payload := buildTestLoginPayload(t, wacore.LoginContinuity{SessionID: 1, ConnectAttemptCount: count, ConnectReason: wacore.WAConnectReasonErrorReconnect})
		field16 := loginPayloadField(t, payload, 16)
		if field16.wireType != 0 || field16.varint != uint64(count) {
			t.Fatalf("connectAttemptCount = %d (wt %d), want %d", field16.varint, field16.wireType, count)
		}
	}
}
