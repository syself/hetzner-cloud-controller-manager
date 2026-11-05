package cache

import (
	"fmt"
	"net/http"
	"os"
	"sync"
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
	defaultCacheTimeout = 5 * time.Minute
)

var _ robotclient.Client = &cacheRobotClient{}

type cacheRobotClient struct {
	// mu guards every field below. The service, node and route controllers
	// call the client from several goroutines at once.
	mu sync.Mutex

	robotClient hrobot.RobotClient
	timeout     time.Duration

	lastUpdate time.Time
	now        func() time.Time

	// cache
	l []models.Server
	m map[int]*models.Server

	// forcedRefreshServerNames stores when a node name last triggered a forced Robot server list
	// refresh. While that timestamp is still within the cache timeout window, repeated lookups for
	// the same missing server name skip the extra uncached Robot API call.
	forcedRefreshServerNames map[string]time.Time
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
		cacheTimeout = defaultCacheTimeout
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
	handler.forcedRefreshServerNames = make(map[string]time.Time)
	return handler, nil
}

func (c *cacheRobotClient) ServerGet(id int) (*models.Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.syncIfNeeded(); err != nil {
		return nil, err
	}

	server, found := c.m[id]
	if !found {
		// return not found error
		return nil, models.Error{Code: models.ErrorCodeServerNotFound, Message: "server not found"}
	}

	return server, nil
}

func (c *cacheRobotClient) ServerGetList() ([]models.Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.syncIfNeeded(); err != nil {
		return nil, err
	}
	return c.l, nil
}

// ServerGetListForceRefresh invalidates the current cache and reloads the list
// from Robot unless nodeName already triggered a forced refresh within the
// current timeout window.
func (c *cacheRobotClient) ServerGetListForceRefresh(nodeName string) ([]models.Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	force := nodeName == "" || !c.nodeHasAlreadyForcedRefresh(nodeName)
	if force {
		// Drop the cached list, so the sync below calls the Robot API.
		c.m = nil
	}

	if err := c.syncIfNeeded(); err != nil {
		return nil, err
	}

	if force && nodeName != "" {
		if c.forcedRefreshServerNames == nil {
			c.forcedRefreshServerNames = make(map[string]time.Time)
		}
		c.forcedRefreshServerNames[nodeName] = c.currentTime()
	}

	return c.l, nil
}

// syncIfNeeded reloads the server list from Robot when the cache is empty or
// expired. The caller must hold c.mu, so concurrent callers share one Robot
// API call instead of each sending their own. The list and map are replaced,
// never modified, so callers may keep using what they got earlier.
func (c *cacheRobotClient) syncIfNeeded() error {
	if !c.shouldSync() {
		return nil
	}

	list, err := c.robotClient.ServerGetList()
	if err != nil {
		return err
	}

	m := make(map[int]*models.Server, len(list))
	for i, server := range list {
		m[server.ServerNumber] = &list[i]
	}

	c.l = list
	c.m = m
	c.lastUpdate = c.currentTime()
	return nil
}

// nodeHasAlreadyForcedRefresh reports whether nodeName already triggered a
// forced refresh within the current cache timeout window. The caller must hold
// c.mu.
func (c *cacheRobotClient) nodeHasAlreadyForcedRefresh(nodeName string) bool {
	if c.forcedRefreshServerNames == nil {
		return false
	}

	forcedAt, found := c.forcedRefreshServerNames[nodeName]
	if !found {
		return false
	}

	if c.currentTime().After(forcedAt.Add(c.forceRefreshTimeout())) {
		delete(c.forcedRefreshServerNames, nodeName)
		return false
	}

	return true
}
func (c *cacheRobotClient) shouldSync() bool {
	// map is nil means we have no cached value yet
	if c.m == nil {
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

func (c *cacheRobotClient) forceRefreshTimeout() time.Duration {
	if c.timeout == 0 {
		return defaultCacheTimeout
	}
	return c.timeout
}

func (c *cacheRobotClient) SetCredentials(username, password string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.robotClient.SetCredentials(username, password)
	if err != nil {
		return err
	}
	// The credentials have been updated, so we need to invalidate the cache.
	c.m = nil
	c.forcedRefreshServerNames = make(map[string]time.Time)
	return nil
}
