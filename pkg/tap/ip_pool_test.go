package tap

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPPool(t *testing.T) {
	_, network, _ := net.ParseCIDR("10.0.0.0/8")
	pool := NewIPPool(network)

	ip1, err := pool.GetOrAssign("mac1")
	assert.NoError(t, err)
	assert.Equal(t, "10.0.0.1", ip1.String())

	ip1, err = pool.GetOrAssign("mac1")
	assert.NoError(t, err)
	assert.Equal(t, "10.0.0.1", ip1.String())

	ip2, err := pool.GetOrAssign("mac2")
	assert.NoError(t, err)
	assert.Equal(t, "10.0.0.2", ip2.String())

	assert.Equal(t, map[string]string{"10.0.0.1": "mac1", "10.0.0.2": "mac2"}, pool.Leases())

	pool.Release("mac1")

	assert.Equal(t, map[string]string{"10.0.0.2": "mac2"}, pool.Leases())

	ip3, err := pool.GetOrAssign("mac3")
	assert.NoError(t, err)
	assert.Equal(t, "10.0.0.1", ip3.String())

	ip4, err := pool.GetOrAssign("mac4")
	assert.NoError(t, err)
	assert.Equal(t, "10.0.0.3", ip4.String())

	assert.Equal(t, map[string]string{"10.0.0.1": "mac3", "10.0.0.2": "mac2", "10.0.0.3": "mac4"}, pool.Leases())
}

func TestIPPoolIPv6(t *testing.T) {
	_, network, _ := net.ParseCIDR("fd00::/64")
	pool := NewIPPool(network)

	ip1, err := pool.GetOrAssign("mac1")
	assert.NoError(t, err)
	assert.Equal(t, "fd00::1", ip1.String())

	ip1, err = pool.GetOrAssign("mac1")
	assert.NoError(t, err)
	assert.Equal(t, "fd00::1", ip1.String())

	ip2, err := pool.GetOrAssign("mac2")
	assert.NoError(t, err)
	assert.Equal(t, "fd00::2", ip2.String())

	assert.Equal(t, map[string]string{"fd00::1": "mac1", "fd00::2": "mac2"}, pool.Leases())

	pool.Release("mac1")

	assert.Equal(t, map[string]string{"fd00::2": "mac2"}, pool.Leases())

	ip3, err := pool.GetOrAssign("mac3")
	assert.NoError(t, err)
	assert.Equal(t, "fd00::1", ip3.String())

	ip4, err := pool.GetOrAssign("mac4")
	assert.NoError(t, err)
	assert.Equal(t, "fd00::3", ip4.String())

	assert.Equal(t, map[string]string{"fd00::1": "mac3", "fd00::2": "mac2", "fd00::3": "mac4"}, pool.Leases())
}
