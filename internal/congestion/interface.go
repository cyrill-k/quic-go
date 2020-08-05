package congestion

import (
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
)

// A SendAlgorithm performs congestion control and calculates the congestion window
type SendAlgorithm interface {
	TimeUntilSend(bytesInFlight protocol.ByteCount) time.Duration
	OnPacketSent(sentTime time.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool)
	GetCongestionWindow() protocol.ByteCount
	MaybeExitSlowStart()
	OnPacketAcked(number protocol.PacketNumber, ackedBytes protocol.ByteCount, priorInFlight protocol.ByteCount, eventTime time.Time)
	OnPacketLost(number protocol.PacketNumber, lostBytes protocol.ByteCount, priorInFlight protocol.ByteCount)
	SetNumEmulatedConnections(n int)
	OnRetransmissionTimeout(packetsRetransmitted bool)
	OnConnectionMigration()

	// Experiments
	SetSlowStartLargeReduction(enabled bool)
}

// SendAlgorithmWithDebugInfo adds some debug functions to SendAlgorithm
type SendAlgorithmWithDebugInfo interface {
	SendAlgorithm
	BandwidthEstimate() Bandwidth

	// Stuff only used in testing

	HybridSlowStart() *HybridSlowStart
	SlowstartThreshold() protocol.ByteCount
	RenoBeta() float32
	InRecovery() bool
}

type FlowteleCongestionControlModifier interface {
	ApplyControl(beta float64, cwnd_adjust int16, cwnd_max_adjust int16, use_conservative_allocation bool) bool

	SetFixedRate(rateInBytePerSecond Bandwidth)
}

// FlowteleSendAlgorithmWithDebugInfo adds flowtele CC control functions to SendAlgorithmWithDebugInfo
type FlowteleSendAlgorithm interface {
	SendAlgorithm

	FlowteleCongestionControlModifier
}

// FlowteleSendAlgorithmWithDebugInfo adds flowtele CC control functions to SendAlgorithmWithDebugInfo
type FlowteleSendAlgorithmWithDebugInfo interface {
	SendAlgorithmWithDebugInfo

	FlowteleCongestionControlModifier
}
