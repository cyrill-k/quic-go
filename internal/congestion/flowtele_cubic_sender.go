package congestion

import (
	"fmt"
	"math"
	"time"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
)

type FlowteleSignalInterface struct {
	NewSrttMeasurement func(t time.Time, srtt time.Duration)
	PacketsLost        func(t time.Time, newSlowStartThreshold uint64)
	PacketsAcked       func(t time.Time, congestionWindow uint64, packetsInFlight uint64, ackedBytes uint64)
}

type flowteleCubicSender struct {
	hybridSlowStart HybridSlowStart
	prr             PrrSender
	rttStats        *RTTStats
	stats           connectionStats
	cubic           *FlowteleCubic

	reno bool

	// Track the largest packet that has been sent.
	largestSentPacketNumber protocol.PacketNumber

	// Track the largest packet that has been acked.
	largestAckedPacketNumber protocol.PacketNumber

	// Track the largest packet number outstanding when a CWND cutback occurs.
	largestSentAtLastCutback protocol.PacketNumber

	// Congestion window in packets.
	congestionWindow protocol.PacketNumber

	// Slow start congestion window in packets, aka ssthresh.
	slowstartThreshold protocol.PacketNumber

	// Whether the last loss event caused us to exit slowstart.
	// Used for stats collection of slowstartPacketsLost
	lastCutbackExitedSlowstart bool

	// When true, exit slow start with large cutback of congestion window.
	slowStartLargeReduction bool

	// Minimum congestion window in packets.
	minCongestionWindow protocol.PacketNumber

	// Maximum number of outstanding packets for tcp.
	maxTCPCongestionWindow protocol.PacketNumber

	// Number of connections to simulate.
	numConnections int

	// ACK counter for the Reno implementation.
	congestionWindowCount protocol.ByteCount

	initialCongestionWindow    protocol.PacketNumber
	initialMaxCongestionWindow protocol.PacketNumber

	// interface to call if a respective event occurs
	flowteleSignalInterface *FlowteleSignalInterface

	useFixedBandwidth bool
	fixedBandwidth    Bandwidth
}

// NewCubicSender makes a new cubic sender
func NewFlowteleCubicSender(clock Clock, rttStats *RTTStats, reno bool, initialCongestionWindow, initialMaxCongestionWindow protocol.PacketNumber, flowteleSignalInterface *FlowteleSignalInterface) FlowteleSendAlgorithmWithDebugInfo {
	return &flowteleCubicSender{
		rttStats:                   rttStats,
		initialCongestionWindow:    initialCongestionWindow,
		initialMaxCongestionWindow: initialMaxCongestionWindow,
		congestionWindow:           initialCongestionWindow,
		minCongestionWindow:        defaultMinimumCongestionWindow,
		slowstartThreshold:         initialMaxCongestionWindow,
		maxTCPCongestionWindow:     initialMaxCongestionWindow,
		numConnections:             defaultNumConnections,
		cubic:                      NewFlowteleCubic(clock),
		reno:                       reno,
		flowteleSignalInterface:    flowteleSignalInterface,
	}
}

func (c *flowteleCubicSender) ApplyControl(beta float64, cwnd_adjust int16, cwnd_max_adjust int16, use_conservative_allocation bool) bool {
	fmt.Printf("FLOWTELE CC: ApplyControl(%f, %d, %d, %t)\n", beta, cwnd_adjust, cwnd_max_adjust, use_conservative_allocation)
	c.cubic.lastMaxCongestionWindowAddDelta = cwnd_max_adjust
	c.cubic.cwndAddDelta = cwnd_adjust
	c.cubic.betaValue = float32(beta)
	c.cubic.isThirdPhaseValue = use_conservative_allocation
	return true
}

func (c *flowteleCubicSender) SetFixedRate(rateInBitsPerSecond Bandwidth) {
	fmt.Printf("FLOWTELE CC: SetFixedRate(%d)\n", rateInBitsPerSecond)
	c.useFixedBandwidth = true
	c.fixedBandwidth = rateInBitsPerSecond
}

func (c *flowteleCubicSender) adjustCongestionWindow() {
	if c.useFixedBandwidth {
		srtt := c.rttStats.SmoothedRTT()
		// If we haven't measured an rtt, we cannot estimate the cwnd
		if srtt != 0 {
			c.congestionWindow = utils.MinPacketNumber(c.maxTCPCongestionWindow, protocol.PacketNumber(math.Ceil(float64(DeltaBytesFromBandwidth(c.fixedBandwidth, srtt))/float64(protocol.DefaultTCPMSS))))
			fmt.Printf("FLOWTELE CC: set congestion window to %d (%d), fixed bw = %d, srtt = %v\n", c.GetCongestionWindow(), c.congestionWindow, c.fixedBandwidth, srtt)
		}
	} else if c.cubic.cwndAddDelta != 0 {
		c.congestionWindow = utils.MaxPacketNumber(
			c.minCongestionWindow,
			utils.MinPacketNumber(
				c.maxTCPCongestionWindow,
				protocol.PacketNumber(
					math.Ceil(float64(
						int64(c.congestionWindow)+int64(c.cubic.cwndAddDelta))/
						float64(protocol.DefaultTCPMSS)))))
		c.cubic.cwndAddDelta = 0
	}

	if c.cubic.isThirdPhaseValue {
		c.cubic.CongestionWindowAfterPacketLoss(c.congestionWindow)
		c.cubic.isThirdPhaseValue = false
	}
}

func (c *flowteleCubicSender) slowStartthresholdUpdated() {
	// todo(cyrill) do we need the actual packet received time or is time.Now() sufficient?
	c.flowteleSignalInterface.PacketsLost(time.Now(), uint64(c.slowstartThreshold))
}

// TimeUntilSend returns when the next packet should be sent.
func (c *flowteleCubicSender) TimeUntilSend(bytesInFlight protocol.ByteCount) time.Duration {
	if c.InRecovery() {
		// PRR is used when in recovery.
		if c.prr.TimeUntilSend(c.GetCongestionWindow(), bytesInFlight, c.GetSlowStartThreshold()) == 0 {
			return 0
		}
	}
	delay := c.rttStats.SmoothedRTT() / time.Duration(2*c.GetCongestionWindow()/protocol.DefaultTCPMSS)
	if !c.InSlowStart() { // adjust delay, such that it's 1.25*cwd/rtt
		delay = delay * 8 / 5
	}
	return delay
}

func (c *flowteleCubicSender) OnPacketSent(sentTime time.Time, bytesInFlight protocol.ByteCount, packetNumber protocol.PacketNumber, bytes protocol.ByteCount, isRetransmittable bool) bool {
	// Only update bytesInFlight for data packets.
	if !isRetransmittable {
		return false
	}
	if c.InRecovery() {
		// PRR is used when in recovery.
		c.prr.OnPacketSent(bytes)
	}
	c.largestSentPacketNumber = packetNumber
	c.hybridSlowStart.OnPacketSent(packetNumber)
	return true
}

func (c *flowteleCubicSender) InRecovery() bool {
	return c.largestAckedPacketNumber <= c.largestSentAtLastCutback && c.largestAckedPacketNumber != 0
}

func (c *flowteleCubicSender) InSlowStart() bool {
	return c.GetCongestionWindow() < c.GetSlowStartThreshold()
}

func (c *flowteleCubicSender) GetCongestionWindow() protocol.ByteCount {
	return protocol.ByteCount(c.congestionWindow) * protocol.DefaultTCPMSS
}

func (c *flowteleCubicSender) GetSlowStartThreshold() protocol.ByteCount {
	return protocol.ByteCount(c.slowstartThreshold) * protocol.DefaultTCPMSS
}

func (c *flowteleCubicSender) ExitSlowstart() {
	c.slowstartThreshold = c.congestionWindow
}

func (c *flowteleCubicSender) SlowstartThreshold() protocol.PacketNumber {
	return c.slowstartThreshold
}

func (c *flowteleCubicSender) MaybeExitSlowStart() {
	if c.InSlowStart() && c.hybridSlowStart.ShouldExitSlowStart(c.rttStats.LatestRTT(), c.rttStats.MinRTT(), c.GetCongestionWindow()/protocol.DefaultTCPMSS) {
		c.ExitSlowstart()
	}
}

func (c *flowteleCubicSender) OnPacketAcked(ackedPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	c.largestAckedPacketNumber = utils.MaxPacketNumber(ackedPacketNumber, c.largestAckedPacketNumber)
	if c.InRecovery() {
		// PRR is used when in recovery.
		c.prr.OnPacketAcked(ackedBytes)
		return
	}
	c.maybeIncreaseCwnd(ackedPacketNumber, ackedBytes, bytesInFlight)
	if c.InSlowStart() {
		c.hybridSlowStart.OnPacketAcked(ackedPacketNumber)
	}

	c.flowteleSignalInterface.PacketsAcked(time.Now(), uint64(c.GetCongestionWindow()), uint64(bytesInFlight), uint64(ackedBytes))
}

func (c *flowteleCubicSender) OnPacketLost(packetNumber protocol.PacketNumber, lostBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	// TCP NewReno (RFC6582) says that once a loss occurs, any losses in packets
	// already sent should be treated as a single loss event, since it's expected.
	if packetNumber <= c.largestSentAtLastCutback {
		if c.lastCutbackExitedSlowstart {
			c.stats.slowstartPacketsLost++
			c.stats.slowstartBytesLost += lostBytes
			if c.slowStartLargeReduction {
				if c.stats.slowstartPacketsLost == 1 || (c.stats.slowstartBytesLost/protocol.DefaultTCPMSS) > (c.stats.slowstartBytesLost-lostBytes)/protocol.DefaultTCPMSS {
					// Reduce congestion window by 1 for every mss of bytes lost.
					c.congestionWindow = utils.MaxPacketNumber(c.congestionWindow-1, c.minCongestionWindow)
				}
				c.slowstartThreshold = c.congestionWindow
			}
		}
		return
	}
	c.lastCutbackExitedSlowstart = c.InSlowStart()
	if c.InSlowStart() {
		c.stats.slowstartPacketsLost++
	}

	c.prr.OnPacketLost(bytesInFlight)

	// TODO(chromium): Separate out all of slow start into a separate class.
	if c.slowStartLargeReduction && c.InSlowStart() {
		c.congestionWindow = c.congestionWindow - 1
	} else if c.reno {
		c.congestionWindow = protocol.PacketNumber(float32(c.congestionWindow) * c.RenoBeta())
	} else {
		c.congestionWindow = c.cubic.CongestionWindowAfterPacketLoss(c.congestionWindow)
	}
	// Enforce a minimum congestion window.
	if c.congestionWindow < c.minCongestionWindow {
		c.congestionWindow = c.minCongestionWindow
	}
	c.slowstartThreshold = c.congestionWindow
	c.largestSentAtLastCutback = c.largestSentPacketNumber
	// reset packet count from congestion avoidance mode. We start
	// counting again when we're out of recovery.
	c.congestionWindowCount = 0

	c.slowStartthresholdUpdated()
}

func (c *flowteleCubicSender) RenoBeta() float32 {
	// kNConnectionBeta is the backoff factor after loss for our N-connection
	// emulation, which emulates the effective backoff of an ensemble of N
	// TCP-Reno connections on a single loss event. The effective multiplier is
	// computed as:
	return (float32(c.numConnections) - 1. + renoBeta) / float32(c.numConnections)
}

// Called when we receive an ack. Normal TCP tracks how many packets one ack
// represents, but quic has a separate ack for each packet.
func (c *flowteleCubicSender) maybeIncreaseCwnd(ackedPacketNumber protocol.PacketNumber, ackedBytes protocol.ByteCount, bytesInFlight protocol.ByteCount) {
	// Do not increase the congestion window unless the sender is close to using
	// the current window.
	if !c.isCwndLimited(bytesInFlight) {
		c.cubic.OnApplicationLimited()
		// fmt.Println("FLOWTELE CC: application limited")
		c.adjustCongestionWindow()
		return
	}
	// fmt.Println("FLOWTELE CC: cwnd limited")
	if c.congestionWindow >= c.maxTCPCongestionWindow {
		return
	}
	if c.InSlowStart() {
		// TCP slow start, exponential growth, increase by one for each ACK.
		c.congestionWindow++
		return
	}
	if c.reno {
		// Classic Reno congestion avoidance.
		c.congestionWindowCount++
		// Divide by num_connections to smoothly increase the CWND at a faster
		// rate than conventional Reno.
		if protocol.PacketNumber(c.congestionWindowCount*protocol.ByteCount(c.numConnections)) >= c.congestionWindow {
			c.congestionWindow++
			c.congestionWindowCount = 0
		}
	} else {
		c.congestionWindow = utils.MinPacketNumber(c.maxTCPCongestionWindow, c.cubic.CongestionWindowAfterAck(c.congestionWindow, c.rttStats.MinRTT()))
		c.adjustCongestionWindow()
	}
}

func (c *flowteleCubicSender) isCwndLimited(bytesInFlight protocol.ByteCount) bool {
	congestionWindow := c.GetCongestionWindow()
	if bytesInFlight >= congestionWindow {
		return true
	}
	availableBytes := congestionWindow - bytesInFlight
	slowStartLimited := c.InSlowStart() && bytesInFlight > congestionWindow/2
	return slowStartLimited || availableBytes <= maxBurstBytes
}

// BandwidthEstimate returns the current bandwidth estimate
func (c *flowteleCubicSender) BandwidthEstimate() Bandwidth {
	srtt := c.rttStats.SmoothedRTT()
	if srtt == 0 {
		// If we haven't measured an rtt, the bandwidth estimate is unknown.
		return 0
	}
	return BandwidthFromDelta(c.GetCongestionWindow(), srtt)
}

// HybridSlowStart returns the hybrid slow start instance for testing
func (c *flowteleCubicSender) HybridSlowStart() *HybridSlowStart {
	return &c.hybridSlowStart
}

// SetNumEmulatedConnections sets the number of emulated connections
func (c *flowteleCubicSender) SetNumEmulatedConnections(n int) {
	c.numConnections = utils.Max(n, 1)
	c.cubic.SetNumConnections(c.numConnections)
}

// OnRetransmissionTimeout is called on an retransmission timeout
func (c *flowteleCubicSender) OnRetransmissionTimeout(packetsRetransmitted bool) {
	c.largestSentAtLastCutback = 0
	if !packetsRetransmitted {
		return
	}
	c.hybridSlowStart.Restart()
	c.cubic.Reset()
	c.slowstartThreshold = c.congestionWindow / 2
	c.congestionWindow = c.minCongestionWindow

	c.slowStartthresholdUpdated()
}

// OnConnectionMigration is called when the connection is migrated (?)
func (c *flowteleCubicSender) OnConnectionMigration() {
	c.hybridSlowStart.Restart()
	c.prr = PrrSender{}
	c.largestSentPacketNumber = 0
	c.largestAckedPacketNumber = 0
	c.largestSentAtLastCutback = 0
	c.lastCutbackExitedSlowstart = false
	c.cubic.Reset()
	c.congestionWindowCount = 0
	c.congestionWindow = c.initialCongestionWindow
	c.slowstartThreshold = c.initialMaxCongestionWindow
	c.maxTCPCongestionWindow = c.initialMaxCongestionWindow

	c.slowStartthresholdUpdated()
}

// SetSlowStartLargeReduction allows enabling the SSLR experiment
func (c *flowteleCubicSender) SetSlowStartLargeReduction(enabled bool) {
	c.slowStartLargeReduction = enabled
}

// RetransmissionDelay gives the time to retransmission
func (c *flowteleCubicSender) RetransmissionDelay() time.Duration {
	if c.rttStats.SmoothedRTT() == 0 {
		return 0
	}
	return c.rttStats.SmoothedRTT() + c.rttStats.MeanDeviation()*4
}
