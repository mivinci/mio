package mio

import (
	"encoding/binary"
	"errors"
)

/*
                     MIO Protocol Specification

						Copyright (c) 2023 x

MIO is a transport-agnostic universal stream multiplexing protocol that can be
used for NAT traversal, or RPC where streams need to be multiplexed over a
single connection. All MIO frames begin with a fixed 8-octet header followed by
a variable-length payload. Here's the frame format.

 0               1               2               3
 0 1 2 3 4 5 6 7 0 1 2 3 4 5 6 7 0 1 2 3 4 5 6 7 0 1 2 3 4 5 6 7
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|R|                       Stream Identifier                     |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                               |     |H|W|R|F|S|
|    Payload Size / Window Size / Error Code    |  R  |F|N|S|I|Y|
|                                               |     |R|D|T|N|N|
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               :
:                             Payload                           :
:                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

R: reserved bit(s), MUST remain 0x0 and MUST be ignored when receiving.
Stream Indentifier: a 31-bit unsigned integer identifying the stream that should
                    handle this frame.

The 4-6th octets depened on the flag bits. It MUST be

Payload Size  when the SYN bit is set without the WND bit being set indicating
			  the payload size. When both the SYN bit and the HFR bit are set
			  it is also the payload size but of the last frame from the sender.
Window Size   when the WND bit is set without the SYN bit being set indicating
			  a window size increment of the underlying transport. When both the
			  SYN bit and the is WND bit are set, it indicates a window size
			  increment of a stream. When the SYN bit, the RST bit and the WND
			  bit are all set, it means that the sender wants to open a stream
			  with both a stream id and a window size specified.
Error Code    when either the RST bit or FIN bit is set meaning the sender closed
              the stream or the underlying connection with an error code specified.

An error code MUST be one of

NO_ERROR           0x0  when no error occurs.
INTERNAL_ERROR     0x1  when an unexpected error occurs.
FLOW_CONTROL_ERROR 0x2  when the sender violates window size.
CLOSED             0x3  when the targeting stream is closed.
REFUSED            0x4  when the receiver refuses to open or process a stream.
EXHAUSTED          0x5	when the stream ids run out.
TRANSPORT_CLOSED   0x6  when the underlying transport is closed.
...

*/

const (
	Syn Flag = 1 << iota
	Fin
	RST
	WND
	HFR

	flagStreamOpen         = RST | WND
	flagStreamClose        = RST
	flagStreamContinuation = Syn
	flagStreamHalfClose    = Syn | HFR
	flagStreamWindowUpdate = Syn | WND
)

type Flag uint8

func (f Flag) Isset(flag Flag) bool {
	return f&flag == flag
}

type Header [8]byte

func (h *Header) SetFlag(flag Flag) {
	h[7] |= byte(flag)
}

func (h Header) Flag() Flag {
	return Flag(h[7])
}

func (h *Header) SetStreamID(v uint32) {
	binary.BigEndian.PutUint32(h[:], v&0x7fffffff)
}

func (h Header) StreamID() uint32 {
	return binary.BigEndian.Uint32(h[:]) & 0x7fffffff
}

func (h *Header) SetSize(v uint32) {
	h[4] = byte(v >> 16)
	h[5] = byte(v >> 8)
	h[6] = byte(v)
}

func (h Header) Size() uint32 {
	return binary.BigEndian.Uint32(h[4:]) >> 8
}

func (h *Header) SetErrorCode(v ErrorCode) {
	h.SetSize(uint32(v))
}

func (h Header) ErrorCode() ErrorCode {
	return ErrorCode(h.Size())
}

type ErrorCode int

const (
	NoError ErrorCode = iota
	InternalError
	FLowControlError
	Closed
	Refused
	Exhausted
	TransportClosed
)

var (
	ErrInternal        = errors.New("internal error")
	ErrFlowControl     = errors.New("flow control violation")
	ErrClosed          = errors.New("closed")
	ErrRefused         = errors.New("refused")
	ErrExhausted       = errors.New("exhausted")
	ErrTransportClosed = errors.New("transport closed")
)

func fromErrorCode(code ErrorCode) error {
	switch code {
	case NoError:
	case InternalError:
		return ErrInternal
	case FLowControlError:
		return ErrFlowControl
	case Closed:
		return ErrClosed
	case Refused:
		return ErrRefused
	case TransportClosed:
		return ErrTransportClosed
	default:
		return nil // ignore invalid error code
	}
	return nil
}
