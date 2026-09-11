package bankingclient

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncCustomerSessionVersionUsesAuthenticatedPrivateCommand(t *testing.T) {
	userID := uuid.New()
	token := "0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/internal/customers/"+userID.String()+"/session-version", r.URL.Path)
		assert.Equal(t, token, r.Header.Get("X-Internal-Service-Token"))
		assert.JSONEq(t, `{"session_version":7}`, func() string {
			defer r.Body.Close()
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			return string(body)
		}())
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, token)
	require.NoError(t, err)
	require.NoError(t, client.SyncCustomerSessionVersion(t.Context(), userID, 7))
}
