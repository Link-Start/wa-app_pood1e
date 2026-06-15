package app

import (
	"crypto/aes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/curve25519"
)

const (
	nativeGPIAErrorCode       = -9
	nativeGPIAPackageName     = "com.whatsapp"
	nativeGPIASourceSize      = "141711087"
	nativeGPIASourceDigest    = "vJrNuYDSuWUZ87O1W5+xs/2g74mwPA2JO+dkqjlJZG4="
	nativeGPIACertDigest      = "OKD31QX+GP7GT780Psqq8xDb15k="
	nativeGPIAClassesDigest   = "qoblldcHz4lA84Sgs1QLZWPpd6YKG25zf0GwJZdTHXk="
	nativeGPIANativeLibDigest = "G9McgxRaSjtq92o7zx0fbf3Ak7+SPmxxNyvNXS01hlM="
	nativeGPIADataSODigest    = "0j9kw9djlCtmCCavV7go2wwge+2os853ubiE7F7Dew4="
	nativeGPIASourceDirPath   = "/data/data/com.whatsapp/files/fixtures/original-app-release.apk"
)

type nativeGPIAMaterial struct {
	Primary       string
	CodeCompact   string
	DeviceCompact string
}

type nativeGPIAJSONField struct {
	Key   string
	Value any
}

func buildNativeGPIAErrorMaterial(input wamsysMaterialInput) (nativeGPIAMaterial, error) {
	sourceDir := nativeGPIASourceDir(input)
	pathDigest := nativeGPIASHA256Base64([]byte(sourceDir))
	keySource := nativeGPIAKeySource(input.State)
	primary, err := encryptNativeGPIAJSON(keySource, []nativeGPIAJSONField{
		{Key: "sizeInBytes", Value: nativeGPIASourceSize},
		{Key: "packageName", Value: nativeGPIAPackageName},
		{Key: "code", Value: nativeGPIAErrorCode},
		{Key: "shatr", Value: nativeGPIASourceDigest},
		{Key: "p", Value: sourceDir},
		{Key: "cert", Value: nativeGPIACertDigest},
		{Key: "sha256", Value: pathDigest},
	})
	if err != nil {
		return nativeGPIAMaterial{}, err
	}
	codeCompact, err := encryptNativeGPIAJSON(keySource, []nativeGPIAJSONField{
		{Key: "_ic", Value: nativeGPIAErrorCode},
	})
	if err != nil {
		return nativeGPIAMaterial{}, err
	}
	deviceCompact, err := encryptNativeGPIAJSON(keySource, []nativeGPIAJSONField{
		{Key: "_dh", Value: nativeGPIAClassesDigest},
		{Key: "_iln", Value: nativeGPIADataSODigest},
		{Key: "_isb", Value: nativeGPIASourceSize},
		{Key: "_ip", Value: nativeGPIAPackageName},
		{Key: "did", Value: nativeGPIADisplayID(input.State)},
		{Key: "_p", Value: sourceDir},
		{Key: "_ln", Value: nativeGPIANativeLibDigest},
		{Key: "_ist", Value: nativeGPIASourceDigest},
		{Key: "_icr", Value: nativeGPIACertDigest},
		{Key: "_is", Value: pathDigest},
	})
	if err != nil {
		return nativeGPIAMaterial{}, err
	}
	return nativeGPIAMaterial{Primary: primary, CodeCompact: codeCompact, DeviceCompact: deviceCompact}, nil
}

func nativeGPIADisplayID(state nativeState) string {
	profile := normalizeNativePhoneProfile(state.Profile, "")
	return firstNonEmpty(profile.BuildDisplayID, defaultNativeDeviceModel().BuildDisplayID)
}

func nativeGPIASourceDir(input wamsysMaterialInput) string {
	_ = input
	return nativeGPIASourceDirPath
}

func nativeGPIASHA256Base64(value []byte) string {
	sum := sha256.Sum256(value)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func nativeGPIAKeySource(state nativeState) string {
	if private, err := state.ChatStatic.privateBytes(); err == nil && len(private) == curve25519.ScalarSize {
		if public, err := curve25519.X25519(private, curve25519.Basepoint); err == nil {
			return base64.StdEncoding.EncodeToString(public)
		}
	}
	for _, candidate := range []string{state.AuthKey, state.ChatStatic.Public} {
		public, err := decodeB64Any(candidate)
		if err == nil && len(public) == curve25519.PointSize {
			return base64.StdEncoding.EncodeToString(public)
		}
	}
	return "default"
}

func encryptNativeGPIAJSON(keySource string, fields []nativeGPIAJSONField) (string, error) {
	plaintext, err := renderNativeGPIAJSONObject(fields)
	if err != nil {
		return "", err
	}
	return encryptNativeGPIAData(keySource, plaintext)
}

func encryptNativeGPIAData(keySource string, plaintext []byte) (string, error) {
	key := sha256.Sum256([]byte(keySource))
	iv := randomBytes(aes.BlockSize)
	ciphertext, err := aesCBCPKCS7Encrypt(plaintext, key[:], iv)
	if err != nil {
		return "", err
	}
	out := make([]byte, 0, len(iv)+len(ciphertext))
	out = append(out, iv...)
	out = append(out, ciphertext...)
	return base64.StdEncoding.EncodeToString(out), nil
}

func renderNativeGPIAJSONObject(fields []nativeGPIAJSONField) ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, field := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(field.Key)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		value, err := renderNativeGPIAJSONValue(field.Value)
		if err != nil {
			return nil, err
		}
		b.Write(value)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

func renderNativeGPIAJSONValue(value any) ([]byte, error) {
	switch v := value.(type) {
	case string:
		return renderNativeGPIAJSONString(v)
	case int:
		return []byte(strconv.Itoa(v)), nil
	case int64:
		return []byte(strconv.FormatInt(v, 10)), nil
	case bool:
		return []byte(strconv.FormatBool(v)), nil
	case nil:
		return []byte("null"), nil
	default:
		return nil, fmt.Errorf("unsupported native GPIA JSON value type %T", value)
	}
}

func renderNativeGPIAJSONString(value string) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return []byte(strings.ReplaceAll(string(encoded), `/`, `\/`)), nil
}
