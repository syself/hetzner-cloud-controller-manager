package hcloud

import (
	"context"
	"net"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syself/hetzner-cloud-controller-manager/internal/annotation"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestDesiredSourceRanges(t *testing.T) {
	tests := []struct {
		name       string
		current    []string
		managed    []string
		wanted     []string
		expected   []string
		expChanged bool
	}{
		{
			name:       "empty field gets the load balancer",
			wanted:     []string{"1.2.3.4/32"},
			expected:   []string{"1.2.3.4/32"},
			expChanged: true,
		},
		{
			name:       "already present",
			current:    []string{"1.2.3.4/32"},
			managed:    []string{"1.2.3.4/32"},
			wanted:     []string{"1.2.3.4/32"},
			expected:   []string{"1.2.3.4/32"},
			expChanged: false,
		},
		{
			name:       "customer entries kept in place",
			current:    []string{"10.0.0.0/8", "192.168.0.0/16"},
			wanted:     []string{"1.2.3.4/32"},
			expected:   []string{"10.0.0.0/8", "192.168.0.0/16", "1.2.3.4/32"},
			expChanged: true,
		},
		{
			name:       "customer already listed the load balancer",
			current:    []string{"1.2.3.4/32"},
			wanted:     []string{"1.2.3.4/32"},
			expected:   []string{"1.2.3.4/32"},
			expChanged: false,
		},
		{
			name:       "load balancer IP changed",
			current:    []string{"10.0.0.0/8", "1.2.3.4/32"},
			managed:    []string{"1.2.3.4/32"},
			wanted:     []string{"5.6.7.8/32"},
			expected:   []string{"10.0.0.0/8", "5.6.7.8/32"},
			expChanged: true,
		},
		{
			name:       "opt-out removes only managed entries",
			current:    []string{"10.0.0.0/8", "1.2.3.4/32"},
			managed:    []string{"1.2.3.4/32"},
			expected:   []string{"10.0.0.0/8"},
			expChanged: true,
		},
		{
			name:       "opt-out with only managed entries leaves the field empty",
			current:    []string{"1.2.3.4/32"},
			managed:    []string{"1.2.3.4/32"},
			expected:   []string{},
			expChanged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := desiredSourceRanges(tt.current, tt.managed, tt.wanted)
			assert.Equal(t, tt.expected, got)
			assert.Equal(t, tt.expChanged, changed)
		})
	}
}

func TestLBSourceCIDRs(t *testing.T) {
	lb := &hcloud.LoadBalancer{
		PublicNet: hcloud.LoadBalancerPublicNet{
			Enabled: true,
			IPv4:    hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("1.2.3.4")},
			IPv6:    hcloud.LoadBalancerPublicNetIPv6{IP: net.ParseIP("2001:db8::1")},
		},
		PrivateNet: []hcloud.LoadBalancerPrivateNet{{IP: net.ParseIP("10.0.0.2")}},
	}
	assert.Equal(t, []string{"1.2.3.4/32", "10.0.0.2/32"}, lbSourceCIDRs(lb))

	lb.PublicNet.Enabled = false
	assert.Equal(t, []string{"10.0.0.2/32"}, lbSourceCIDRs(lb))

	assert.Empty(t, lbSourceCIDRs(&hcloud.LoadBalancer{}))
}

func testLB() *hcloud.LoadBalancer {
	return &hcloud.LoadBalancer{
		ID: 1,
		PublicNet: hcloud.LoadBalancerPublicNet{
			Enabled: true,
			IPv4:    hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("1.2.3.4")},
			IPv6:    hcloud.LoadBalancerPublicNetIPv6{IP: net.ParseIP("2001:db8::1")},
		},
	}
}

func testService(annotations map[string]string, ranges ...string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", Annotations: annotations},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeLoadBalancer,
			LoadBalancerSourceRanges: ranges,
		},
	}
}

func getService(t *testing.T, client *fake.Clientset) *corev1.Service {
	t.Helper()
	svc, err := client.CoreV1().Services("default").Get(context.Background(), "web", metav1.GetOptions{})
	require.NoError(t, err)
	return svc
}

func countUpdates(client *fake.Clientset) int {
	n := 0
	for _, a := range client.Actions() {
		if a.GetVerb() == "update" {
			n++
		}
	}
	return n
}

func TestReconcileNodePortSource(t *testing.T) {
	restrict := string(annotation.LBRestrictNodePortSource)
	managed := string(annotation.LBNodePortSourceRanges)

	tests := []struct {
		name       string
		envDefault bool
		svc        *corev1.Service
		expRanges  []string
		expManaged string // "" means the annotation is absent
		expUpdates int
	}{
		{
			name:       "enabled by env",
			envDefault: true,
			svc:        testService(nil),
			expRanges:  []string{"1.2.3.4/32"},
			expManaged: "1.2.3.4/32",
			expUpdates: 1,
		},
		{
			name:       "enabled by annotation",
			svc:        testService(map[string]string{restrict: "true"}),
			expRanges:  []string{"1.2.3.4/32"},
			expManaged: "1.2.3.4/32",
			expUpdates: 1,
		},
		{
			name:       "disabled by default",
			svc:        testService(nil),
			expUpdates: 0,
		},
		{
			name:       "annotation opts out despite env",
			envDefault: true,
			svc:        testService(map[string]string{restrict: "false"}),
			expUpdates: 0,
		},
		{
			name:       "opt-out removes managed entries and annotation",
			envDefault: true,
			svc: testService(map[string]string{restrict: "false", managed: "1.2.3.4/32"},
				"10.0.0.0/8", "1.2.3.4/32"),
			expRanges:  []string{"10.0.0.0/8"},
			expUpdates: 1,
		},
		{
			name:       "customer ranges kept",
			envDefault: true,
			svc:        testService(nil, "10.0.0.0/8"),
			expRanges:  []string{"10.0.0.0/8", "1.2.3.4/32"},
			expManaged: "1.2.3.4/32",
			expUpdates: 1,
		},
		{
			name:       "already up to date",
			envDefault: true,
			svc:        testService(map[string]string{managed: "1.2.3.4/32"}, "1.2.3.4/32"),
			expRanges:  []string{"1.2.3.4/32"},
			expManaged: "1.2.3.4/32",
			expUpdates: 0,
		},
		{
			name:       "customer already listed the load balancer IP",
			envDefault: true,
			svc:        testService(nil, "1.2.3.4/32"),
			expRanges:  []string{"1.2.3.4/32"},
			expUpdates: 0,
		},
		{
			name:       "opt-out keeps a customer-listed load balancer IP",
			svc:        testService(map[string]string{restrict: "false"}, "1.2.3.4/32"),
			expRanges:  []string{"1.2.3.4/32"},
			expUpdates: 0,
		},
		{
			name:       "load balancer IP changed",
			envDefault: true,
			svc:        testService(map[string]string{managed: "9.9.9.9/32"}, "10.0.0.0/8", "9.9.9.9/32"),
			expRanges:  []string{"10.0.0.0/8", "1.2.3.4/32"},
			expManaged: "1.2.3.4/32",
			expUpdates: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset(tt.svc.DeepCopy())
			l := newLoadBalancers(nil, nil, false, false, tt.envDefault)
			l.k8sClient = client

			require.NoError(t, l.reconcileNodePortSource(context.Background(), tt.svc, testLB()))

			got := getService(t, client)
			if len(tt.expRanges) == 0 {
				assert.Empty(t, got.Spec.LoadBalancerSourceRanges)
			} else {
				assert.Equal(t, tt.expRanges, got.Spec.LoadBalancerSourceRanges)
			}
			v, ok := got.Annotations[managed]
			if tt.expManaged == "" {
				assert.False(t, ok, "bookkeeping annotation should be absent, got %q", v)
			} else {
				assert.Equal(t, tt.expManaged, v)
			}
			assert.Equal(t, tt.expUpdates, countUpdates(client))
		})
	}
}

func TestReconcileNodePortSource_InvalidAnnotation(t *testing.T) {
	svc := testService(map[string]string{string(annotation.LBRestrictNodePortSource): "maybe"})
	l := newLoadBalancers(nil, nil, false, false, true)
	l.k8sClient = fake.NewClientset(svc.DeepCopy())
	assert.Error(t, l.reconcileNodePortSource(context.Background(), svc, testLB()))
}

func TestReconcileNodePortSource_NoClient(t *testing.T) {
	l := newLoadBalancers(nil, nil, false, false, true)
	assert.NoError(t, l.reconcileNodePortSource(context.Background(), testService(nil), testLB()))
}

func TestReconcileNodePortSource_NoUsableIP(t *testing.T) {
	svc := testService(nil)
	client := fake.NewClientset(svc.DeepCopy())
	l := newLoadBalancers(nil, nil, false, false, true)
	l.k8sClient = client

	require.NoError(t, l.reconcileNodePortSource(context.Background(), svc, &hcloud.LoadBalancer{ID: 1}))
	assert.Equal(t, 0, countUpdates(client))
}

func TestReconcileNodePortSource_RetriesOnConflict(t *testing.T) {
	svc := testService(nil)
	client := fake.NewClientset(svc.DeepCopy())
	conflicts := 1
	client.PrependReactor("update", "services", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		if conflicts > 0 {
			conflicts--
			return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "services"}, "web", nil)
		}
		return false, nil, nil
	})
	l := newLoadBalancers(nil, nil, false, false, true)
	l.k8sClient = client

	require.NoError(t, l.reconcileNodePortSource(context.Background(), svc, testLB()))
	assert.Equal(t, []string{"1.2.3.4/32"}, getService(t, client).Spec.LoadBalancerSourceRanges)
}

func TestRemoveNodePortSource(t *testing.T) {
	managed := string(annotation.LBNodePortSourceRanges)

	t.Run("removes managed entries after type change", func(t *testing.T) {
		svc := testService(map[string]string{managed: "1.2.3.4/32"}, "10.0.0.0/8", "1.2.3.4/32")
		svc.Spec.Type = corev1.ServiceTypeNodePort
		client := fake.NewClientset(svc.DeepCopy())
		l := newLoadBalancers(nil, nil, false, false, true)
		l.k8sClient = client

		require.NoError(t, l.removeNodePortSource(context.Background(), svc))
		got := getService(t, client)
		assert.Equal(t, []string{"10.0.0.0/8"}, got.Spec.LoadBalancerSourceRanges)
		assert.NotContains(t, got.Annotations, managed)
	})

	t.Run("nothing managed", func(t *testing.T) {
		svc := testService(nil, "10.0.0.0/8")
		client := fake.NewClientset(svc.DeepCopy())
		l := newLoadBalancers(nil, nil, false, false, true)
		l.k8sClient = client

		require.NoError(t, l.removeNodePortSource(context.Background(), svc))
		assert.Empty(t, client.Actions())
	})

	t.Run("service already deleted", func(t *testing.T) {
		svc := testService(map[string]string{managed: "1.2.3.4/32"}, "1.2.3.4/32")
		l := newLoadBalancers(nil, nil, false, false, true)
		l.k8sClient = fake.NewClientset()

		assert.NoError(t, l.removeNodePortSource(context.Background(), svc))
	})
}

func TestLoadBalancers_EnsureLoadBalancer_RestrictsNodePortSource(t *testing.T) {
	tests := []LoadBalancerTestCase{
		{
			Name:       "ensure writes the load balancer IP into the source ranges",
			ServiceUID: "np-1",
			ServiceAnnotations: map[annotation.Name]interface{}{
				annotation.LBRestrictNodePortSource: true,
			},
			LB: &hcloud.LoadBalancer{
				ID:               1,
				Name:             "np-lb",
				LoadBalancerType: &hcloud.LoadBalancerType{Name: "lb11"},
				Location:         &hcloud.Location{Name: "nbg1", NetworkZone: hcloud.NetworkZoneEUCentral},
				PublicNet: hcloud.LoadBalancerPublicNet{
					Enabled: true,
					IPv4:    hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("1.2.3.4")},
					IPv6:    hcloud.LoadBalancerPublicNetIPv6{IP: net.ParseIP("2001:db8::1")},
				},
			},
			Mock: func(_ *testing.T, tt *LoadBalancerTestCase) {
				tt.Service.Name = "web"
				tt.Service.Namespace = "default"
				tt.Service.Spec.Type = corev1.ServiceTypeLoadBalancer
				tt.LBOps.On("GetByK8SServiceUID", tt.Ctx, tt.Service).Return(tt.LB, nil)
				tt.LBOps.On("ReconcileHCLB", tt.Ctx, tt.LB, tt.Service).Return(false, nil)
				tt.LBOps.On("ReconcileHCLBServices", tt.Ctx, tt.LB, tt.Service).Return(false, nil)
				tt.LBOps.On("ReconcileHCLBTargets", tt.Ctx, tt.LB, tt.Service, tt.Nodes).Return(false, nil)
			},
			Perform: func(t *testing.T, tt *LoadBalancerTestCase) {
				client := fake.NewClientset(tt.Service.DeepCopy())
				tt.LoadBalancers.k8sClient = client

				_, err := tt.LoadBalancers.EnsureLoadBalancer(tt.Ctx, tt.ClusterName, tt.Service, tt.Nodes)
				require.NoError(t, err)

				got := getService(t, client)
				assert.Equal(t, []string{"1.2.3.4/32"}, got.Spec.LoadBalancerSourceRanges)
				assert.Equal(t, "1.2.3.4/32", got.Annotations[string(annotation.LBNodePortSourceRanges)])
			},
		},
	}

	RunLoadBalancerTests(t, tests)
}
