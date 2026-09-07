package cache

import "testing"

func TestBitmapSetAndGet(t *testing.T) {
	b := newBitmap(0)
	if b.get(0) || b.get(1000) {
		t.Fatal("a fresh bitmap should have nothing set")
	}
	if !b.set(5) {
		t.Error("setting an unset block should report a change")
	}
	if b.set(5) {
		t.Error("setting an already-set block should report no change")
	}
	if !b.get(5) || b.get(4) || b.get(6) {
		t.Error("set(5) touched the wrong blocks")
	}
	if got := b.count(); got != 1 {
		t.Errorf("count = %d; want 1", got)
	}
}

func TestBitmapSetRangeReportsNewBlocks(t *testing.T) {
	b := newBitmap(0)
	if n := b.setRange(10, 19); n != 10 {
		t.Errorf("setRange(10,19) = %d; want 10", n)
	}
	if n := b.setRange(15, 24); n != 5 {
		t.Errorf("overlapping setRange = %d; want 5 new blocks", n)
	}
	if got := b.count(); got != 15 {
		t.Errorf("count = %d; want 15", got)
	}
}

func TestBitmapClearFrom(t *testing.T) {
	b := newBitmap(0)
	b.setRange(0, 99)
	if n := b.clearFrom(40); n != 60 {
		t.Errorf("clearFrom(40) = %d; want 60", n)
	}
	if b.get(40) || !b.get(39) {
		t.Error("clearFrom cleared the wrong side of the boundary")
	}
	if got := b.count(); got != 40 {
		t.Errorf("count after clear = %d; want 40", got)
	}
}

func TestBitmapSerialisation(t *testing.T) {
	b := newBitmap(0)
	for _, i := range []int{0, 1, 63, 64, 65, 200} {
		b.set(i)
	}
	round := bitmapFromBytes(b.bytes(), 201)
	for _, i := range []int{0, 1, 63, 64, 65, 200} {
		if !round.get(i) {
			t.Errorf("block %d was lost in serialisation", i)
		}
	}
	if round.count() != b.count() {
		t.Errorf("count after round trip = %d; want %d", round.count(), b.count())
	}
	if round.get(2) || round.get(100) {
		t.Error("serialisation invented blocks that were never set")
	}
}

func TestBitmapGrowsOnDemand(t *testing.T) {
	b := newBitmap(1)
	b.set(5000)
	if !b.get(5000) {
		t.Error("bitmap did not grow to hold block 5000")
	}
	if b.count() != 1 {
		t.Errorf("count = %d; want 1", b.count())
	}
}
