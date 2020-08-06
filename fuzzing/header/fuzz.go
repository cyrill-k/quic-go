// +build gofuzz

package header

import (
	"bytes"
	"fmt"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/wire"
)

const version = protocol.VersionTLS

func Fuzz(data []byte) int {
	if len(data) < 1 {
		return 0
	}
	connIDLen := int(data[0] % 21)
	data = data[1:]

	if wire.IsVersionNegotiationPacket(data) {
		return fuzzVNP(data)
	}
	connID, err := wire.ParseConnectionID(data, connIDLen)
	if err != nil {
		return 0
	}
	hdr, _, _, err := wire.ParsePacket(data, connIDLen)
	if err != nil {
		return 0
	}
	if !hdr.DestConnectionID.Equal(connID) {
		panic(fmt.Sprintf("Expected connection IDs to match: %s vs %s", hdr.DestConnectionID, connID))
	}

	var extHdr *wire.ExtendedHeader
	// Parse the extended header, if this is not a Retry packet.
	if hdr.Type == protocol.PacketTypeRetry {
		extHdr = &wire.ExtendedHeader{Header: *hdr}
	} else {
		var err error
		extHdr, err = hdr.ParseExtended(bytes.NewReader(data), version)
		if err != nil {
			return 0
		}
	}
	b := &bytes.Buffer{}
	if err := extHdr.Write(b, version); err != nil {
		// We are able to parse packets with connection IDs longer than 20 bytes,
		// but in QUIC version 1, we don't write headers with longer connection IDs.
		if hdr.DestConnectionID.Len() <= protocol.MaxConnIDLen &&
			hdr.SrcConnectionID.Len() <= protocol.MaxConnIDLen {
			panic(err)
		}
		return 0
	}
	// GetLength is not implemented for Retry packets
	if hdr.Type != protocol.PacketTypeRetry {
		if expLen := extHdr.GetLength(version); expLen != protocol.ByteCount(b.Len()) {
			panic(fmt.Sprintf("inconsistent header length: %#v. Expected %d, got %d", extHdr, expLen, b.Len()))
		}
	}
	return 0
}

func fuzzVNP(data []byte) int {
	connID, err := wire.ParseConnectionID(data, 0)
	if err != nil {
		return 0
	}
	hdr, versions, err := wire.ParseVersionNegotiationPacket(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	if !hdr.DestConnectionID.Equal(connID) {
		panic("connection IDs don't match")
	}
	if len(versions) == 0 {
		panic("no versions")
	}
	if _, err := wire.ComposeVersionNegotiation(hdr.SrcConnectionID, hdr.DestConnectionID, versions); err != nil {
		panic(err)
	}
	return 0
}
