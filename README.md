# The MIO Protocol

MIO is a transport-agnostic universal stream multiplexing protocol that can be used for NAT traversal, or RPC where streams need to be multiplexed over a single connection.

## Features

- Can be used over any `io.ReadWriteCloser` interface.
- Streams are multiplexed over a single connection.
- Stream-specific flow control using siding windows.
- Streams can be opened from either side (hence NAT traversal).
- Customizable protocol parameters.

## Motivation

I need a stream multiplexing protocol over TCP for my microservice framework (coming soon). I've tried to use HTTP/2 at first but found it impossible to get myself coding at its stream layer, either because of the lack of APIs like how [golang.org/x/net/http2](https://golang.org/x/net/http2) is or of a too heavy dependency like [gRPC](https://github.com/grpc/grpc-go). What I need is just a tiny library that focus on the stream multiplexing thing with a simple fast stream-specific flow control mechanism and can be used as equivalent APIs with the [QUIC](https://en.wikipedia.org/wiki/QUIC) protocol hence eventually making the transport layer of my microservice framework pluggable using Go interfaces.

MIO over TCP is like QUIC over UDP :) Given this, it is possible for applications to switch their transport implementation according to the network environment they are at.

## Specification

The specification is placed in the source code, check out [frame.go](./frame.go) :)

## Tradeoffs

When using TCP, since all MIO streams share one single TCP connection and due to the [head-of-line blocking](https://en.wikipedia.org/wiki/Head-of-line_blocking) problem, it is recommended to use MIO only if your application has a large number of small-payload streams like RPC or a tunnel for NAT traversal. Otherwise, MIO works bad af :)

Although this library can be used for any `io.ReadWriteCloser` interface, it is designed to mainly be used over TCP. Therefore, MIO uses only a per-stream sliding window for simple flow control, the frame order and other mechanisms making the stream reliable are guaranteed by TCP behind the scene.



## Security considerations

Issue or PR me when you find some.

## References

- [RFC 7540 - Hypertext Transfer Protocol Version 2](https://httpwg.org/specs/rfc7540.html)
- [RFC 6455 - The WebSocket Protocol](https://www.rfc-editor.org/rfc/rfc6455)
- [RFC 9293 - Transmission Control Protocol](https://www.ietf.org/rfc/rfc9293.html)
- [inconshreveable/muxado](https://github.com/inconshreveable/muxado)
- [rancher/remotedialer](https://github.com/rancher/remotedialer)

## License

MIO is MIT licensed.