package client

import "github.com/syself/hrobot-go/models"

// Client provides the Robot API operations used by this controller.
type Client interface {
	// ServerGet returns the Robot server with the given ID.
	ServerGet(id int) (*models.Server, error)

	// ServerGetList returns the cached or freshly loaded Robot server list.
	ServerGetList() ([]models.Server, error)

	// ServerGetListForceRefresh reloads the Robot server list immediately for
	// nodeName, bypassing the normal cache timeout unless that nodeName already
	// triggered a forced refresh within the current timeout window.
	ServerGetListForceRefresh(nodeName string) ([]models.Server, error)

	// SetCredentials updates the Robot client credentials.
	SetCredentials(username, password string) error
}
