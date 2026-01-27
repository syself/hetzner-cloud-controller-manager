package providerid

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	// prefixCloud is the prefix for Cloud Server provider IDs.
	//
	// It MUST not be changed, otherwise existing nodes will not be recognized anymore.
	prefixCloud = "hcloud://"

	// prefixRobotLegacy is the prefix used by the Syself Fork for Robot Server provider IDs.
	// This Prefix is no longer used for new nodes, instead prefix "hrobot://" should be used.
	//
	// It MUST not be changed, otherwise existing nodes will not be recognized anymore.
	prefixRobotLegacy = "hcloud://bm-"

	prefixRobotNew = "hrobot://"

	hetznerBMProviderIDPrefixAnnotation = "node.cluster.x-k8s.io/hetzner-bm-provider-id-prefix"
)

type UnkownPrefixError struct {
	ProviderID string
}

func (e *UnkownPrefixError) Error() string {
	return fmt.Sprintf(
		"Provider ID does not have one of the the expected prefixes (%s, %s): %s",
		prefixCloud,
		prefixRobotLegacy,
		e.ProviderID,
	)
}

// ToServerID parses the Cloud or Robot Server ID from a ProviderID.
//
// This method supports all formats for the ProviderID that were ever used.
// If a format is ever dropped from this method the Nodes that still use that
// format will get abandoned and can no longer be processed by HCCM.
func ToServerID(providerID string) (id int64, isCloudServer bool, err error) {
	idString := ""
	switch {
	case strings.HasPrefix(providerID, prefixRobotNew):
		// If a cluster switched from old-syself-ccm to upstream-hcloud-ccm, and then back again to
		// old-syself-ccm, then there might be nodes with the new format. Let's support this
		// edge-case, but in the long run the upstream-hcloud-ccm should be used. Related:
		// https://github.com/syself/cluster-api-provider-hetzner/pull/1703
		idString = strings.ReplaceAll(providerID, prefixRobotNew, "")

	case strings.HasPrefix(providerID, prefixRobotLegacy):
		// This case needs to be before [prefixCloud], as [prefixCloud] is a superset of [prefixRobotLegacy]
		idString = strings.ReplaceAll(providerID, prefixRobotLegacy, "")

	case strings.HasPrefix(providerID, prefixCloud):
		isCloudServer = true
		idString = strings.ReplaceAll(providerID, prefixCloud, "")

	default:
		return 0, false, &UnkownPrefixError{providerID}
	}

	if idString == "" {
		return 0, false, fmt.Errorf("providerID is missing a serverID: %s", providerID)
	}

	id, err = strconv.ParseInt(idString, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("unable to parse server id: %s", providerID)
	}
	return id, isCloudServer, nil
}

// FromCloudServerID generates the canonical ProviderID for a Cloud Server.
func FromCloudServerID(serverID int64) string {
	return fmt.Sprintf("%s%d", prefixCloud, serverID)
}

func GetProviderId(node *corev1.Node, serverNumber int) (string, error) {
	if node.Spec.ProviderID != "" {
		return node.Spec.ProviderID, nil
	}
	prefix, ok := node.Annotations[hetznerBMProviderIDPrefixAnnotation]
	if !ok {
		prefix = prefixRobotLegacy
	}
	if prefix != prefixRobotLegacy && prefix != prefixRobotNew {
		return "", fmt.Errorf(
			"Value %q of node (%s) annotation %s is invalid. Ony %q and %q are supported",
			prefix,
			node.Name,
			hetznerBMProviderIDPrefixAnnotation,
			prefixRobotLegacy,
			prefixRobotNew,
		)
	}
	return fmt.Sprintf("%s%d", prefix, serverNumber), nil
}
