package shared

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Control message opcodes
const (
	OpPing     byte = 0x01
	OpPong     byte = 0x02
	OpShutdown byte = 0x03
)

// Ping represents a ping message with a nonce
type Ping struct {
	Nonce uint64
}

// WritePing writes a ping message to the writer
func WritePing(w io.Writer, nonce uint64) error {
	if err := writeByte(w, OpPing); err != nil {
		return fmt.Errorf("failed to write ping opcode: %w", err)
	}
	if err := writeUint64(w, nonce); err != nil {
		return fmt.Errorf("failed to write ping nonce: %w", err)
	}
	return nil
}

// WritePong writes a pong message to the writer
func WritePong(w io.Writer, nonce uint64) error {
	if err := writeByte(w, OpPong); err != nil {
		return fmt.Errorf("failed to write pong opcode: %w", err)
	}
	if err := writeUint64(w, nonce); err != nil {
		return fmt.Errorf("failed to write pong nonce: %w", err)
	}
	return nil
}

// WriteShutdown writes a shutdown message to the writer
func WriteShutdown(w io.Writer) error {
	return writeByte(w, OpShutdown)
}

// ReadControlMessage reads a control message from the reader
func ReadControlMessage(r io.Reader) (opcode byte, nonce uint64, err error) {
	opcode, err = readByte(r)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read opcode: %w", err)
	}
	
	switch opcode {
	case OpPing, OpPong:
		nonce, err = readUint64(r)
		if err != nil {
			return opcode, 0, fmt.Errorf("failed to read nonce: %w", err)
		}
	case OpShutdown:
		// No additional data for shutdown
	default:
		return opcode, 0, fmt.Errorf("unknown opcode: %02x", opcode)
	}
	
	return opcode, nonce, nil
}

// Helper functions for reading/writing
func writeByte(w io.Writer, b byte) error {
	_, err := w.Write([]byte{b})
	return err
}

func writeUint64(w io.Writer, v uint64) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	_, err := w.Write(buf)
	return err
}

func readByte(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r, buf)
	return buf[0], err
}

func readUint64(r io.Reader) (uint64, error) {
	buf := make([]byte, 8)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf), nil
}