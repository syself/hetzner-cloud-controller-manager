package mocks

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/mock"
)

type NetworkClient struct {
	mock.Mock
}

func (m *NetworkClient) GetByID(ctx context.Context, id int64) (*hcloud.Network, *hcloud.Response, error) {
	args := m.Called(ctx, id)
	return getNetworkPtr(args, 0), getResponsePtr(args, 1), args.Error(2)
}
