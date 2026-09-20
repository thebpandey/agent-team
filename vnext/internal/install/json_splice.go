package install

import (
	"bytes"
	"encoding/json"
	"errors"
)

type jsonSpan struct{ start, end int }

type jsonNode struct {
	jsonSpan
	kind    byte
	items   []*jsonNode
	members map[string]*jsonNode
}

func (n *jsonNode) member(name string) *jsonNode {
	if n == nil {
		return nil
	}
	return n.members[name]
}

func (n *jsonNode) removals(owned map[*jsonNode]bool) []jsonSpan {
	var spans []jsonSpan
	for first := 0; first < len(n.items); {
		if !owned[n.items[first]] {
			first++
			continue
		}
		last := first
		for last+1 < len(n.items) && owned[n.items[last+1]] {
			last++
		}
		switch {
		case last+1 < len(n.items):
			spans = append(spans, jsonSpan{n.items[first].start, n.items[last+1].start})
		case first > 0:
			spans = append(spans, jsonSpan{n.items[first-1].end, n.items[last].end})
		default:
			spans = append(spans, jsonSpan{n.items[first].start, n.items[last].end})
		}
		first = last + 1
	}
	return spans
}

func parseJSONSpans(raw []byte) (*jsonNode, error) {
	p := jsonSpanParser{raw: raw}
	n, err := p.value()
	p.space()
	if err != nil || p.at != len(raw) {
		return nil, errors.New("invalid json")
	}
	return n, nil
}

type jsonSpanParser struct {
	raw []byte
	at  int
}

func (p *jsonSpanParser) space() {
	for p.at < len(p.raw) && bytes.IndexByte([]byte(" \t\r\n"), p.raw[p.at]) >= 0 {
		p.at++
	}
}

func (p *jsonSpanParser) value() (*jsonNode, error) {
	p.space()
	start := p.at
	if p.at >= len(p.raw) {
		return nil, errors.New("missing value")
	}
	switch p.raw[p.at] {
	case '{':
		return p.object(start)
	case '[':
		return p.array(start)
	case '"':
		if _, err := p.string(); err != nil {
			return nil, err
		}
	default:
		for p.at < len(p.raw) && !bytes.ContainsRune([]byte(",]} \t\r\n"), rune(p.raw[p.at])) {
			p.at++
		}
		if p.at == start {
			return nil, errors.New("invalid value")
		}
	}
	n := &jsonNode{jsonSpan: jsonSpan{start, p.at}, kind: p.raw[start]}
	if !json.Valid(p.raw[start:p.at]) {
		return nil, errors.New("invalid value")
	}
	return n, nil
}

func (p *jsonSpanParser) string() (string, error) {
	start := p.at
	p.at++
	for p.at < len(p.raw) {
		if p.raw[p.at] == '\\' {
			p.at += 2
			continue
		}
		p.at++
		if p.raw[p.at-1] == '"' {
			var value string
			if json.Unmarshal(p.raw[start:p.at], &value) != nil {
				return "", errors.New("invalid string")
			}
			return value, nil
		}
	}
	return "", errors.New("unterminated string")
}

func (p *jsonSpanParser) object(start int) (*jsonNode, error) {
	n := &jsonNode{jsonSpan: jsonSpan{start: start}, kind: '{', members: map[string]*jsonNode{}}
	p.at++
	for {
		p.space()
		if p.at < len(p.raw) && p.raw[p.at] == '}' {
			p.at++
			n.end = p.at
			return n, nil
		}
		if p.at >= len(p.raw) || p.raw[p.at] != '"' {
			return nil, errors.New("invalid object")
		}
		key, err := p.string()
		if err != nil {
			return nil, err
		}
		p.space()
		if p.at >= len(p.raw) || p.raw[p.at] != ':' {
			return nil, errors.New("invalid object")
		}
		p.at++
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		if _, exists := n.members[key]; exists {
			return nil, errors.New("duplicate key")
		}
		n.members[key] = value
		p.space()
		if p.at < len(p.raw) && p.raw[p.at] == ',' {
			p.at++
			continue
		}
		if p.at >= len(p.raw) || p.raw[p.at] != '}' {
			return nil, errors.New("invalid object")
		}
	}
}

func (p *jsonSpanParser) array(start int) (*jsonNode, error) {
	n := &jsonNode{jsonSpan: jsonSpan{start: start}, kind: '['}
	p.at++
	for {
		p.space()
		if p.at < len(p.raw) && p.raw[p.at] == ']' {
			p.at++
			n.end = p.at
			return n, nil
		}
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		n.items = append(n.items, value)
		p.space()
		if p.at < len(p.raw) && p.raw[p.at] == ',' {
			p.at++
			continue
		}
		if p.at >= len(p.raw) || p.raw[p.at] != ']' {
			return nil, errors.New("invalid array")
		}
	}
}
