package cache

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/syself/hetzner-cloud-controller-manager/internal/credentials"
	robotclient "github.com/syself/hetzner-cloud-controller-manager/internal/robot/client"
	"github.com/syself/hetzner-cloud-controller-manager/internal/util"
	hrobot "github.com/syself/hrobot-go"
	"github.com/syself/hrobot-go/models"
	"k8s.io/klog/v2"
)

const (
	robotUserNameENVVar = "ROBOT_USER_NAME"
	robotPasswordENVVar = "ROBOT_PASSWORD"
	cacheTimeoutENVVar  = "CACHE_TIMEOUT"

	// We don't want to remember a node name for ever. Imagine a bm machine with constant hostname
	// gets provisioned twice. Then we want the second time to work like the first: The cache of
	// Robot servers should be updated via ServerGetListForceRefresh().
	defaultMissingServerNameTTL = 5 * time.Minute
)

var _ robotclient.Client = &cacheRobotClient{}

type cacheRobotClient struct {
	robotClient hrobot.RobotClient
	timeout     time.Duration

	lastUpdate time.Time
	now        func() time.Time

	// cache
	l                  []models.Server
	m                  map[int]*models.Server
	missingServerNames map[string]time.Time
}

// NewCachedRobotClient creates a new robot client with caching enabled.
// rootDir: root directory for reading credentials from file.
// httpClient: http client to use for the robot client.
// baseURL: base URL for the robot client. Optional, leave empty for default.
// Returns nil and no error if the robot client could not be created, because
// the credentials are optional.
func NewCachedRobotClient(rootDir string, httpClient *http.Client, baseURL string) (robotclient.Client, error) {
	const op = "hcloud/newRobotClient"

	if httpClient == nil {
		return nil, fmt.Errorf("%s: httpClient is nil", op)
	}
	cacheTimeout, err := util.GetEnvDuration(cacheTimeoutENVVar)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if cacheTimeout == 0 {
		cacheTimeout = 5 * time.Minute
	}

	credentialsDir := credentials.GetDirectory(rootDir)
	_, err = os.Stat(credentialsDir)
	var robotUser, robotPassword string
	if err != nil {
		klog.V(1).Infof("reading Hetzner Robot credentials from file failed. %q does not exist", credentialsDir)
		robotUser = os.Getenv(robotUserNameENVVar)
		robotPassword = os.Getenv(robotPasswordENVVar)
		if robotUser == "" || robotPassword == "" {
			klog.Infof("Hetzner robot is not support because of insufficient credentials: Env vars (%q, %q) not set, and from file failed: %s",
				robotUserNameENVVar, robotPasswordENVVar,
				err.Error())
			return nil, nil
		}
	} else {
		robotUser, robotPassword, err = credentials.GetInitialRobotCredentials(credentialsDir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		if robotUser == "" || robotPassword == "" {
			klog.Infof("Hetzner robot is not supported because of insufficient credentials: credentials file exists but username or password is empty")
			return nil, nil
		}
	}
	c := hrobot.NewBasicAuthClientWithCustomHttpClient(robotUser, robotPassword, httpClient)
	if baseURL != "" {
		c.SetBaseURL(baseURL)
	}

	handler := &cacheRobotClient{}
	handler.timeout = cacheTimeout
	handler.robotClient = c
	handler.now = time.Now
	return handler, nil
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
		c.lastUpdate = c.currentTime()
		c.missingServerNames = nil
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
		c.lastUpdate = c.currentTime()
		c.missingServerNames = nil
	}

	return c.l, nil
}

// ServerGetListForceRefresh invalidates the current cache and reloads the list from Robot.
func (c *cacheRobotClient) ServerGetListForceRefresh() ([]models.Server, error) {
	c.m = nil
	return c.ServerGetList()
}

// HasMissingServerName reports whether name is still cached as a recent miss.
func (c *cacheRobotClient) HasMissingServerName(name string) bool {
	if c.missingServerNames == nil {
		return false
	}

	expiresAt, found := c.missingServerNames[name]
	if !found {
		return false
	}
	if c.currentTime().After(expiresAt) {
		delete(c.missingServerNames, name)
		return false
	}
	return true
}

// RememberMissingServerName stores name in the cache of recent misses until its
// expiration time is reached.
func (c *cacheRobotClient) RememberMissingServerName(name string) {
	if c.missingServerNames == nil {
		c.missingServerNames = make(map[string]time.Time)
	}
	c.missingServerNames[name] = c.currentTime().Add(defaultMissingServerNameTTL)
}

func (c *cacheRobotClient) shouldSync() bool {
	// map is nil means we have no cached value yet
	if c.m == nil {
		c.m = make(map[int]*models.Server)
		return true
	}
	if c.currentTime().After(c.lastUpdate.Add(c.timeout)) {
		return true
	}
	return false
}

func (c *cacheRobotClient) currentTime() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

func (c *cacheRobotClient) SetCredentials(username, password string) error {
	err := c.robotClient.SetCredentials(username, password)
	if err != nil {
		return err
	}
	// The credentials have been updated, so we need to invalidate the cache.
	c.m = nil
	c.missingServerNames = nil
	return nil
}
