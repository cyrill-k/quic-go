package congestion

import (
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
)

// A SendAlgorithm performs congestion control
type SendAlgorithm interface {
	TimeUntilSend(bytesInFlight protocol.ByteCount) time.Time
	HasPacingBudget() bool
	OnPacketSent(sentTime time.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool)
	CanSend(bytesInFlight protocol.ByteCount) bool
	MaybeExitSlowStart()
	OnPacketAcked(number protocol.PacketNumber, ackedBytes protocol.ByteCount, priorInFlight protocol.ByteCount, eventTime time.Time)
	OnPacketLost(number protocol.PacketNumber, lostBytes protocol.ByteCount, priorInFlight protocol.ByteCount)
	OnRetransmissionTimeout(packetsRetransmitted bool)
}

// A SendAlgorithmWithDebugInfos is a SendAlgorithm that exposes some debug infos
type SendAlgorithmWithDebugInfos interface {
	SendAlgorithm
	InSlowStart() bool
	InRecovery() bool
	GetCongestionWindow() protocol.ByteCount
}

type FlowteleCongestionControlModifier interface {
	ApplyControl(beta float64, cwnd_adjust int64, cwnd_max_adjust int64, use_conservative_allocation bool) bool

	SetFixedRate(rateInBytePerSecond Bandwidth)
}

// FlowteleSendAlgorithmWithDebugInfo adds flowtele CC control functions to SendAlgorithmWithDebugInfo
type FlowteleSendAlgorithm interface {
	SendAlgorithm

	FlowteleCongestionControlModifier
}

// FlowteleSendAlgorithmWithDebugInfo adds flowtele CC control functions to SendAlgorithmWithDebugInfo
type FlowteleSendAlgorithmWithDebugInfos interface {
	SendAlgorithmWithDebugInfos

	FlowteleCongestionControlModifier
}
