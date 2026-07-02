package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
)

func TestAuthMethodID(t *testing.T) {
	tests := []struct {
		name string
		m    acp.AuthMethod
		want string
	}{
		{
			name: "agent",
			m:    acp.AuthMethod{Agent: &acp.AuthMethodAgent{Id: "agent-login", Name: "Login"}},
			want: "agent-login",
		},
		{
			name: "env_var",
			m:    acp.AuthMethod{EnvVar: &acp.AuthMethodEnvVarInline{Id: "api-key", Name: "API Key"}},
			want: "api-key",
		},
		{
			name: "empty",
			m:    acp.AuthMethod{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authMethodID(tt.m); got != tt.want {
				t.Fatalf("authMethodID() = %q, want %q", got, tt.want)
			}
		})
	}
}
