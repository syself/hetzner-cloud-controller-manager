package hotreload

import (
	"errors"
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

// Watch the mounted secrets. Reload the credentials, when the files get updated. The robotClient can be nil.
func Watch(hetznerSecretDirectory string, hcloudClient *hcloud.Client, robotClient robotclient.Client) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		klog.Fatal(err)
	}

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if !isValidEvent(event) {
					continue
				}
				baseName := filepath.Base(event.Name)
				var err error
				switch baseName {
				case "robot-user":
					err = LoadRobotCredentials(hetznerSecretDirectory, robotClient)
				case "robot-password":
					err = LoadRobotCredentials(hetznerSecretDirectory, robotClient)
				case "hcloud":
					err = LoadHcloudCredentials(hetznerSecretDirectory, hcloudClient)
				case "..data":
					// The files (for example hcloud) are symlinks to ..data/. For example to ../data/hcloud
					// This means the files/symlinks don't change. When the secrets get changed, then
					// a new ..data directory gets created. This is done by Kubernetes to make the
					// update atomic.
					err = LoadHcloudCredentials(hetznerSecretDirectory, hcloudClient)
					if robotClient != nil {
						err = errors.Join(err, LoadRobotCredentials(hetznerSecretDirectory, robotClient))
					}
				default:
					klog.Infof("Ignoring fsnotify event for %q: %s", baseName, event.String())
				}
				if err != nil {
					klog.Errorf("error processing fsnotify event: %s", err.Error())
					continue
				}

			case err := <-watcher.Errors:
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
	baseName := filepath.Base(event.Name)
	if strings.HasPrefix(baseName, "..") && baseName != "..data" {
		// Skip ..data_tmp and ..YYYY_MM_DD...
		return false
	}
	if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
		return true
	}
	return false
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

func ReadRobotCredentialsFromDirectory(hetznerSecretDirectory string) (username, password string, err error) {
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
