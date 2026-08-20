package firecracker

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// FirecrackerShell wraps a persistent vsock connection to the guest
// agent's shell listener, applying the same length-prefixed framing the
// guest speaks
type FirecrackerShell struct {
	conn net.Conn
}

// Read blocks until a data frame arrives, skipping any unexpected
// non-data frames rather than erroring the whole session over one
// malformed frame
func (s *FirecrackerShell) Read(p []byte) (int, error) {
	for {
		typ, payload, err := readFrame(s.conn)
		if err != nil {
			return 0, err
		}
		if typ == frameData {
			return copy(p, payload), nil
		}
	}
}

func (s *FirecrackerShell) Write(p []byte) (int, error) {
	if err := writeFrame(s.conn, frameData, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *FirecrackerShell) Resize(cols, rows uint16) error {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[0:2], cols)
	binary.BigEndian.PutUint16(payload[2:4], rows)
	return writeFrame(s.conn, frameResize, payload)
}

func (s *FirecrackerShell) Close() error {
	return s.conn.Close()
}

// frame type constants and writeFrame/readFrame mirror guest-agent's
// protocol exactly -- keep both sides in sync if this ever changes
const (
	frameData   byte = 0x01
	frameResize byte = 0x02
)

func writeFrame(w io.Writer, typ byte, payload []byte) error {
	header := make([]byte, 5)
	header[0] = typ
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("failed to write frame header: %w", err)
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return fmt.Errorf("failed to write frame payload: %w", err)
		}
	}
	return nil
}

func readFrame(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	typ := header[0]
	length := binary.BigEndian.Uint32(header[1:])
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}
	return typ, payload, nil
}
