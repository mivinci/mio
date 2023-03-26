package mio

import "testing"

func TestFrame(t *testing.T) {
	h := Header{}
	id := 42
	size := 7890
	flag := Syn | WND

	h.SetStreamID(uint32(id))
	h.SetSize(uint32(size))
	h.SetFlag(flag)

	if i := h.StreamID(); i != uint32(id) {
		t.Fatalf("stream id expected %d got %d", id, i)
	}
	if s := h.Size(); s != uint32(size) {
		t.Fatalf("size expected %d got %d", size, s)
	}
	if f := h.Flag(); !f.Isset(flag) {
		t.Fatalf("flag expected %d got %d", flag, f)
	}
}
