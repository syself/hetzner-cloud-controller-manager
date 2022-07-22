/*
Copyright 2018 Hetzner Cloud GmbH.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hcloud

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/syself/hrobot-go/models"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	cloudproviderapi "k8s.io/cloud-provider/api"
	"k8s.io/klog/v2"
)

type addressFamily int

const (
	AddressFamilyDualStack addressFamily = iota
	AddressFamilyIPv6
	AddressFamilyIPv4
)

type instances struct {
	client        *client
	addressFamily addressFamily
}

func newInstances(client *client, addressFamily addressFamily) *instances {
	return &instances{client, addressFamily}
}

func (i *instances) NodeAddressesByProviderID(ctx context.Context, providerID string) ([]v1.NodeAddress, error) {
	const op = "hcloud/instances.NodeAddressesByProviderID"

	id, isHCloudServer, err := providerIDToServerID(providerID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	klog.V(4).Info("Called ", op, " providerID=", providerID, " isHCloudServer=", isHCloudServer)

	if isHCloudServer {
		server, err := getHCloudServerByID(ctx, i.client.cloudClient, id)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		return i.hcloudNodeAddresses(ctx, server), nil
	}

	server, err := getRobotServerByID(ctx, i.client.robotClient, id)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return i.robotNodeAddresses(ctx, server), nil
}

func (i *instances) NodeAddresses(ctx context.Context, nodeName types.NodeName) ([]v1.NodeAddress, error) {
	const op = "hcloud/instances.NodeAddresses"

	klog.V(4).Info("Called ", op)

	hserver, err := getHCloudServerByName(ctx, i.client.cloudClient, string(nodeName))
	if err == nil {
		return i.hcloudNodeAddresses(ctx, hserver), nil
	} else if err != cloudprovider.InstanceNotFound {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	rserver, err := getRobotServerByName(ctx, i.client.robotClient, string(nodeName))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return i.robotNodeAddresses(ctx, rserver), nil
}

func (i *instances) ExternalID(ctx context.Context, nodeName types.NodeName) (string, error) {
	const op = "hcloud/instances.ExternalID"

	klog.V(4).Info("Called ", op)

	id, err := i.InstanceID(ctx, nodeName)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	return id, nil
}

func (i *instances) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	const op = "hcloud/instances.InstanceID"

	klog.V(4).Info("Called ", op)

	if isHCloudServerByName(string(nodeName)) {
		server, err := getHCloudServerByName(ctx, i.client.cloudClient, string(nodeName))
		if err != nil {
			return "", fmt.Errorf("%s: %w", op, err)
		}
		return strconv.Itoa(server.ID), nil
	}

	server, err := getRobotServerByName(ctx, i.client.robotClient, string(nodeName))
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return hostNamePrefixRobot + strconv.Itoa(server.ServerNumber), nil
}

func (i *instances) InstanceType(ctx context.Context, nodeName types.NodeName) (string, error) {
	const op = "hcloud/instances.InstanceType"

	klog.V(4).Info("Called ", op)

	if isHCloudServerByName(string(nodeName)) {
		server, err := getHCloudServerByName(ctx, i.client.cloudClient, string(nodeName))
		if err != nil {
			return "", fmt.Errorf("%s: %w", op, err)
		}
		return server.ServerType.Name, nil
	}

	server, err := getRobotServerByName(ctx, i.client.robotClient, string(nodeName))
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	return server.Product, nil
}

func (i *instances) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	const op = "hcloud/instances.InstanceTypeByProviderID"

	klog.V(4).Info("Called ", op)

	id, isHCloudServer, err := providerIDToServerID(providerID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	if isHCloudServer {
		server, err := getHCloudServerByID(ctx, i.client.cloudClient, id)
		if err != nil {
			return "", fmt.Errorf("%s: %w", op, err)
		}
		return server.ServerType.Name, nil
	}

	server, err := getRobotServerByID(ctx, i.client.robotClient, id)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	return server.Product, nil
}

func (i *instances) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	return cloudprovider.NotImplemented
}

func (i *instances) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	return types.NodeName(hostname), nil
}

func (i instances) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	const op = "hcloud/instances.InstanceExistsByProviderID"

	klog.V(4).Info("Called ", op)

	id, isHCloudServer, err := providerIDToServerID(providerID)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	if isHCloudServer {
		server, _, err := i.client.cloudClient.Server.GetByID(ctx, id)
		if err != nil {
			return false, fmt.Errorf("%s: %w", op, err)
		}
		return server != nil, nil
	}

	if i.client.robotClient == nil {
		return false, errMissingRobotCredentials
	}

	klog.Infof("%s: calling robot API to get server with provider_id %v", op, providerID)

	server, err := i.client.robotClient.ServerGet(id)
	if err != nil {
		if models.IsError(err, models.ErrorCodeNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("%s: %w", op, err)
	}
	return isRobotServerInCluster(server.Name), nil
}

func (i instances) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	const op = "hcloud/instances.InstanceShutdownByProviderID"

	id, isHCloudServer, err := providerIDToServerID(providerID)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	if isHCloudServer {
		server, _, err := i.client.cloudClient.Server.GetByID(ctx, id)
		if err != nil {
			return false, fmt.Errorf("%s: %w", op, err)
		}
		return server != nil && server.Status == hcloud.ServerStatusOff, nil
	}

	// Robot does not support shutdowns
	return false, nil
}

func (i *instances) hcloudNodeAddresses(ctx context.Context, server *hcloud.Server) []v1.NodeAddress {
	var addresses []v1.NodeAddress
	addresses = append(
		addresses,
		v1.NodeAddress{Type: v1.NodeHostName, Address: server.Name},
	)

	if i.addressFamily == AddressFamilyIPv4 || i.addressFamily == AddressFamilyDualStack {
		addresses = append(
			addresses,
			v1.NodeAddress{Type: v1.NodeExternalIP, Address: server.PublicNet.IPv4.IP.String()},
		)
	}

	if i.addressFamily == AddressFamilyIPv6 || i.addressFamily == AddressFamilyDualStack {
		// For a given IPv6 network of 2001:db8:1234::/64, the instance address is 2001:db8:1234::1
		host_address := server.PublicNet.IPv6.IP
		host_address[len(host_address)-1] |= 0x01

		addresses = append(
			addresses,
			v1.NodeAddress{Type: v1.NodeExternalIP, Address: host_address.String()},
		)
	}

	n := os.Getenv(hcloudNetworkENVVar)
	if len(n) > 0 {
		network, _, _ := i.client.cloudClient.Network.Get(ctx, n)
		if network != nil {
			for _, privateNet := range server.PrivateNet {
				if privateNet.Network.ID == network.ID {
					addresses = append(
						addresses,
						v1.NodeAddress{Type: v1.NodeInternalIP, Address: privateNet.IP.String()},
					)
				}
			}

		}
	}
	return addresses
}

func (i *instances) robotNodeAddresses(ctx context.Context, server *models.Server) []v1.NodeAddress {
	var addresses []v1.NodeAddress
	addresses = append(
		addresses,
		v1.NodeAddress{Type: v1.NodeHostName, Address: server.Name},
	)

	if i.client.kubernetes != nil {
		if node, _ := i.client.kubernetes.CoreV1().Nodes().Get(context.TODO(), server.Name, metav1.GetOptions{}); node != nil {
			providedIP, ok := node.ObjectMeta.Annotations[cloudproviderapi.AnnotationAlphaProvidedIPAddr]
			if ok {
				addresses = append(
					addresses,
					v1.NodeAddress{Type: v1.NodeInternalIP, Address: providedIP},
				)
			}
		}
	}

	if i.addressFamily == AddressFamilyIPv4 || i.addressFamily == AddressFamilyDualStack {
		addresses = append(
			addresses,
			v1.NodeAddress{Type: v1.NodeExternalIP, Address: server.ServerIP},
		)
	}

	if i.addressFamily == AddressFamilyIPv6 || i.addressFamily == AddressFamilyDualStack {
		// For a given IPv6 network of 2a01:f48:111:4221::, the instance address is 2a01:f48:111:4221::1
		host_address := server.ServerIPv6Net
		host_address = host_address + "1"

		addresses = append(
			addresses,
			v1.NodeAddress{Type: v1.NodeExternalIP, Address: host_address},
		)
	}

	klog.V(4).Info("robotNodeAddresses addresses=", addresses)

	return addresses
}
