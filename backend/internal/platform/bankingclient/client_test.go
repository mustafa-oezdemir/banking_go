package bankingclient

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevokeCustomerSessionsUsesAuthenticatedPrivateCommand(t *testing.T) {
	userID := uuid.New()
	token := "0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/internal/customers/"+userID.String()+"/sessions/revoke", r.URL.Path)
		assert.Equal(t, token, r.Header.Get("X-Internal-Service-Token"))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, token)
	require.NoError(t, err)
	require.NoError(t, client.RevokeCustomerSessions(t.Context(), userID))
}
