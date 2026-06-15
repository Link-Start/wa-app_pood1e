package app

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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
	nativeWamsysRequestedPermissionsDigest = "NNj5BoWX+yvZBYEY46Ze+Ad6Ykk0Z27FjgSysvkzzCU="
	nativeWamsysInstallAgeMinSeconds       = int64(21600)
	nativeWamsysInstallAgeSpreadSeconds    = uint64(2048)
	nativeWamsysDataAgeDeltaMinSeconds     = int64(5600)
	nativeWamsysDataAgeDeltaSpreadSeconds  = uint64(768)
	nativeWamsysPathAgeJitterSeconds       = uint64(512)
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
		{Key: "_gp", Value: []byte(nativeWamsysRequestedPermissionsDigest)},
		{Key: "_ga", Value: ga},
		{Key: "aid", Value: nativeWamsysAndroidIDDigest(input)},
	}}, nil
}

func buildLocalWamsysGA(input wamsysMaterialInput) ([]byte, error) {
	bi, err := encryptNativeGPIAData(nativeGPIAKeySource(input.State), []byte(nativeWamsysBootID(input)))
	if err != nil {
		return nil, err
	}
	return renderNativeGPIAJSONObject([]nativeGPIAJSONField{
		{Key: "bi", Value: bi},
		{Key: "ap", Value: nativeWamsysPathAgeSeconds(input, "source-dir")},
		{Key: "ai", Value: nativeWamsysPathAgeSeconds(input, "data-dir")},
		{Key: "mp", Value: false},
		{Key: "ae", Value: nativeWamsysPathAgeSeconds(input, "external-files-dir")},
		{Key: "mu", Value: false},
	})
}

func nativeWamsysPathAgeSeconds(input wamsysMaterialInput, label string) int64 {
	installAge := nativeWamsysVirtualInstallAgeSeconds(input)
	switch label {
	case "source-dir":
		return installAge + nativeWamsysRuntimePathAges.SourceJitterSeconds
	case "data-dir":
		return maxInt64(1024, installAge-nativeWamsysRuntimePathAges.DataDeltaSeconds)
	case "external-files-dir":
		return maxInt64(1024, installAge-nativeWamsysRuntimePathAges.ExternalJitterSeconds)
	default:
		return installAge
	}
}

func nativeWamsysVirtualInstallAgeSeconds(input wamsysMaterialInput) int64 {
	age := nativeWamsysNow(input).Unix() - nativeWamsysRuntimeInstallUnix
	return maxInt64(nativeWamsysInstallAgeMinSeconds, age)
}

type nativeWamsysRuntimePathAgeOffsets struct {
	SourceJitterSeconds   int64
	DataDeltaSeconds      int64
	ExternalJitterSeconds int64
}

var nativeWamsysRuntimeInstallUnix = newNativeWamsysRuntimeInstallUnix()
var nativeWamsysRuntimePathAges = newNativeWamsysRuntimePathAgeOffsets()

func newNativeWamsysRuntimeInstallUnix() int64 {
	return time.Now().UTC().Unix() - nativeWamsysInstallAgeMinSeconds - nativeWamsysRandomJitterSeconds(nativeWamsysInstallAgeSpreadSeconds)
}

func newNativeWamsysRuntimePathAgeOffsets() nativeWamsysRuntimePathAgeOffsets {
	return nativeWamsysRuntimePathAgeOffsets{
		SourceJitterSeconds:   nativeWamsysRandomJitterSeconds(nativeWamsysPathAgeJitterSeconds),
		DataDeltaSeconds:      nativeWamsysDataAgeDeltaMinSeconds + nativeWamsysRandomJitterSeconds(nativeWamsysDataAgeDeltaSpreadSeconds),
		ExternalJitterSeconds: nativeWamsysRandomJitterSeconds(nativeWamsysPathAgeJitterSeconds),
	}
}

func nativeWamsysRandomJitterSeconds(spread uint64) int64 {
	if spread == 0 {
		return 0
	}
	raw := randomBytes(8)
	return int64(binary.BigEndian.Uint64(raw) % spread)
}

func nativeWamsysNow(input wamsysMaterialInput) time.Time {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	return now.UTC()
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func nativeWamsysBootID(input wamsysMaterialInput) string {
	_ = input
	return nativeRuntimeBootID
}

var nativeRuntimeBootID = newNativeRuntimeBootID()

func newNativeRuntimeBootID() string {
	sum := randomBytes(16)
	id := append([]byte(nil), sum[:16]...)
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(id)
	return strings.Join([]string{
		encoded[0:8],
		encoded[8:12],
		encoded[12:16],
		encoded[16:20],
		encoded[20:32],
	}, "-")
}

func nativeWamsysAndroidIDDigest(input wamsysMaterialInput) []byte {
	return []byte(nativeGPIASHA256Base64([]byte(nativeWamsysAndroidID(input))))
}

func nativeWamsysAndroidID(input wamsysMaterialInput) string {
	profile := normalizeNativePhoneProfile(input.State.Profile, "")
	seed := strings.Join([]string{
		"byte-v-forge-wa-android-id/v1",
		phoneCC(input.Phone),
		phoneNational(input.Phone),
		input.State.Profile.PhoneSHA256,
		profile.FDID,
		profile.ExpIDUUID,
		input.State.Profile.AccessSessionIDUUID,
		input.State.AuthKey,
		input.State.KeyBundle.IdentityPublic,
	}, "|")
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("%016x", binary.BigEndian.Uint64(sum[:8]))
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
	excluded := map[string]struct{}{}
	if !nativeShouldSendRegistrationGPIA(state) {
		excluded["gpia"] = struct{}{}
	}
	applyOpaqueWamsysMapParams(params, rawKeys, capture, excluded)
	return nil
}

func applyOpaqueWamsysMapParams(params map[string]string, rawKeys map[string]struct{}, capture *waappv1.WamsysCapture, excluded map[string]struct{}) {
	if capture == nil {
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
