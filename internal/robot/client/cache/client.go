package cache

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/syself/hetzner-cloud-controller-manager/internal/hotreload"
	robotclient "github.com/syself/hetzner-cloud-controller-manager/internal/robot/client"
	"github.com/syself/hetzner-cloud-controller-manager/internal/util"
	hrobot "github.com/syself/hrobot-go"
	"github.com/syself/hrobot-go/models"
	"k8s.io/klog/v2"
)

const (
	robotDebugENVVar    = "ROBOT_DEBUG"
	robotUserNameENVVar = "ROBOT_USER_NAME"
	robotPasswordENVVar = "ROBOT_PASSWORD"
	cacheTimeoutENVVar  = "CACHE_TIMEOUT"
)

var handler = &cacheRobotClient{}

var _ robotclient.Client = handler

type cacheRobotClient struct {
	robotClient hrobot.RobotClient
	timeout     time.Duration

	lastUpdate time.Time

	// cache
	l []models.Server
	m map[int]*models.Server
}

func NewClient(robotClient hrobot.RobotClient, cacheTimeout time.Duration) robotclient.Client {
	handler.timeout = cacheTimeout
	handler.robotClient = robotClient
	return handler
}

// NewCachedRobotClient creates a new robot client with caching enabled.
// rootDir: root directory for reading credentials from file.
// httpClient: http client to use for the robot client.
// baseURL: base URL for the robot client. Optional, leave empty for default.
func NewCachedRobotClient(rootDir string, httpClient *http.Client, baseURL string) (robotclient.Client, error) {
	const op = "hcloud/newRobotClient"
	cacheTimeout, err := util.GetEnvDuration(cacheTimeoutENVVar)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if cacheTimeout == 0 {
		cacheTimeout = 5 * time.Minute
	}

	secretsDir := filepath.Join(rootDir, "etc", "hetzner-secret")
	_, err = os.Stat(secretsDir)
	var robotUser, robotPassword string
	if err != nil {
		klog.V(1).Infof("reading Hetzner Robot credentials from file failed. %q does not exist", secretsDir)
		robotUser = os.Getenv(robotUserNameENVVar)
		robotPassword = os.Getenv(robotPasswordENVVar)
		if robotUser == "" || robotPassword == "" {
			klog.Infof("Hetzner robot is not support because of insufficient credentials: Env vars (%q, %q) not set, and from file failed: %s",
				robotUserNameENVVar, robotPasswordENVVar,
				err.Error())
			return nil, nil
		}
	} else {
		robotUser, robotPassword, err = hotreload.ReadRobotCredentialsFromDirectory(secretsDir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	}
	c := hrobot.NewBasicAuthClientWithCustomHttpClient(robotUser, robotPassword, httpClient)
	if baseURL != "" {
		c.SetBaseURL(baseURL)
	}
	robotClient := NewClient(c, cacheTimeout)
	return robotClient, nil
}

func (c *cacheRobotClient) ServerGet(id int) (*models.Server, error) {
	if c.shouldSync() {
		list, err := c.robotClient.ServerGetList()
		if err != nil {
			return nil, err
		}

		// populate list
		c.l = list

		// remove all entries from map and populate it freshly
		c.m = make(map[int]*models.Server)
		for i, server := range list {
			c.m[server.ServerNumber] = &list[i]
		}

		// set time of last update
		c.lastUpdate = time.Now()
	}

	server, found := c.m[id]
	if !found {
		// return not found error
		return nil, models.Error{Code: models.ErrorCodeServerNotFound, Message: "server not found"}
	}

	return server, nil
}

func (c *cacheRobotClient) ServerGetList() ([]models.Server, error) {
	if c.shouldSync() {
		list, err := c.robotClient.ServerGetList()
		if err != nil {
			return list, err
		}

		// populate list
		c.l = list

		// remove all entries from map and populate it freshly
		c.m = make(map[int]*models.Server)
		for i, server := range list {
			c.m[server.ServerNumber] = &list[i]
		}

		// set time of last update
		c.lastUpdate = time.Now()
	}

	return c.l, nil
}

func (c *cacheRobotClient) shouldSync() bool {
	// map is nil means we have no cached value yet
	if c.m == nil {
		c.m = make(map[int]*models.Server)
		return true
	}
	if time.Now().After(c.lastUpdate.Add(c.timeout)) {
		return true
	}
	return false
}

func (c *cacheRobotClient) SetCredentials(username, password string) error {
	err := c.robotClient.SetCredentials(username, password)
	if err != nil {
		return err
	}
	c.m = nil
	return nil
}
