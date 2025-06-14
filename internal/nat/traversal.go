package nat

import (
	"net"
	"time"

	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
)

// Traversal handles UDP hole punching
type Traversal interface {
	CreateUDPSocket() (*net.UDPConn, int, error)
	PerformHolePunch(conn *net.UDPConn, sessionID string, lambdaAddr *net.UDPAddr, timeout time.Duration) error
}

// DefaultTraversal implements Traversal
type DefaultTraversal struct{}

// New creates a new NAT traversal client
func New() Traversal {
	return &DefaultTraversal{}
}

// CreateUDPSocket creates a UDP socket for hole punching
func (n *DefaultTraversal) CreateUDPSocket() (*net.UDPConn, int, error) {
	return shared.CreateUDPSocket()
}

// PerformHolePunch performs NAT hole punching with the Lambda
func (n *DefaultTraversal) PerformHolePunch(conn *net.UDPConn, sessionID string, lambdaAddr *net.UDPAddr, timeout time.Duration) error {
	return shared.PerformNATHolePunch(conn, sessionID, lambdaAddr, timeout, true)
}