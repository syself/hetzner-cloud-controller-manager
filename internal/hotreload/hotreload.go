package hotreload

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	fsnotify "github.com/fsnotify/fsnotify"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	robotclient "github.com/syself/hetzner-cloud-controller-manager/internal/robot/client"
	"k8s.io/klog/v2"
)

func Watch(hetznerSecretDirectory string, robotClient robotclient.Client, hcloudClient *hcloud.Client) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		klog.Fatal(err)
	}

	go func() {
		for {
			fmt.Printf("################################# \n")

			select {
			case event := <-watcher.Events:
				fmt.Printf("################################# %s\n", event.String())
				if !isValidEvent(event) {
					continue
				}
				klog.Infof("Secret file changed: %s", event.String())
				baseName := filepath.Base(event.Name)
				switch baseName {
				case "robot-user":
					LoadRobotCredentials(hetznerSecretDirectory, robotClient)
				case "robot-password":
					LoadRobotCredentials(hetznerSecretDirectory, robotClient)
				case "hcloud":
					LoadHcloudCredentials(hetznerSecretDirectory, hcloudClient)
				}
			case err := <-watcher.Errors:
				fmt.Printf("eeeeeeeeee################################# \n")

				klog.Infof("error: %s", err)
			}
		}
	}()

	err = watcher.Add(hetznerSecretDirectory)
	if err != nil {
		return fmt.Errorf("watcher.Add: %w", err)
	}
	return nil
}

func isValidEvent(event fsnotify.Event) bool {
	return event.Op&fsnotify.Write == fsnotify.Write
}

var (
	oldRobotUser       string
	oldRobotPassword   string
	RobotReloadCounter uint64
	robotMutex         sync.Mutex
)

func LoadRobotCredentials(hetznerSecretDirectory string, robotClient robotclient.Client) error {
	robotMutex.Lock()
	defer robotMutex.Unlock()
	username, password, err := ReadRobotCredentialsFromDirectory(hetznerSecretDirectory)
	if err != nil {
		return fmt.Errorf("reading robot credentials from secret: %w", err)
	}
	if username == oldRobotUser && password == oldRobotPassword {
		klog.Info("Hetzner Robot credentials unchanged")
		return nil
	}
	oldRobotUser = username
	oldRobotPassword = password
	atomic.AddUint64(&RobotReloadCounter, 1)
	err = robotClient.SetCredentials(username, password)
	if err != nil {
		return fmt.Errorf("SetCredentials: %w", err)
	}
	klog.Infof("Hetzner Robot credentials updated to new value: %q %s...", username, password[:3])
	return nil
}

func ReadRobotCredentialsFromDirectory(hetznerSecretDirectory string) (username string, password string, err error) {
	robotUserNameFile := filepath.Join(hetznerSecretDirectory, "robot-user")
	robotPasswordFile := filepath.Join(hetznerSecretDirectory, "robot-password")
	u, err := os.ReadFile(robotUserNameFile)
	if err != nil {
		return "", "", fmt.Errorf("reading robot user name from %q: %w", robotUserNameFile, err)
	}
	p, err := os.ReadFile(robotPasswordFile)
	if err != nil {
		return "", "", fmt.Errorf("reading robot password from %q: %w", robotPasswordFile, err)
	}
	return strings.TrimSpace(string(u)), strings.TrimSpace(string(p)), nil
}

var (
	oldHcloudToken           string
	HcloudTokenReloadCounter uint64
	hcloudMutex              sync.Mutex
)

func LoadHcloudCredentials(hetznerSecretDirectory string, hcloudClient *hcloud.Client) error {
	hcloudMutex.Lock()
	defer hcloudMutex.Unlock()
	op := "hcloud/updateHcloudToken"
	token, err := ReadHcloudCredentialsFromDirectory(hetznerSecretDirectory)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if len(token) != 64 {
		return fmt.Errorf("%s: entered token is invalid (must be exactly 64 characters long)", op)
	}
	if token == oldHcloudToken {
		klog.Info("Hetzner Cloud token unchanged")
		return nil
	}
	oldHcloudToken = token
	atomic.AddUint64(&HcloudTokenReloadCounter, 1)
	hcloud.WithToken(token)(hcloudClient)
	klog.Infof("Hetzner Cloud token updated to new value: %s...", token[:5])
	return nil
}

func ReadHcloudCredentialsFromDirectory(hetznerSecretDirectory string) (string, error) {
	hcloudTokenFile := filepath.Join(hetznerSecretDirectory, "hcloud")
	data, err := os.ReadFile(hcloudTokenFile)
	if err != nil {
		return "", fmt.Errorf("reading hcloud token from %q: %w", hcloudTokenFile, err)
	}
	return strings.TrimSpace(string(data)), nil
}
