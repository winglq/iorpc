package iorpc

import (
	"sync"
	"time"
)

// ConnStats provides connection statistics. Applied to both gorpc.Client
// and gorpc.Server.
//
// Use stats returned from ConnStats.Snapshot() on live Client and / or Server,
// since the original stats can be updated by concurrently running goroutines.
type ConnStats struct {
	// The number of rpc calls performed.
	RPCCalls uint64

	// The total aggregate time for all rpc calls in milliseconds.
	//
	// This time can be used for calculating the average response time
	// per RPC:
	//     avgRPCTtime = RPCTime / RPCCalls
	RPCTime uint64

	// The number of bytes of body written to the underlying connection.
	BodyWritten uint64

	// The number of bytes of body read from the underlying connection.
	BodyRead uint64

	// The number of bytes written of head (the start line, error and headers) to the underlying connection.
	HeadWritten uint64

	// The number of bytes of head (start line and headers) read from the underlying connection.
	HeadRead uint64

	// The number of Read() calls.
	ReadCalls uint64

	// The number of Read() errors.
	ReadErrors uint64

	// The number of Write() calls.
	WriteCalls uint64

	// The number of Write() errors.
	WriteErrors uint64

	// The number of Dial() calls.
	DialCalls uint64

	// The number of Dial() errors.
	DialErrors uint64

	// The number of Accept() calls.
	AcceptCalls uint64

	// The number of Accept() errors.
	AcceptErrors uint64

	// lock is for 386 builds. See https://github.com/valyala/gorpc/issues/5 .
	lock sync.Mutex
}

// AvgRPCTime returns the average RPC execution time.
//
// Use stats returned from ConnStats.Snapshot() on live Client and / or Server,
// since the original stats can be updated by concurrently running goroutines.
func (cs *ConnStats) AvgRPCTime() time.Duration {
	return time.Duration(float64(cs.RPCTime)/float64(cs.RPCCalls)) * time.Millisecond
}

// AvgRPCBytes returns the average bytes sent / received per RPC.
//
// Use stats returned from ConnStats.Snapshot() on live Client and / or Server,
// since the original stats can be updated by concurrently running goroutines.
func (cs *ConnStats) AvgRPCBytes() (send float64, recv float64) {
	return float64(cs.HeadWritten+cs.BodyWritten) / float64(cs.RPCCalls), float64(cs.HeadRead+cs.BodyRead) / float64(cs.RPCCalls)
}

// AvgRPCHeadBytes returns the average bytes of header sent / received per RPC.
//
// Use stats returned from ConnStats.Snapshot() on live Client and / or Server,
// since the original stats can be updated by concurrently running goroutines.
func (cs *ConnStats) AvgRPCHeadBytes() (send float64, recv float64) {
	return float64(cs.HeadWritten) / float64(cs.RPCCalls), float64(cs.HeadRead) / float64(cs.RPCCalls)
}

// AvgRPCBodyBytes returns the average bytes of body sent / received per RPC.
//
// Use stats returned from ConnStats.Snapshot() on live Client and / or Server,
// since the original stats can be updated by concurrently running goroutines.
func (cs *ConnStats) AvgRPCBodyBytes() (send float64, recv float64) {
	return float64(cs.BodyWritten) / float64(cs.RPCCalls), float64(cs.BodyRead) / float64(cs.RPCCalls)
}

// AvgRPCCalls returns the average number of write() / read() syscalls per PRC.
//
// Use stats returned from ConnStats.Snapshot() on live Client and / or Server,
// since the original stats can be updated by concurrently running goroutines.
func (cs *ConnStats) AvgRPCCalls() (write float64, read float64) {
	return float64(cs.WriteCalls) / float64(cs.RPCCalls), float64(cs.ReadCalls) / float64(cs.RPCCalls)
}
