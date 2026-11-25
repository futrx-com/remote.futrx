package chat

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/futrx-com/remote.futrx.com/internal/service/project"
	"golang.org/x/text/unicode/norm"
)

var workspaceCharEquivalents = map[rune]rune{
	'ى': 'ي', 'ة': 'ه', 'ی': 'ي', 'ک': 'ك', 'ڪ': 'ك', 'ٰ': 'ا',
}

func foldWorkspaceText(text string) string {
	var folded strings.Builder
	folded.Grow(len(text))
	for _, ch := range text {
		if ch <= 0xffff {
			lower := unicode.ToLower(ch)
			if lower <= 0x7f {
				ch = lower
			} else {
				for _, base := range norm.NFD.String(string(lower)) {
					if base <= 0xffff {
						ch = base
					}
					break
				}
			}
		}
		if equivalent, ok := workspaceCharEquivalents[ch]; ok {
			ch = equivalent
		}
		switch {
		case ch >= '٠' && ch <= '٩':
			ch = '0' + ch - '٠'
		case ch >= '۰' && ch <= '۹':
			ch = '0' + ch - '۰'
		}
		folded.WriteRune(ch)
	}
	return folded.String()
}

func workspaceWord(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsNumber(ch) || unicode.IsMark(ch)
}

var (
	workspaceCamelBoundary = regexp.MustCompile(`(\p{Ll}|\p{N})(\p{Lu})`)
	workspaceDigitBoundary = regexp.MustCompile(`(\p{L})(\p{N})`)
	workspaceWordRun       = regexp.MustCompile(`[\p{L}\p{N}\p{M}]+`)
)

func workspaceSearchTokens(query string) [][]uint16 {
	query = workspaceCamelBoundary.ReplaceAllString(query, "$1 $2")
	query = workspaceDigitBoundary.ReplaceAllString(query, "$1 $2")
	var tokens [][]uint16
	for _, chunk := range strings.FieldsFunc(foldWorkspaceText(query), func(ch rune) bool {
		return unicode.IsSpace(ch) || ch == '\uFEFF'
	}) {
		words := workspaceWordRun.FindAllString(chunk, -1)
		if len(words) == 0 {
			words = []string{chunk}
		}
		for _, word := range words {
			tokens = append(tokens, utf16.Encode([]rune(word)))
		}
	}
	return tokens
}

func matchWorkspaceField(field []uint16, tokens [][]uint16) (int, []TextSpan) {
	score := 0
	var spans []TextSpan
	for _, token := range tokens {
		points, ranges := matchWorkspaceToken(field, token)
		if points == 0 {
			return 0, nil
		}
		score += points
		spans = append(spans, ranges...)
	}
	slices.SortStableFunc(spans, func(a, b TextSpan) int { return a.Start - b.Start })
	merged := spans[:0]
	for _, span := range spans {
		if n := len(merged); n > 0 && span.Start <= merged[n-1].End {
			merged[n-1].End = max(merged[n-1].End, span.End)
		} else {
			merged = append(merged, span)
		}
	}
	return score, merged
}

func matchWorkspaceToken(field, token []uint16) (int, []TextSpan) {
	for start := 0; start+len(token) <= len(field); start++ {
		if !slices.Equal(field[start:start+len(token)], token) {
			continue
		}
		score := 45
		switch {
		case len(field) == len(token):
			score = 120
		case start == 0:
			score = 90
		case !workspaceWord(rune(field[start-1])):
			score = 70
		}
		return score, []TextSpan{{Start: start, End: start + len(token)}}
	}
	if len(token) >= 4 {
		budget := 1
		if len(token) > 6 {
			budget = 2
		}
		for start := 0; start < len(field); {
			ch, width := workspaceRuneAt(field, start)
			if !workspaceWord(ch) {
				start += width
				continue
			}
			end := start + width
			for end < len(field) {
				ch, width = workspaceRuneAt(field, end)
				if !workspaceWord(ch) {
					break
				}
				end += width
			}
			if workspaceWithinEdits(field[start:end], token, budget) {
				return 14, []TextSpan{{Start: start, End: end}}
			}
			start = end
		}
	}
	if len(token) > len(field) {
		return 0, nil
	}
	if spans := workspaceAcronym(field, token); spans != nil {
		return 22, spans
	}
	return 0, nil
}

func workspaceRuneAt(text []uint16, start int) (rune, int) {
	ch := rune(text[start])
	if start+1 < len(text) && utf16.IsSurrogate(ch) {
		if combined := utf16.DecodeRune(ch, rune(text[start+1])); combined != unicode.ReplacementChar {
			return combined, 2
		}
	}
	return ch, 1
}

func workspaceWithinEdits(left, right []uint16, budget int) bool {
	if len(left)-len(right) > budget || len(right)-len(left) > budget {
		return false
	}
	prev, curr := make([]int, len(right)+1), make([]int, len(right)+1)
	for i := range prev {
		prev[i] = i
	}
	for i, ch := range left {
		curr[0] = i + 1
		best := curr[0]
		for j, target := range right {
			substitution := prev[j]
			if ch != target {
				substitution++
			}
			curr[j+1] = min(curr[j]+1, prev[j+1]+1, substitution)
			best = min(best, curr[j+1])
		}
		if best > budget {
			return false
		}
		prev, curr = curr, prev
	}
	return prev[len(right)] <= budget
}

func workspaceAcronym(field, token []uint16) []TextSpan {
	positions := make([]TextSpan, len(token))
	failed := make(map[[2]int]bool)
	var place func(int, int) bool
	place = func(index, cursor int) bool {
		if index == len(token) {
			return true
		}
		key := [2]int{index, cursor}
		if failed[key] {
			return false
		}
		char := token[index]
		for at := cursor; at <= len(field)-(len(token)-index); at++ {
			if field[at] != char {
				continue
			}
			continues := index > 0 && at == cursor
			if continues || at == 0 || !workspaceWord(rune(field[at-1])) {
				positions[index] = TextSpan{Start: at, End: at + 1}
				if place(index+1, at+1) {
					return true
				}
			}
		}
		failed[key] = true
		return false
	}
	if !place(0, 0) {
		return nil
	}
	return positions
}

func scoreWorkspaceChat(meta Meta, p *project.Meta, skills []string, tokens [][]uint16) (WorkspaceSearchHit, bool) {
	hit := WorkspaceSearchHit{Chat: meta, Project: p, TitleSpans: []TextSpan{}, MatchedField: "none"}
	if len(tokens) == 0 {
		return hit, true
	}
	projectText := ""
	if p != nil {
		projectText = p.Name + " " + p.Slug
	}
	fields := [...]struct {
		name, text string
		weight     float64
	}{
		{"title", meta.Title, 1}, {"project", projectText, .55}, {"path", meta.Cwd, .4},
		{"skill", strings.Join(skills, " "), .35}, {"model", meta.Model, .3},
	}
	best, total := 0.0, 0.0
	for _, field := range fields {
		score, spans := matchWorkspaceField(utf16.Encode([]rune(foldWorkspaceText(field.text))), tokens)
		weighted := float64(score) * field.weight
		total += weighted
		if weighted > best {
			best = weighted
			hit.MatchedField = field.name
		}
		if field.name == "title" && score > 0 {
			hit.TitleSpans = spans
		}
	}
	hit.Score = best + (total-best)*.15
	return hit, best > 0
}
