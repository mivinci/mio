# MIO

MIO is a transport-agnostic universal stream multiplexing protocol that can be used for NAT traversal, or RPC where streams need to be multiplexed over a single connection. All MIO frames begin with a fixed 8-octet header followed by a variable-length payload.



## Specification

Check out [frame.go](./frame.go)

## License

MIO is MIT licensed