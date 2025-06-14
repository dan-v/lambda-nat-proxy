package shared

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// PerformNATHolePunch performs NAT hole punching between two UDP endpoints
func PerformNATHolePunch(conn *net.UDPConn, sessionID string, remoteAddr *net.UDPAddr, timeout time.Duration, isServer bool) error {
	role := "client"
	if isServer {
		role = "server"
	}
	log.Printf("🔨 [%s] Starting NAT hole punching to %s", role, remoteAddr)

	// Send punch packets
	punchDone := make(chan bool)
	go func() {
		for i := 0; i < HolePunchPacketCount; i++ {
			message := fmt.Sprintf("PUNCH:%s:%d", sessionID, i)
			conn.WriteToUDP([]byte(message), remoteAddr)
			time.Sleep(HolePunchInterval)
		}
		close(punchDone)
	}()

	// Listen for remote's punch packets
	successChan := make(chan bool)
	go func() {
		buf := make([]byte, UDPBufferSize)
		for {
			conn.SetReadDeadline(time.Now().Add(UDPReadTimeout))
			n, addr, err := conn.ReadFromUDP(buf)

			if err == nil && addr.IP.Equal(remoteAddr.IP) && addr.Port == remoteAddr.Port {
				data := string(buf[:n])
				if strings.HasPrefix(data, "PUNCH:") {
					log.Printf("✅ [%s] Received punch packet from remote: %s", role, data)
					successChan <- true
					return
				}
			}
		}
	}()

	// Wait for success or timeout
	select {
	case <-successChan:
		<-punchDone // Wait for sender to finish
		conn.SetReadDeadline(time.Time{}) // Clear deadline
		return nil
	case <-time.After(timeout):
		conn.SetReadDeadline(time.Time{}) // Clear deadline
		return fmt.Errorf("NAT hole punching timeout")
	}
}

// CreateUDPSocket creates a UDP socket for NAT traversal
func CreateUDPSocket() (*net.UDPConn, int, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create UDP socket: %w", err)
	}

	port := conn.LocalAddr().(*net.UDPAddr).Port
	return conn, port, nil
}