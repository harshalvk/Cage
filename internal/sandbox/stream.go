package sandbox

import (
	"bytes"
	"encoding/binary"
	"io"
)

func demuxOutput(reader io.Reader) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	header := make([]byte, 8)

	for {
		_, err := io.ReadFull(reader, header)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", err
		}

		streamType := header[0]
		size := binary.BigEndian.Uint32(header[4:8])

		payload := make([]byte, size)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return "", "", err
		}

		switch streamType {
		case 1:
			outBuf.Write(payload)
		case 2:
			errBuf.Write(payload)
		}
	}

	return outBuf.String(), errBuf.String(), nil
}
