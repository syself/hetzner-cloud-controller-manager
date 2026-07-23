package addressfamily_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syself/hetzner-cloud-controller-manager/internal/addressfamily"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    addressfamily.Family
		wantErr bool
	}{
		{in: "ipv4", want: addressfamily.IPv4},
		{in: "ipv6", want: addressfamily.IPv6},
		{in: "dualstack", want: addressfamily.DualStack},
		{in: "IPv4", want: addressfamily.IPv4},
		{in: "DualStack", want: addressfamily.DualStack},
		{in: "", wantErr: true},
		{in: "both", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			got, err := addressfamily.Parse(test.in)
			if test.wantErr {
				assert.ErrorIs(t, err, addressfamily.ErrInvalid)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestFamilyUses(t *testing.T) {
	tests := []struct {
		name     string
		family   addressfamily.Family
		wantIPv4 bool
		wantIPv6 bool
	}{
		{name: "ipv4", family: addressfamily.IPv4, wantIPv4: true},
		{name: "ipv6", family: addressfamily.IPv6, wantIPv6: true},
		{name: "dualstack", family: addressfamily.DualStack, wantIPv4: true, wantIPv6: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.wantIPv4, test.family.UsesIPv4())
			assert.Equal(t, test.wantIPv6, test.family.UsesIPv6())
		})
	}
}
