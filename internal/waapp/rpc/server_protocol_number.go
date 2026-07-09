package rpc

import (
	"context"

	waappv1 "github.com/byte-v-forge/wa-app/gen/go/byte/v/forge/waapp/v1"
	"github.com/byte-v-forge/wa-app/internal/waapp/shared"
)

// protocolNumberExporter is the narrow engine capability the export needs: given
// a client_profile_id it renders the committed native state as the 协议号 6-段
// string. The native engine satisfies it via method promotion; kept local so the
// RPC layer depends on the capability, not the concrete engine implementation.
type protocolNumberExporter interface {
	ExportProtocolNumber(context.Context, string) (string, error)
}

// ExportProtocolNumber resolves an account (via the shared account-login
// selector: wa_account_id / client_profile_id / login_state_id /
// registered_identity_id) to its active client profile and returns the WhatsApp
// 协议号 6-段 string built from that profile's committed native state.
//
// Value-safety: the returned string embeds the account's private keys = full
// account control. Callers MUST NOT log it; it is carried only to the
// authenticated dashboard.
func (s *serverCore) ExportProtocolNumber(ctx context.Context, selector *waappv1.AccountLoginSelector) (string, error) {
	loginState, err := s.accountSettingsLoginState(ctx, selector)
	if err != nil {
		return "", err
	}
	exporter, ok := s.runner.(protocolNumberExporter)
	if !ok {
		return "", shared.NewError(waappv1.WaErrorCode_WA_ERROR_CODE_UNSUPPORTED_OPERATION, "native engine is required", false)
	}
	return exporter.ExportProtocolNumber(ctx, loginState.GetClientProfileId())
}
