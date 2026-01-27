package providerid

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFromCloudServerID(t *testing.T) {
	tests := []struct {
		name     string
		serverID int64
		want     string
	}{
		{
			name:     "simple id",
			serverID: 1234,
			want:     "hcloud://1234",
		},
		{
			name:     "large id",
			serverID: 2251799813685247,
			want:     "hcloud://2251799813685247",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromCloudServerID(tt.serverID); got != tt.want {
				t.Errorf("FromCloudServerID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToServerID(t *testing.T) {
	tests := []struct {
		name              string
		providerID        string
		wantID            int64
		wantIsCloudServer bool
		wantErr           error
	}{
		{
			name:              "[cloud] simple id",
			providerID:        "hcloud://1234",
			wantID:            1234,
			wantIsCloudServer: true,
			wantErr:           nil,
		},
		{
			name:              "[cloud] large id",
			providerID:        "hcloud://2251799813685247",
			wantID:            2251799813685247,
			wantIsCloudServer: true,
			wantErr:           nil,
		},
		{
			name:              "[cloud] invalid id",
			providerID:        "hcloud://my-cloud",
			wantID:            0,
			wantIsCloudServer: false,
			wantErr:           errors.New("unable to parse server id: hcloud://my-cloud"),
		},
		{
			name:              "[cloud] missing id",
			providerID:        "hcloud://",
			wantID:            0,
			wantIsCloudServer: false,
			wantErr:           errors.New("providerID is missing a serverID: hcloud://"),
		},
		{
			name:              "[robot-syself] simple id (legacy)",
			providerID:        "hcloud://bm-4321",
			wantID:            4321,
			wantIsCloudServer: false,
			wantErr:           nil,
		},
		{
			name:              "[robot-syself] simple id (new)",
			providerID:        "hrobot://4321",
			wantID:            4321,
			wantIsCloudServer: false,
			wantErr:           nil,
		},
		{
			name:              "[robot-syself] invalid id",
			providerID:        "hcloud://bm-my-robot",
			wantID:            0,
			wantIsCloudServer: false,
			wantErr:           errors.New("unable to parse server id: hcloud://bm-my-robot"),
		},
		{
			name:              "[robot-syself] missing id",
			providerID:        "hcloud://bm-",
			wantID:            0,
			wantIsCloudServer: false,
			wantErr:           errors.New("providerID is missing a serverID: hcloud://bm-"),
		},
		{
			name:              "unknown format",
			providerID:        "foobar/321",
			wantID:            0,
			wantIsCloudServer: false,
			wantErr:           &UnkownPrefixError{"foobar/321"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotIsCloudServer, err := ToServerID(tt.providerID)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ToServerID() expected error = %v, got nil", tt.wantErr)
					return
				}
				if errors.As(tt.wantErr, new(*UnkownPrefixError)) {
					assert.ErrorAsf(t, err, new(*UnkownPrefixError), "ToServerID() error = %v, wantErr %v", err, tt.wantErr)
				} else {
					assert.EqualErrorf(t, err, tt.wantErr.Error(), "ToServerID() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("ToServerID() unexpected error = %v, wantErr nil", err)
			}
			if gotID != tt.wantID {
				t.Errorf("ToServerID() gotID = %v, want %v", gotID, tt.wantID)
			}
			if gotIsCloudServer != tt.wantIsCloudServer {
				t.Errorf("ToServerID() gotIsCloudServer = %v, want %v", gotIsCloudServer, tt.wantIsCloudServer)
			}
		})
	}
}

func FuzzRoundTripCloud(f *testing.F) {
	f.Add(int64(123123123))

	f.Fuzz(func(t *testing.T, serverID int64) {
		providerID := FromCloudServerID(serverID)
		id, isCloudServer, err := ToServerID(providerID)
		if err != nil {
			t.Fatal(err)
		}
		if id != serverID {
			t.Fatalf("expected %d, got %d", serverID, id)
		}
		if !isCloudServer {
			t.Fatalf("expected %t, got %t", true, isCloudServer)
		}
	})
}

func FuzzToServerId(f *testing.F) {
	f.Add("hcloud://123123123")
	f.Add("hcloud://bm-123123123")

	f.Fuzz(func(t *testing.T, providerID string) {
		_, _, err := ToServerID(providerID)
		if err != nil {
			if strings.HasPrefix(err.Error(), "providerID does not have one of the the expected prefixes") {
				return
			}
			if strings.HasPrefix(err.Error(), "providerID is missing a serverID") {
				return
			}
			if strings.HasPrefix(err.Error(), "unable to parse server id") {
				return
			}

			t.Fatal(err)
		}
	})
}

func TestGetProviderId(t *testing.T) {
	tests := []struct {
		name         string
		node         *corev1.Node
		serverNumber int
		want         string
		wantErr      bool
	}{
		{
			name: "provider id already set",
			node: &corev1.Node{
				Spec: corev1.NodeSpec{ProviderID: "hcloud://bm-999"},
			},
			serverNumber: 321,
			want:         "hcloud://bm-999",
		},
		{
			name: "no annotation uses legacy",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "bm-node-1"},
			},
			serverNumber: 321,
			want:         "hcloud://bm-321",
		},
		{
			name: "annotation uses hrobot",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bm-node-2",
					Annotations: map[string]string{
						hetznerBMProviderIDPrefixAnnotation: "hrobot://",
					},
				},
			},
			serverNumber: 321,
			want:         "hrobot://321",
		},
		{
			name: "invalid annotation prefix",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bm-node-3",
					Annotations: map[string]string{
						hetznerBMProviderIDPrefixAnnotation: "bad://",
					},
				},
			},
			serverNumber: 321,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetProviderId(tt.node, tt.serverNumber)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				assert.ErrorContains(t, err, "invalid")
				assert.ErrorContains(t, err, "hetzner-bm-provider-id-prefix")
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
