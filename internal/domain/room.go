package domain

// Room is a named, long-lived pairing between exactly two specific
// peers. Unlike a pairing code (usecase.PairingCodeTTL, single guest
// use) or a Network (many members, mesh IPs, TUN), a Room persists
// indefinitely once both members have joined and always resolves to
// "the other member" for whichever of the two asks — the point is
// letting two people who already know each other skip re-exchanging a
// peer ID or code on every reconnect.
type Room struct {
	Name        string
	InviteToken string
	Members     []PeerID
}

// HasMember reports whether peer already belongs to the room.
func (r Room) HasMember(peer PeerID) bool {
	for _, m := range r.Members {
		if m == peer {
			return true
		}
	}
	return false
}

// OtherMember returns whichever member of a full (two-member) room isn't
// peer. ok is false if the room doesn't have a second member yet, or if
// peer isn't a member at all.
func (r Room) OtherMember(peer PeerID) (other PeerID, ok bool) {
	if !r.HasMember(peer) || len(r.Members) < 2 {
		return "", false
	}
	for _, m := range r.Members {
		if m != peer {
			return m, true
		}
	}
	return "", false
}
