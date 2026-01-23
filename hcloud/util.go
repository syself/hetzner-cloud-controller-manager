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
	"regexp"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/syself/hetzner-cloud-controller-manager/internal/hcops"
	"github.com/syself/hetzner-cloud-controller-manager/internal/metrics"
	robotclient "github.com/syself/hetzner-cloud-controller-manager/internal/robot/client"
	"github.com/syself/hrobot-go/models"
	corev1 "k8s.io/api/core/v1"
)

func getHCloudServerByName(ctx context.Context, c *hcloud.Client, name string) (*hcloud.Server, error) {
	const op = "hcloud/getServerByName"
	metrics.OperationCalled.WithLabelValues(op).Inc()

	server, _, err := c.Server.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return server, nil
}

func getHCloudServerByID(ctx context.Context, c *hcloud.Client, id int64) (*hcloud.Server, error) {
	const op = "hcloud/getServerByID"
	metrics.OperationCalled.WithLabelValues(op).Inc()

	server, _, err := c.Server.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return server, nil
}

func getRobotServerByName(c robotclient.Client, node *corev1.Node) (server *models.Server, err error) {
	const op = "robot/getServerByName"

	if c == nil {
		return nil, errMissingRobotCredentials
	}

	// check for rate limit
	if hcops.IsRateLimitExceeded(node) {
		return nil, fmt.Errorf("%s: rate limit exceeded - next try at %q", op, hcops.TimeOfNextPossibleAPICall().String())
	}

	serverList, err := c.ServerGetList()
	if err != nil {
		hcops.HandleRateLimitExceededError(err, node)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	for i, s := range serverList {
		if s.Name == node.Name {
			server = &serverList[i]
		}
	}

	return server, nil
}

func getRobotServerByID(c robotclient.Client, id int, node *corev1.Node) (s *models.Server, e error) {
	const op = "robot/getServerByID"
	if node.Name == "" {
		return nil, fmt.Errorf("%s: node name is empty", op)
	}

	if c == nil {
		return nil, errMissingRobotCredentials
	}

	// check for rate limit
	if hcops.IsRateLimitExceeded(node) {
		return nil, fmt.Errorf("%s: rate limit exceeded - next try at %q", op, hcops.TimeOfNextPossibleAPICall().String())
	}

	server, err := c.ServerGet(id)
	if models.IsError(err, models.ErrorCodeServerNotFound) {
		return nil, nil
	}
	if err != nil {
		hcops.HandleRateLimitExceededError(err, node)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// check whether name matches - otherwise this server does not belong to the respective node anymore
	if server == nil {
		return nil, nil
	}
	if server.Name != node.Name {
		return nil, nil
	}

	// return nil, nil if server could not be found
	return server, nil
}

func isHCloudServerByName(name string) bool {
	return !strings.HasPrefix(name, hostNamePrefixRobot)
}

func getInstanceTypeOfRobotServer(bmServer *models.Server) string {
	if bmServer == nil {
		panic("getInstanceTypeOfRobotServer called with nil server")
	}
	return stringToLabelValue(bmServer.Product)
}

var stringToLabelValueRegex = regexp.MustCompile(`[^a-zA-Z0-9_.]+`)

func stringToLabelValue(s string) string {
	s = stringToLabelValueRegex.ReplaceAllString(s, "-")
	trimChars := "_.-"
	s = strings.Trim(s, trimChars)
	return s
}

func getZoneOfRobotServer(bmServer *models.Server) string {
	if bmServer == nil {
		panic("getZoneOfRobotServer called with nil server")
	}
	return strings.ToLower(bmServer.Dc[:min(4, len(bmServer.Dc))])
}

func getRegionOfRobotServer(bmServer *models.Server) string {
	if bmServer == nil {
		panic("getZoneOfRobotServer called with nil server")
	}
	zoneToRegionMap := map[string]string{
		"nbg1": "eu-central",
		"fsn1": "eu-central",
		"hel1": "eu-central",
		"ash":  "us-east",
	}
	zone := getZoneOfRobotServer(bmServer)
	region, found := zoneToRegionMap[zone]
	if !found {
		panic("zoneToRegionMap: unknown zone")
	}
	return region
}
