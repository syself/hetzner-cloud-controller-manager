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
	"github.com/syself/hetzner-cloud-controller-manager/internal/hotreload"
	"github.com/syself/hrobot-go/models"
)

func Test_updateRobotCredentials(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	os.Unsetenv(robotUserNameENVVar)
	os.Unsetenv(robotPasswordENVVar)

	tmp, err := os.MkdirTemp("", "Test_newHcloudClient-*")
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(tmp, "etc", "hetzner-secret"), 0o755)
	require.NoError(t, err)

	err = os.Symlink("..data/robot-user", filepath.Join(tmp, "etc", "hetzner-secret", "robot-user"))
	require.NoError(t, err)

	err = os.Symlink("..data/robot-password", filepath.Join(tmp, "etc", "hetzner-secret", "robot-password"))
	require.NoError(t, err)

	err = writeCredentials(tmp, "my-robot-user", "my-robot-password")
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
	robotClient, err := NewCachedRobotClient(tmp, httpClient, server.URL+"/robot")
	require.NoError(t, err)
	require.NotNil(t, robotClient)
	err = hotreload.Watch(filepath.Join(tmp, "etc", "hetzner-secret"), nil, robotClient)
	require.NoError(t, err)
	servers, err := robotClient.ServerGetList()
	require.NoError(t, err)
	require.Len(t, servers, 1)

	oldCount := hotreload.GetRobotReloadCounter()
	err = writeCredentials(tmp, "user2", "password2")
	require.NoError(t, err)
	start := time.Now()
	for {
		// if hotreload.robotReloadCounter > oldCount {
		if hotreload.GetRobotReloadCounter() > oldCount {
			break
		}
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

func writeCredentials(tmpDir, user, password string) error {
	newDir := filepath.Join(tmpDir, "etc", "hetzner-secret", "..dataNew")
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
	targetDir := filepath.Join(tmpDir, "etc", "hetzner-secret", "..data")
	if err := os.RemoveAll(targetDir); err != nil {
		return err
	}
	if err := os.Rename(newDir, targetDir); err != nil {
		return err
	}
	return nil
}
