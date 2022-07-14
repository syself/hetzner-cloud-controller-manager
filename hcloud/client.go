package hcloud

import (
	hcloud "github.com/hetznercloud/hcloud-go/hcloud"
	hrobot "github.com/syself/hrobot-go"
	clientset "k8s.io/client-go/kubernetes"
)

type client struct {
	cloudClient *hcloud.Client
	robotClient hrobot.RobotClient
	kubernetes  clientset.Interface
}

func newClient(hcloud *hcloud.Client, robotClient hrobot.RobotClient) *client {
	return &client{
		cloudClient: hcloud,
		robotClient: robotClient,
	}
}
