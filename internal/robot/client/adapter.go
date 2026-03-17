package client

import (
	hrobot "github.com/syself/hrobot-go"
	"github.com/syself/hrobot-go/models"
)

var _ Client = &adapter{}

type adapter struct {
	hrobot.RobotClient
}

// New wraps a plain Robot client so it satisfies the local Client interface.
func New(robotClient hrobot.RobotClient) Client {
	if robotClient == nil {
		return nil
	}
	return &adapter{RobotClient: robotClient}
}

// ServerGetListForceRefresh falls back to the plain list call because the
// uncached behavior only exists in the cache-backed client implementation.
func (a *adapter) ServerGetListForceRefresh(_ string) ([]models.Server, error) {
	return a.ServerGetList()
}
