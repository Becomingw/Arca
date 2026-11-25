package plugins

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeGoPluginQuotaTransitions(t *testing.T) {
	plugin, ok := jsplugin.DefaultRegistry.Get("opencode-go")
	require.True(t, ok)
	require.True(t, plugin.Meta.HasProviderPart())
	route, ok := plugin.Meta.RelayRoute("/v1/chat/completions", "gemini-4-argon-contributor")
	require.True(t, ok)
	assert.Equal(t, "openai_chat_completions_to_gemini_generate_content", route.Converter)

	credentials := jsplugin.CredentialContext{BaseURL: "https://opencode.example/inference/go/v1", APIKey: "fixture-key"}
	request, err := plugin.CallBuildBalanceRequest(t.Context(), credentials)
	require.NoError(t, err)
	assert.Equal(t, "https://opencode.example/zen/go/v1/usage", request.URL)
	assert.Equal(t, "Bearer fixture-key", request.Headers["Authorization"])
	credentials.BaseURL = "https://opencode.example"
	fiveHours, week, month := time.Now().Add(time.Hour).Truncate(time.Second), time.Now().Add(3*time.Hour).Truncate(time.Second), time.Now().Add(5*time.Hour).Truncate(time.Second)
	exhausted := map[string]any{"usage": map[string]any{
		"rolling": map[string]any{"status": "rate-limited", "percent": 99, "resetsAt": fiveHours.Format(time.RFC3339)},
		"weekly":  map[string]any{"status": "ok", "percent": "100%", "resets_at": week.UnixMilli()},
		"monthly": map[string]any{"status": "ok", "usedPercent": 0, "reset_at": month.Unix()},
	}}
	healthy := map[string]any{
		"rolling": map[string]any{"status": "ok", "percent": 28},
		"weekly":  map[string]any{"status": "ok", "percent": 17},
		"monthly": map[string]any{"status": "ok", "percent": 0},
	}
	for _, tc := range []struct {
		name, previous, disabledReason, want string
		status                               int
		body                                 any
		wantBalance                          bool
	}{
		{"pause on active overlapping windows", "enabled", "", "auto_disabled", 200, exhausted, true},
		{"recover the plugin's exhausted key", "auto_disabled", "OpenCode Go quota exhausted", "enabled", 200, healthy, true},
		{"keep unrelated disable", "auto_disabled", "operator action", "", 200, healthy, true},
		{"keep when a window is absent", "auto_disabled", "OpenCode Go quota exhausted", "", 200, map[string]any{"rolling": healthy["rolling"]}, true},
		{"pause rejected API key", "enabled", "", "auto_disabled", 401, nil, false},
		{"pause unsubscribed API key", "enabled", "", "auto_disabled", 403, nil, false},
		{"keep last known state on throttled probe", "auto_disabled", "OpenCode Go quota exhausted", "", 429, nil, false},
		{"keep last known state on upstream outage", "auto_disabled", "OpenCode Go quota exhausted", "", 503, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := common.Marshal(tc.body)
			require.NoError(t, err)
			ctx := jsplugin.WithFetcher(t.Context(), func(_ context.Context, request jsplugin.FetchRequest) (jsplugin.FetchResponse, error) {
				assert.Equal(t, "https://opencode.example/zen/go/v1/usage", request.URL)
				assert.Equal(t, "Bearer fixture-key", request.Headers["Authorization"])
				assert.Equal(t, http.MethodGet, request.Method)
				return jsplugin.FetchResponse{Status: tc.status, Headers: map[string]string{"content-type": "application/json"}, Body: body}, nil
			})
			result, err := plugin.CallScheduled(ctx, jsplugin.ScheduledContext{
				CredentialContext: credentials, Reason: "timer", KeyStatus: tc.previous, DisabledReason: tc.disabledReason,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.KeyStatus)
			assert.Equal(t, tc.wantBalance, result.Balance != nil)
			if tc.name != "pause on active overlapping windows" {
				return
			}
			require.NotNil(t, result.Balance)
			require.Len(t, result.Balance.Windows, 3)
			assert.Equal(t, fiveHours.Unix(), result.Balance.Windows[0].ResetAt)
			assert.Equal(t, week.Unix(), result.Balance.Windows[1].ResetAt)
			assert.Zero(t, result.Balance.Windows[2].ResetAt)
			assert.InDelta(t, (3 * time.Hour).Seconds(), result.Next.Seconds(), 3)
		})
	}
}
