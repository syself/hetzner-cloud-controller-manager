package client

import "github.com/syself/hrobot-go/models"

// Client provides the Robot API operations used by this controller.
type Client interface {
	// ServerGet returns the Robot server with the given ID.
	ServerGet(id int) (*models.Server, error)

	// ServerGetList returns the cached or freshly loaded Robot server list.
	ServerGetList() ([]models.Server, error)

	// ServerGetListForceRefresh reloads the Robot server list immediately,
	// bypassing the normal cache timeout.
	ServerGetListForceRefresh() ([]models.Server, error)

	// NodeHasAlreadyForcedRefresh reports whether nodeName already triggered a
	// forced Robot list refresh within the current timeout window.
	NodeHasAlreadyForcedRefresh(nodeName string) bool

	// NodeTriggeredForcedRefresh records that nodeName already triggered a
	// forced Robot list refresh.
	NodeTriggeredForcedRefresh(nodeName string)

	// SetCredentials updates the Robot client credentials.
	SetCredentials(username, password string) error
}
