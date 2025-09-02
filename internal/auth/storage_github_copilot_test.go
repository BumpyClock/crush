package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGithubCopilotStorage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	am := NewAuthManager(dir)

	require.False(t, am.HasGithubCopilotAuth())

	// Store refresh (GitHub OAuth token).
	err := am.StoreGithubCopilotRefresh("test-github-oauth-token")
	require.NoError(t, err)
	require.True(t, am.HasGithubCopilotAuth())

	// Verify file exists and contents parse.
	data, err := am.LoadAuthData()
	require.NoError(t, err)
	require.NotNil(t, data.GithubCopilot)
	require.Equal(t, "oauth", data.GithubCopilot.Type)
	require.Equal(t, "test-github-oauth-token", data.GithubCopilot.Refresh)

	// Clear and verify removal.
	err = am.ClearGithubCopilotCredentials()
	require.NoError(t, err)
	require.False(t, am.HasGithubCopilotAuth())

	// Ensure file remains present but field cleared.
	_, statErr := os.Stat(filepath.Join(dir, AuthFileName))
	require.NoError(t, statErr)
	data, err = am.LoadAuthData()
	require.NoError(t, err)
	require.Nil(t, data.GithubCopilot)
}
