package cache

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/syself/hetzner-cloud-controller-manager/internal/credentials"
	"github.com/syself/hetzner-cloud-controller-manager/internal/mocks"
	"github.com/syself/hrobot-go/models"
)

func Test_updateRobotCredentials(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	os.Unsetenv(robotUserNameENVVar)
	os.Unsetenv(robotPasswordENVVar)

	rootDir, err := os.MkdirTemp("", "Test_newHcloudClient-*")
	require.NoError(t, err)

	credentialsDir := credentials.GetDirectory(rootDir)
	err = os.MkdirAll(credentialsDir, 0o755)
	require.NoError(t, err)

	err = os.Symlink("..data/robot-user", filepath.Join(credentialsDir, "robot-user"))
	require.NoError(t, err)

	err = os.Symlink("..data/robot-password", filepath.Join(credentialsDir, "robot-password"))
	require.NoError(t, err)

	err = writeCredentials(rootDir, "my-robot-user", "my-robot-password")
	require.NoError(t, err)

	wantAuth := base64.StdEncoding.EncodeToString([]byte("my-robot-user:my-robot-password"))

	mux.HandleFunc("/robot/server", func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		require.Equal(t, "Basic "+wantAuth, header)
		fmt.Println(header)
		json.NewEncoder(w).Encode([]models.ServerResponse{
			{
				Server: models.Server{
					ServerIP:      "123.123.123.12",
					ServerIPv6Net: "2a01:f48:111:4221::",
					ServerNumber:  321,
					Name:          "bm-server1",
				},
			},
		})
	})

	httpClient := server.Client()
	robotClient, err := NewCachedRobotClient(rootDir, httpClient, server.URL+"/robot")
	require.NoError(t, err)
	require.NotNil(t, robotClient)
	err = credentials.Watch(credentials.GetDirectory(rootDir), nil, robotClient)
	require.NoError(t, err)
	servers, err := robotClient.ServerGetList()
	require.NoError(t, err)
	require.Len(t, servers, 1)

	oldCount := credentials.GetRobotReloadCounter()
	err = writeCredentials(rootDir, "user2", "password2")
	require.NoError(t, err)
	start := time.Now()
	for credentials.GetRobotReloadCounter() <= oldCount {
		if time.Since(start) > time.Second*3 {
			t.Fatal("timeout waiting for reload")
		}
		time.Sleep(time.Millisecond * 100)
	}

	wantAuth = base64.StdEncoding.EncodeToString([]byte("user2:password2"))
	servers, err = robotClient.ServerGetList()
	require.NoError(t, err)
	require.Len(t, servers, 1)
}

func TestForcedRefreshNameExpiresAfterCacheTimeout(t *testing.T) {
	now := time.Date(2026, time.March, 16, 10, 0, 0, 0, time.UTC)
	client := &cacheRobotClient{
		now:     func() time.Time { return now },
		timeout: 10 * time.Minute,
	}
	client.forcedRefreshServerNames = make(map[string]time.Time)
	client.forcedRefreshServerNames["bm-missing"] = now
	require.True(t, client.nodeHasAlreadyForcedRefresh("bm-missing"))

	now = now.Add(client.timeout - time.Second)
	require.True(t, client.nodeHasAlreadyForcedRefresh("bm-missing"))

	now = now.Add(2 * time.Second)
	require.False(t, client.nodeHasAlreadyForcedRefresh("bm-missing"))
	require.Empty(t, client.forcedRefreshServerNames)
}

func TestForcedRefreshNameTimestampCanBeUpdated(t *testing.T) {
	now := time.Date(2026, time.March, 16, 10, 0, 0, 0, time.UTC)
	client := &cacheRobotClient{
		now:     func() time.Time { return now },
		timeout: 5 * time.Minute,
	}
	client.forcedRefreshServerNames = make(map[string]time.Time)

	client.forcedRefreshServerNames["bm-missing"] = now
	firstForcedAt := client.forcedRefreshServerNames["bm-missing"]

	now = now.Add(2 * time.Minute)
	client.forcedRefreshServerNames["bm-missing"] = now
	secondForcedAt := client.forcedRefreshServerNames["bm-missing"]

	require.True(t, secondForcedAt.After(firstForcedAt))

	now = now.Add(4 * time.Minute)
	require.True(t, client.nodeHasAlreadyForcedRefresh("bm-missing"))

	now = secondForcedAt.Add(client.timeout + time.Second)
	require.False(t, client.nodeHasAlreadyForcedRefresh("bm-missing"))
}

func TestServerGetListKeepsForcedRefreshNames(t *testing.T) {
	now := time.Date(2026, time.March, 16, 10, 0, 0, 0, time.UTC)
	robotClient := &mocks.RobotClient{}
	robotClient.On("ServerGetList").Return([]models.Server{
		{Name: "bm-existing", ServerNumber: 321},
	}, nil)

	client := &cacheRobotClient{
		robotClient: robotClient,
		now:         func() time.Time { return now },
		timeout:     time.Hour,
	}
	client.forcedRefreshServerNames = make(map[string]time.Time)
	client.forcedRefreshServerNames["bm-missing"] = now
	servers, err := client.ServerGetList()
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.True(t, client.nodeHasAlreadyForcedRefresh("bm-missing"))
	robotClient.AssertExpectations(t)
}

func writeCredentials(rootDir, user, password string) error {
	credentialsDir := credentials.GetDirectory(rootDir)
	newDir := filepath.Join(credentialsDir, "..dataNew")
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		return err
	}
	err := os.WriteFile(filepath.Join(newDir, "robot-user"),
		[]byte(user), 0o600)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(newDir, "robot-password"),
		[]byte(password), 0o600)
	if err != nil {
		return err
	}
	targetDir := filepath.Join(credentialsDir, "..data")
	if err := os.RemoveAll(targetDir); err != nil {
		return err
	}
	if err := os.Rename(newDir, targetDir); err != nil {
		return err
	}
	return nil
}
