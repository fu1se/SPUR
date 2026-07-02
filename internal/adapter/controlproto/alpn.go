package controlproto

// ALPN is the QUIC application protocol negotiated for the control
// channel. Both controlserver and controlclient depend on this neutral
// package rather than on each other.
const ALPN = "spur-control/1"
