package handshake

import (
	"crypto/cipher"
	"encoding/binary"

	"github.com/lucas-clemente/quic-go/internal/protocol"
)

type sealer struct {
	iv   []byte
	aead cipher.AEAD
}

var _ Sealer = &sealer{}

func newSealer(aead cipher.AEAD, iv []byte) Sealer {
	return &sealer{
		iv:   iv,
		aead: aead,
	}
}

func (s *sealer) Seal(dst, src []byte, pn protocol.PacketNumber, ad []byte) []byte {
	return s.aead.Seal(dst, makeNonce(s.iv, pn), src, ad)
}

func (s *sealer) Overhead() int {
	return s.aead.Overhead()
}

type opener struct {
	iv   []byte
	aead cipher.AEAD
}

var _ Opener = &opener{}

func newOpener(aead cipher.AEAD, iv []byte) Opener {
	return &opener{
		iv:   iv,
		aead: aead,
	}
}

func (o *opener) Open(dst, src []byte, pn protocol.PacketNumber, ad []byte) ([]byte, error) {
	return o.aead.Open(dst, makeNonce(o.iv, pn), src, ad)
}

func makeNonce(iv []byte, pn protocol.PacketNumber) []byte {
	ivLen := len(iv)
	nonce := make([]byte, ivLen)
	binary.BigEndian.PutUint64(nonce[ivLen-8:], uint64(pn))
	for i := 0; i < ivLen; i++ {
		nonce[i] ^= iv[i]
	}
	return nonce
}
