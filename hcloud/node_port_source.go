package hcloud

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/syself/hetzner-cloud-controller-manager/internal/annotation"
	"github.com/syself/hetzner-cloud-controller-manager/internal/metrics"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

// restrictNodePortSourceEnabled reports whether the Service's NodePort should
// be limited to its Load Balancer. The annotation wins over the default from
// the environment.
func (l *loadBalancers) restrictNodePortSourceEnabled(svc *corev1.Service) (bool, error) {
	enabled, err := annotation.LBRestrictNodePortSource.BoolFromService(svc)
	if err == nil {
		return enabled, nil
	}
	if errors.Is(err, annotation.ErrNotSet) {
		return l.restrictNodePortSourceDefault, nil
	}
	return false, err
}

// lbSourceCIDRs returns the addresses the Load Balancer uses to connect to its
// targets, as /32 CIDRs. Nodes are reached over IPv4 even when clients use
// IPv6, so the Load Balancer's IPv6 address is not included.
func lbSourceCIDRs(lb *hcloud.LoadBalancer) []string {
	var cidrs []string
	if lb.PublicNet.Enabled {
		if ip := lb.PublicNet.IPv4.IP.To4(); ip != nil && !ip.IsUnspecified() {
			cidrs = append(cidrs, ip.String()+"/32")
		}
	}
	for _, nw := range lb.PrivateNet {
		if ip := nw.IP.To4(); ip != nil && !ip.IsUnspecified() {
			cidrs = append(cidrs, ip.String()+"/32")
		}
	}
	return cidrs
}

// desiredSourceRanges returns the source ranges after replacing the entries
// this controller added before (previouslyManaged) with the ones it wants now
// (wanted). Entries set by anyone else keep their place and are never removed.
func desiredSourceRanges(current, previouslyManaged, wanted []string) ([]string, bool) {
	result := make([]string, 0, len(current)+len(wanted))
	for _, c := range current {
		if slices.Contains(previouslyManaged, c) && !slices.Contains(wanted, c) {
			continue
		}
		result = append(result, c)
	}
	for _, w := range wanted {
		if !slices.Contains(result, w) {
			result = append(result, w)
		}
	}
	return result, !slices.Equal(current, result)
}

// ownedEntries returns the wanted entries this controller owns. An entry that
// someone else already listed stays theirs, so turning the feature off later
// does not remove it.
func ownedEntries(current, previouslyManaged, wanted []string) []string {
	var owned []string
	for _, w := range wanted {
		if slices.Contains(current, w) && !slices.Contains(previouslyManaged, w) {
			continue
		}
		owned = append(owned, w)
	}
	return owned
}

// managedSourceRanges reads the entries this controller added from the
// bookkeeping annotation.
func managedSourceRanges(svc *corev1.Service) []string {
	v, ok := annotation.LBNodePortSourceRanges.StringFromService(svc)
	if !ok || v == "" {
		return nil
	}
	var ranges []string
	for _, r := range strings.Split(v, ",") {
		if r = strings.TrimSpace(r); r != "" {
			ranges = append(ranges, r)
		}
	}
	return ranges
}

// reconcileNodePortSource limits the Service's NodePort to the Load Balancer
// by keeping the Load Balancer's IPs in spec.loadBalancerSourceRanges. When
// the feature is off for the Service, it removes the entries it added earlier.
func (l *loadBalancers) reconcileNodePortSource(ctx context.Context, svc *corev1.Service, lb *hcloud.LoadBalancer) error {
	const op = "hcloud/loadBalancers.reconcileNodePortSource"
	metrics.OperationCalled.WithLabelValues(op).Inc()

	enabled, err := l.restrictNodePortSourceEnabled(svc)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	var wanted []string
	if enabled {
		wanted = lbSourceCIDRs(lb)
		if len(wanted) == 0 {
			klog.InfoS("cannot restrict NodePort source: load balancer has no usable IPv4", "op", op,
				"service", svc.Name, "namespace", svc.Namespace, "loadBalancerID", lb.ID)
			return nil
		}
	}
	if err := l.setManagedSourceRanges(ctx, svc, wanted); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// removeNodePortSource removes the entries this controller added, for a
// Service whose Load Balancer is gone. A Service that changed to type NodePort
// would otherwise stay limited to an address that no longer exists.
func (l *loadBalancers) removeNodePortSource(ctx context.Context, svc *corev1.Service) error {
	const op = "hcloud/loadBalancers.removeNodePortSource"
	metrics.OperationCalled.WithLabelValues(op).Inc()

	if err := l.setManagedSourceRanges(ctx, svc, nil); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// setManagedSourceRanges makes wanted the set of entries this controller owns
// in spec.loadBalancerSourceRanges, and records it in the bookkeeping
// annotation. The cloud-provider service controller only patches status and
// metadata, so the spec change goes through a separate client.
func (l *loadBalancers) setManagedSourceRanges(ctx context.Context, svc *corev1.Service, wanted []string) error {
	if len(wanted) == 0 && len(managedSourceRanges(svc)) == 0 {
		return nil
	}
	if l.k8sClient == nil {
		klog.InfoS("cannot update NodePort source ranges: no Kubernetes client", "service", svc.Name, "namespace", svc.Namespace)
		return nil
	}

	services := l.k8sClient.CoreV1().Services(svc.Namespace)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest, err := services.Get(ctx, svc.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if latest.DeletionTimestamp != nil {
			return nil
		}

		previouslyManaged := managedSourceRanges(latest)
		ranges, changed := desiredSourceRanges(latest.Spec.LoadBalancerSourceRanges, previouslyManaged, wanted)
		managed := strings.Join(ownedEntries(latest.Spec.LoadBalancerSourceRanges, previouslyManaged, wanted), ",")
		current, hasAnnotation := annotation.LBNodePortSourceRanges.StringFromService(latest)
		annotationChanged := current != managed || (managed == "" && hasAnnotation)
		if !changed && !annotationChanged {
			return nil
		}

		updated := latest.DeepCopy()
		updated.Spec.LoadBalancerSourceRanges = ranges
		if managed == "" {
			delete(updated.Annotations, string(annotation.LBNodePortSourceRanges))
		} else if err := annotation.LBNodePortSourceRanges.AnnotateService(updated, managed); err != nil {
			return err
		}

		if _, err := services.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
			return err
		}
		klog.InfoS("updated NodePort source ranges", "service", svc.Name, "namespace", svc.Namespace,
			"old", latest.Spec.LoadBalancerSourceRanges, "new", ranges)
		return nil
	})
}
