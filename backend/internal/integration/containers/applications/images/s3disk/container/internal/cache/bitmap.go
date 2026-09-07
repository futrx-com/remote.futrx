package cache

// bitmap tracks which fixed-size blocks of a file are present on local disk.
type bitmap struct {
	words []uint64
	n     int
}

func newBitmap(n int) *bitmap {
	if n < 0 {
		n = 0
	}
	return &bitmap{words: make([]uint64, (n+63)/64), n: n}
}

func (b *bitmap) grow(n int) {
	if n <= b.n {
		return
	}
	need := (n + 63) / 64
	for len(b.words) < need {
		b.words = append(b.words, 0)
	}
	b.n = n
}

func (b *bitmap) get(i int) bool {
	if i < 0 || i >= b.n {
		return false
	}
	return b.words[i/64]&(1<<uint(i%64)) != 0
}

// set marks a block present and reports whether it was not already.
func (b *bitmap) set(i int) bool {
	if i < 0 {
		return false
	}
	b.grow(i + 1)
	if b.words[i/64]&(1<<uint(i%64)) != 0 {
		return false
	}
	b.words[i/64] |= 1 << uint(i%64)
	return true
}

// setRange marks blocks [lo, hi] present and returns how many changed, so the
// cache can keep an accurate disk-usage total.
func (b *bitmap) setRange(lo, hi int) int {
	n := 0
	for i := lo; i <= hi; i++ {
		if b.set(i) {
			n++
		}
	}
	return n
}

// clear marks a single block absent and reports whether it had been present.
func (b *bitmap) clear(i int) bool {
	if i < 0 || i >= b.n {
		return false
	}
	if b.words[i/64]&(1<<uint(i%64)) == 0 {
		return false
	}
	b.words[i/64] &^= 1 << uint(i%64)
	return true
}

// clearFrom drops every block from i onwards and returns how many were set.
func (b *bitmap) clearFrom(i int) int {
	n := 0
	for j := i; j < b.n; j++ {
		if j < 0 {
			continue
		}
		if b.words[j/64]&(1<<uint(j%64)) != 0 {
			n++
		}
		b.words[j/64] &^= 1 << uint(j%64)
	}
	return n
}

func (b *bitmap) clearAll() {
	for i := range b.words {
		b.words[i] = 0
	}
}

func (b *bitmap) count() int {
	n := 0
	for _, w := range b.words {
		for ; w != 0; w &= w - 1 {
			n++
		}
	}
	return n
}

// bytes serialises the bitmap for the on-disk cache index.
func (b *bitmap) bytes() []byte {
	out := make([]byte, len(b.words)*8)
	for i, w := range b.words {
		for j := 0; j < 8; j++ {
			out[i*8+j] = byte(w >> uint(8*j))
		}
	}
	return out
}

func bitmapFromBytes(data []byte, n int) *bitmap {
	b := newBitmap(n)
	for i := 0; i < len(data) && i/8 < len(b.words); i++ {
		b.words[i/8] |= uint64(data[i]) << uint(8*(i%8))
	}
	return b
}
