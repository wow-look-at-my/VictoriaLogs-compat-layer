package proxy

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// evalProbe evaluates a constant Prometheus expression that Grafana sends as a
// liveness probe (e.g. "vector(1)+vector(1)" → 2). Supports integer/float
// literals, unary minus, the binary operators + - * /, parentheses, and the
// no-op vector()/scalar() wrappers. Returns ok=false if the expression
// references series, labels, or anything else outside that subset.
func evalProbe(q string) (float64, bool) {
	p := &probeParser{lex: probeLexer{s: strings.TrimSpace(q)}}
	if err := p.advance(); err != nil {
		return 0, false
	}
	v, err := p.parseExpr()
	if err != nil || p.cur.kind != probeEOF {
		return 0, false
	}
	return v, true
}

type probeTokenKind int

const (
	probeNumber probeTokenKind = iota
	probeIdent
	probePlus
	probeMinus
	probeStar
	probeSlash
	probeLParen
	probeRParen
	probeEOF
)

type probeToken struct {
	kind probeTokenKind
	num  float64
	val  string
}

type probeLexer struct {
	s   string
	pos int
}

func (l *probeLexer) next() (probeToken, error) {
	for l.pos < len(l.s) && unicode.IsSpace(rune(l.s[l.pos])) {
		l.pos++
	}
	if l.pos >= len(l.s) {
		return probeToken{kind: probeEOF}, nil
	}
	c := l.s[l.pos]
	switch c {
	case '+':
		l.pos++
		return probeToken{kind: probePlus}, nil
	case '-':
		l.pos++
		return probeToken{kind: probeMinus}, nil
	case '*':
		l.pos++
		return probeToken{kind: probeStar}, nil
	case '/':
		l.pos++
		return probeToken{kind: probeSlash}, nil
	case '(':
		l.pos++
		return probeToken{kind: probeLParen}, nil
	case ')':
		l.pos++
		return probeToken{kind: probeRParen}, nil
	}
	if (c >= '0' && c <= '9') || c == '.' {
		start := l.pos
		for l.pos < len(l.s) && ((l.s[l.pos] >= '0' && l.s[l.pos] <= '9') || l.s[l.pos] == '.') {
			l.pos++
		}
		num, err := strconv.ParseFloat(l.s[start:l.pos], 64)
		if err != nil {
			return probeToken{}, err
		}
		return probeToken{kind: probeNumber, num: num}, nil
	}
	if unicode.IsLetter(rune(c)) || c == '_' {
		start := l.pos
		for l.pos < len(l.s) {
			r := rune(l.s[l.pos])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				break
			}
			l.pos++
		}
		return probeToken{kind: probeIdent, val: l.s[start:l.pos]}, nil
	}
	return probeToken{}, fmt.Errorf("unexpected character %q at offset %d", c, l.pos)
}

type probeParser struct {
	lex probeLexer
	cur probeToken
}

func (p *probeParser) advance() error {
	t, err := p.lex.next()
	if err != nil {
		return err
	}
	p.cur = t
	return nil
}

func (p *probeParser) parseExpr() (float64, error) {
	v, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.cur.kind == probePlus || p.cur.kind == probeMinus {
		op := p.cur.kind
		if err := p.advance(); err != nil {
			return 0, err
		}
		r, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == probePlus {
			v += r
		} else {
			v -= r
		}
	}
	return v, nil
}

func (p *probeParser) parseTerm() (float64, error) {
	v, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for p.cur.kind == probeStar || p.cur.kind == probeSlash {
		op := p.cur.kind
		if err := p.advance(); err != nil {
			return 0, err
		}
		r, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if op == probeStar {
			v *= r
		} else {
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			v /= r
		}
	}
	return v, nil
}

func (p *probeParser) parseFactor() (float64, error) {
	switch p.cur.kind {
	case probeNumber:
		v := p.cur.num
		if err := p.advance(); err != nil {
			return 0, err
		}
		return v, nil
	case probeMinus:
		if err := p.advance(); err != nil {
			return 0, err
		}
		v, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -v, nil
	case probePlus:
		if err := p.advance(); err != nil {
			return 0, err
		}
		return p.parseFactor()
	case probeLParen:
		if err := p.advance(); err != nil {
			return 0, err
		}
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.cur.kind != probeRParen {
			return 0, fmt.Errorf("expected ')'")
		}
		if err := p.advance(); err != nil {
			return 0, err
		}
		return v, nil
	case probeIdent:
		name := p.cur.val
		if err := p.advance(); err != nil {
			return 0, err
		}
		if p.cur.kind != probeLParen {
			return 0, fmt.Errorf("expected '(' after %s", name)
		}
		if err := p.advance(); err != nil {
			return 0, err
		}
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.cur.kind != probeRParen {
			return 0, fmt.Errorf("expected ')' to close %s(", name)
		}
		if err := p.advance(); err != nil {
			return 0, err
		}
		switch name {
		case "vector", "scalar":
			return v, nil
		default:
			return 0, fmt.Errorf("unsupported function %s", name)
		}
	}
	return 0, fmt.Errorf("unexpected token")
}
