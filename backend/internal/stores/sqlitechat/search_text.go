package sqlitechat

import (
	"strings"
	"unicode/utf8"
)

const searchChunkBytes = 8192
const searchOverlapBytes = 2048 // Covers the service's maximum 512-rune query.

type textPart struct {
	Seq  int64  `json:"seq"`
	Text string `json:"text"`
}

type pendingText struct {
	TurnID      string     `json:"turn,omitempty"`
	GroupKey    string     `json:"group,omitempty"`
	TurnOrdinal int64      `json:"ordinal,omitempty"`
	Parts       []textPart `json:"parts,omitempty"`
	HasNew      bool       `json:"new,omitempty"`
	PrefixBuilt bool       `json:"prefixBuilt,omitempty"`
}

func partsText(parts []textPart) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

func partsBytes(parts []textPart) int {
	size := 0
	for _, p := range parts {
		size += len(p.Text)
	}
	return size
}

func suffixParts(parts []textPart, limit int) []textPart {
	keep := []textPart{}
	used := 0
	for i := len(parts) - 1; i >= 0 && used < limit; i-- {
		p := parts[i]
		room := limit - used
		if len(p.Text) > room {
			start := len(p.Text) - room
			for start > 0 && !utf8.RuneStart(p.Text[start]) {
				start--
			}
			p.Text = p.Text[start:]
		}
		keep = append(keep, p)
		used += len(p.Text)
	}
	for i, j := 0, len(keep)-1; i < j; i, j = i+1, j-1 {
		keep[i], keep[j] = keep[j], keep[i]
	}
	return keep
}

// flush emits synchronously before changing the pending state so a failed write
// leaves the current chunk available to the transaction owner.
func (p *pendingText) flush(overlap bool, emit func(pendingText) error) error {
	if p.HasNew {
		if err := emit(*p); err != nil {
			return err
		}
	}
	if overlap {
		p.Parts = suffixParts(p.Parts, searchOverlapBytes)
	} else {
		p.Parts = nil
	}
	p.HasNew = false
	return nil
}

func (p *pendingText) appendText(seq, ordinal int64, turn, group, text string, emit func(pendingText) error) error {
	if p.GroupKey != group {
		if err := p.flush(false, emit); err != nil {
			return err
		}
		p.GroupKey = group
		p.TurnID = turn
		p.TurnOrdinal = ordinal
	}
	for len(text) > 0 {
		room := searchChunkBytes - partsBytes(p.Parts)
		take := len(text)
		if take > room {
			take = room
			for take > 0 && !utf8.RuneStart(text[take]) {
				take--
			}
		}
		if take == 0 {
			if err := p.flush(true, emit); err != nil {
				return err
			}
			continue
		}
		part := text[:take]
		text = text[take:]
		if n := len(p.Parts); n > 0 && p.Parts[n-1].Seq == seq {
			p.Parts[n-1].Text += part
		} else {
			p.Parts = append(p.Parts, textPart{seq, part})
		}
		p.HasNew = true
		if partsBytes(p.Parts) >= searchChunkBytes-utf8.UTFMax {
			if err := p.flush(true, emit); err != nil {
				return err
			}
		}
	}
	return nil
}
