package engine

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/byte-v-forge/wa-app/internal/waapp/shared"
)

// sixSegmentKeyBytes is the raw Curve25519 key length; public and private keys
// are both 32 bytes.
const sixSegmentKeyBytes = 32

// ExportProtocolNumber loads the committed native state for a client profile and
// renders it as the WhatsApp 协议号 6-段 import string.
//
// Value-safety: the returned string embeds this account's private keys, i.e.
// full account control. Callers MUST NOT log it — it belongs only in the
// authenticated dashboard response.
func (e *engineCore) ExportProtocolNumber(ctx context.Context, clientProfileID string) (string, error) {
	state, err := e.loadState(ctx, clientProfileID)
	if err != nil {
		return "", err
	}
	return buildSixSegment(&state)
}

// buildSixSegment renders a registered account's committed native state into the
// WhatsApp "协议号 6 段" string consumed by WA cloud-control / marketing tools:
//
//		手机号,公钥,私钥,消息公钥,消息私钥,号码ID
//
//	  - 手机号: bare digits, country code + national number, no '+'.
//	  - 公钥/私钥/消息公钥/消息私钥: 32-byte Curve25519 keys in STANDARD base64
//	    (StdEncoding, with padding).
//	  - 号码ID: standard base64 of <phone-ascii-bytes> || <account-id bytes>.
//
// Value-safety: the result carries private key material. Neither it nor any of
// the keys/tokens are ever logged; the errors below carry only key labels and
// byte lengths, never key material.
func buildSixSegment(ns *NativeState) (string, error) {
	if ns == nil {
		return "", fmt.Errorf("native state is unavailable")
	}
	phoneDigits := shared.DigitsOnly(ns.CC + ns.Phone)
	if phoneDigits == "" {
		return "", fmt.Errorf("account phone digits are unavailable")
	}

	// ─────────────────────────────────────────────────────────────────────────
	// FLIPPABLE SPOT #1 — KEY-ORDER PAIRING (UNVERIFIED)
	// Target order is 手机号,公钥,私钥,消息公钥,消息私钥,号码ID. We currently map:
	//     公钥/私钥     (fields 2,3) ← Signal.Identity (Signal identity keypair)
	//     消息公钥/私钥 (fields 4,5) ← ChatStatic      (Noise static/connection keypair)
	// This pairing has NOT been verified against a real target-tool import. If a
	// test-import shows the two pairs are swapped, exchange the two keypairs on
	// the next two lines (identityPair <-> messagePair); nothing else changes.
	identityPair := ns.Signal.Identity // → field 2 公钥 / field 3 私钥
	messagePair := ns.ChatStatic       // → field 4 消息公钥 / field 5 消息私钥
	// ─────────────────────────────────────────────────────────────────────────

	publicKey, err := sixSegmentStdKey(identityPair.publicBytes, "public key")
	if err != nil {
		return "", err
	}
	privateKey, err := sixSegmentStdKey(identityPair.privateBytes, "private key")
	if err != nil {
		return "", err
	}
	messagePublicKey, err := sixSegmentStdKey(messagePair.publicBytes, "message public key")
	if err != nil {
		return "", err
	}
	messagePrivateKey, err := sixSegmentStdKey(messagePair.privateBytes, "message private key")
	if err != nil {
		return "", err
	}

	// ─────────────────────────────────────────────────────────────────────────
	// FLIPPABLE SPOT #2 — FIELD 6 号码ID CONSTRUCTION (BEST-GUESS)
	// 号码ID = StdBase64( phoneDigits(ASCII) || accountIdBytes ). In the reference
	// sample the decoded field 6 begins with the phone-number ASCII followed by
	// ~21 id bytes; we take those id bytes from Profile.BackupToken. The exact
	// id-byte source and ordering are a best-guess pending a real sample — adjust
	// numberIDAccountBytes / this concatenation here if an import proves otherwise.
	idBytes, err := numberIDAccountBytes(ns)
	if err != nil {
		return "", err
	}
	numberID := base64.StdEncoding.EncodeToString(append([]byte(phoneDigits), idBytes...))
	// ─────────────────────────────────────────────────────────────────────────

	return strings.Join([]string{phoneDigits, publicKey, privateKey, messagePublicKey, messagePrivateKey, numberID}, ","), nil
}

// sixSegmentStdKey decodes a stored Curve25519 key (any base64 variant) to its
// raw 32 bytes and re-encodes it as standard base64 (with padding), validating
// the key length. It never includes key material in errors.
func sixSegmentStdKey(decode func() ([]byte, error), label string) (string, error) {
	raw, err := decode()
	if err != nil {
		return "", fmt.Errorf("%s is not decodable base64", label)
	}
	if len(raw) != sixSegmentKeyBytes {
		return "", fmt.Errorf("%s must be %d bytes, got %d", label, sixSegmentKeyBytes, len(raw))
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// numberIDAccountBytes resolves the account-id bytes appended after the phone in
// field 6. It prefers Profile.BackupToken (decoded via the shared base64-any
// helper) and falls back to the hex-encoded BackupTokenHex when the base64 form
// is empty or not decodable — BackupToken is persisted percent-encoded, so the
// hex form is usually the clean source.
func numberIDAccountBytes(ns *NativeState) ([]byte, error) {
	if token := strings.TrimSpace(ns.Profile.BackupToken); token != "" {
		if raw, err := decodeB64Any(token); err == nil && len(raw) > 0 {
			return raw, nil
		}
	}
	if hexToken := strings.TrimSpace(ns.Profile.BackupTokenHex); hexToken != "" {
		if raw, err := hex.DecodeString(hexToken); err == nil && len(raw) > 0 {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("account id material is unavailable")
}
