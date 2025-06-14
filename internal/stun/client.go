package stun

import (
	"context"
	"fmt"
	"net"

	"github.com/pion/stun"
)

// Client handles public IP discovery via STUN servers
type Client interface {
	DiscoverPublicIP(ctx context.Context, stunServer string) (string, error)
}

// DefaultClient implements Client
type DefaultClient struct{}

// New creates a new STUN client
func New() Client {
	return &DefaultClient{}
}

// DiscoverPublicIP discovers the public IP address using STUN
func (c *DefaultClient) DiscoverPublicIP(ctx context.Context, stunServer string) (string, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", stunServer)
	if err != nil {
		return "", fmt.Errorf("failed to dial STUN server: %w", err)
	}
	defer conn.Close()

	client, err := stun.NewClient(conn)
	if err != nil {
		return "", fmt.Errorf("failed to create STUN client: %w", err)
	}
	defer client.Close()

	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	var publicIP string
	var stunErr error
	err = client.Do(message, func(res stun.Event) {
		if res.Error != nil {
			stunErr = res.Error
			return
		}

		var xorAddr stun.XORMappedAddress
		if err := xorAddr.GetFrom(res.Message); err != nil {
			stunErr = err
			return
		}

		publicIP = xorAddr.IP.String()
	})

	if stunErr != nil {
		return "", stunErr
	}

	if err != nil {
		return "", err
	}

	return publicIP, nil
}