package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	waappv1 "github.com/byte-v-forge/wa-app/gen/go/byte/v/forge/waapp/v1"
)

type wamsysMaterialInput struct {
	Capture *waappv1.WamsysCapture
	Kind    waappv1.RegistrationRequestKind
	Phone   *waappv1.PhoneTarget
	State   nativeState
	Now     time.Time
}

type wamsysMaterialProvider interface {
	RegistrationMaterial(context.Context, wamsysMaterialInput) (*waappv1.WamsysCapture, error)
}

type localWamsysMaterialProvider struct{}

const (
	nativeWamsysSmallByteLength = 32
)

var (
	nativeWamsysRuntimeStartedAt      = time.Now().UTC()
	nativeWamsysRuntimeBootID         = newUUIDString()
	nativeWamsysRuntimeDataDirMTime   = nativeWamsysRuntimeStartedAt.Add(-nativeRuntimeJitterDuration(120, 220))
	nativeWamsysRuntimeSourceDirMTime = nativeWamsysRuntimeStartedAt.Add(-nativeRuntimeJitterDuration(150, 260))
)

func (localWamsysMaterialProvider) RegistrationMaterial(ctx context.Context, input wamsysMaterialInput) (*waappv1.WamsysCapture, error) {
	_ = ctx
	if input.Capture != nil {
		return input.Capture, nil
	}
	switch input.Kind {
	case waappv1.RegistrationRequestKind_REGISTRATION_REQUEST_KIND_EXIST,
		waappv1.RegistrationRequestKind_REGISTRATION_REQUEST_KIND_CODE:
		return buildLocalWamsysCapture(input)
	default:
		return nil, nil
	}
}

func buildLocalWamsysCapture(input wamsysMaterialInput) (*waappv1.WamsysCapture, error) {
	gpia, err := buildNativeGPIAErrorMaterial(input)
	if err != nil {
		return nil, err
	}
	ga, err := buildLocalWamsysGA(input)
	if err != nil {
		return nil, err
	}
	return &waappv1.WamsysCapture{MapParams: []*waappv1.WamsysMapParam{
		{Key: "gpia", Value: []byte(gpia.Primary)},
		{Key: "_ge", Value: []byte(`{"sb":false,"sv":false}`)},
		{Key: "_gi", Value: []byte(gpia.DeviceCompact)},
		{Key: "_gg", Value: []byte(gpia.CodeCompact)},
		{Key: "_gp", Value: localWamsysBase64Bytes(deriveLocalWamsysBytes(input, "_gp", nativeWamsysSmallByteLength))},
		{Key: "_ga", Value: ga},
		{Key: "aid", Value: localWamsysBase64Bytes(deriveLocalWamsysBytes(input, "aid", nativeWamsysSmallByteLength))},
	}}, nil
}

func buildLocalWamsysGA(input wamsysMaterialInput) ([]byte, error) {
	now := nativeWamsysNow(input)
	bi, err := encryptNativeGPIAData(nativeGPIAKeySource(input.State), []byte(nativeWamsysRuntimeBootID))
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(
		`{"mu":false,"mp":false,"ae":0,"ai":%d,"ap":%d,"bi":%q}`,
		nativeRuntimePathAgeSeconds(now, nativeWamsysRuntimeDataDirMTime),
		nativeRuntimePathAgeSeconds(now, nativeWamsysRuntimeSourceDirMTime),
		bi,
	)), nil
}

func nativeWamsysNow(input wamsysMaterialInput) time.Time {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	return now.UTC()
}

func nativeRuntimePathAgeSeconds(now time.Time, modifiedAt time.Time) int64 {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if modifiedAt.IsZero() || now.Before(modifiedAt) {
		return 0
	}
	return int64(now.Sub(modifiedAt).Seconds())
}

func nativeRuntimeJitterDuration(minSeconds int64, maxSeconds int64) time.Duration {
	if maxSeconds <= minSeconds {
		return time.Duration(minSeconds) * time.Second
	}
	raw := randomBytes(4)
	value := binary.BigEndian.Uint32(raw)
	span := uint32(maxSeconds - minSeconds + 1)
	return time.Duration(minSeconds+int64(value%span)) * time.Second
}

func localWamsysBase64Bytes(value []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(value))
}

func deriveLocalWamsysBytes(input wamsysMaterialInput, label string, length int) []byte {
	seed := strings.Join([]string{
		"byte-v-forge-wa-wamsys-precision/v1",
		label,
		phoneCC(input.Phone),
		phoneNational(input.Phone),
		input.State.Profile.PhoneSHA256,
		input.State.Profile.FDID,
		input.State.Profile.ExpIDUUID,
		input.State.Profile.AccessSessionIDUUID,
		input.State.Profile.IDHex,
		input.State.Profile.BackupTokenHex,
		input.State.AuthKey,
		input.State.KeyBundle.IdentityPublic,
	}, "|")
	key := sha256.Sum256([]byte(seed))
	out := make([]byte, 0, length)
	for counter := uint32(0); len(out) < length; counter++ {
		mac := hmac.New(sha256.New, key[:])
		_, _ = mac.Write([]byte(label))
		_, _ = mac.Write(binary.BigEndian.AppendUint32(nil, counter))
		out = append(out, mac.Sum(nil)...)
	}
	return out[:length]
}

func (e *NativeEngine) applyRuntimeWamsys(
	ctx context.Context,
	kind waappv1.RegistrationRequestKind,
	phone *waappv1.PhoneTarget,
	state nativeState,
	params map[string]string,
	rawKeys map[string]struct{},
) error {
	capture, err := e.wamsysProvider().RegistrationMaterial(ctx, wamsysMaterialInput{Kind: kind, Phone: phone, State: state, Now: e.clock.Now()})
	if err != nil {
		return err
	}
	applyOpaqueWamsysMapParams(params, rawKeys, capture)
	return nil
}

func applyOpaqueWamsysMapParams(params map[string]string, rawKeys map[string]struct{}, capture *waappv1.WamsysCapture) {
	if capture == nil {
		return
	}
	for _, item := range capture.GetMapParams() {
		key := item.GetKey()
		if !isOpaqueWamsysMapKey(key) {
			continue
		}
		params[key] = pctBytes(item.GetValue())
		rawKeys[key] = struct{}{}
	}
}

func applyOrderedWamsysKey(params *orderedParams, capture *waappv1.WamsysCapture, key string) {
	if params == nil || capture == nil || !isOpaqueWamsysMapKey(key) {
		return
	}
	for _, item := range capture.GetMapParams() {
		if item.GetKey() == key {
			params.set(key, pctBytes(item.GetValue()), true)
			return
		}
	}
}

func applyOrderedWamsysExcept(params *orderedParams, capture *waappv1.WamsysCapture, excluded map[string]struct{}) {
	if params == nil || capture == nil {
		return
	}
	for _, item := range capture.GetMapParams() {
		key := item.GetKey()
		if !isOpaqueWamsysMapKey(key) {
			continue
		}
		if _, skip := excluded[key]; skip {
			continue
		}
		params.set(key, pctBytes(item.GetValue()), true)
	}
}

// Opaque WAMSYS values stay behind a dedicated material provider so registration
// maps do not leak opaque blobs into generic phone profile fields.
var opaqueWamsysMapKeys = map[string]struct{}{
	"gpia": {},
	"_ge":  {},
	"_gi":  {},
	"_gg":  {},
	"_gp":  {},
	"_ga":  {},
	"aid":  {},
}

func isOpaqueWamsysMapKey(key string) bool {
	_, ok := opaqueWamsysMapKeys[key]
	return ok
}
