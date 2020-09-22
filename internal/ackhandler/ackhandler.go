package ackhandler

import (
	"github.com/lucas-clemente/quic-go/internal/congestion"
	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
	"github.com/lucas-clemente/quic-go/qlog"
	"github.com/lucas-clemente/quic-go/quictrace"
)

func NewFlowteleAckHandler(
	initialPacketNumber protocol.PacketNumber,
	rttStats *congestion.RTTStats,
	pers protocol.Perspective,
	traceCallback func(quictrace.Event),
	qlogger qlog.Tracer,
	logger utils.Logger,
	version protocol.VersionNumber,
	flowteleSignalInterface *congestion.FlowteleSignalInterface,
) (SentPacketHandler, ReceivedPacketHandler) {
	sph := newFlowteleSentPacketHandler(initialPacketNumber, rttStats, pers, traceCallback, qlogger, logger, flowteleSignalInterface)
	return sph, newReceivedPacketHandler(sph, rttStats, logger, version)
}

func NewAckHandler(
	initialPacketNumber protocol.PacketNumber,
	rttStats *congestion.RTTStats,
	pers protocol.Perspective,
	traceCallback func(quictrace.Event),
	qlogger qlog.Tracer,
	logger utils.Logger,
	version protocol.VersionNumber,
) (SentPacketHandler, ReceivedPacketHandler) {
	sph := newSentPacketHandler(initialPacketNumber, rttStats, pers, traceCallback, qlogger, logger)
	return sph, newReceivedPacketHandler(sph, rttStats, logger, version)
}
