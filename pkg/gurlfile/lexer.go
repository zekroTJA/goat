package gurlfile

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

type token int

const eof = rune(0)

const (
	// Special tokens
	tokILLEGAL token = iota
	tokEOF
	tokWS
	tokLF

	// Literals
	tokIDENT
	tokPARAMETER

	// Control Characters
	tokCOMMENT
	tokESCAPE
	tokSECTION
	tokDELIMITER
	tokBLOCKSTART
	tokBLOCKEND
	tokCOLON
	tokCOMMA
	tokASSIGNMENT

	// Types
	tokSTRING
	tokINTEGER
	tokFLOAT

	// Keywords
	tokUSE
)

type scanner struct {
	r           *bufio.Reader
	line        int
	lastlinepos int
	linepos     int
	pos         int
}

func newScanner(r io.Reader) *scanner {
	return &scanner{
		r: bufio.NewReader(r),
	}
}

func (t *scanner) read() rune {
	t.pos++
	t.linepos++

	r, _, err := t.r.ReadRune()
	if err != nil {
		return eof
	}

	if r == '\n' {
		t.line++
		t.lastlinepos = t.linepos - 1
		t.linepos = 0
	}

	return r
}

func (t *scanner) unread() {
	t.pos--
	if t.linepos == 0 {
		t.linepos = t.lastlinepos
		t.line--
	} else {
		t.linepos--
	}
	t.r.UnreadRune()
}

func (t *scanner) scan() (tk token, lit string) {
	r := t.read()

	if isWhitespace(r) {
		t.unread()
		return t.scanWhitespace()
	}

	if isLetter(r) {
		t.unread()
		return t.scanIdent()
	}

	if isDigit(r) {
		t.unread()
		return t.scanNumber()
	}

	switch r {
	case '/':
		return t.scanComment()
	case '"', '\'':
		t.unread()
		return t.scanString()
	case '-':
		return t.scanDash()
	case '#':
		return t.scanSection()
	case '{':
		return t.scanCurlyBrace()

	case '[':
		return tokBLOCKSTART, ""
	case ']':
		return tokBLOCKEND, ""
	case ':':
		return tokCOLON, ""
	case ',':
		return tokCOMMA, ""
	case '=':
		return tokASSIGNMENT, ""
	case '\n':
		return tokLF, ""
	case eof:
		return tokEOF, ""
	}

	return tokILLEGAL, string(r)
}

func (t *scanner) readToLF() string {
	var b bytes.Buffer

	for {
		r := t.read()
		if r == eof || r == '\n' {
			break
		}
		b.WriteRune(r)
	}

	return strings.TrimSpace(b.String())
}

func (t *scanner) scanWhitespace() (tk token, lit string) {
	var b bytes.Buffer
	b.WriteRune(t.read())

	for {
		if r := t.read(); r == eof {
			break
		} else if !isWhitespace(r) {
			t.unread()
			break
		} else {
			b.WriteRune(r)
		}
	}

	return tokWS, b.String()
}

func (t *scanner) skipToLF() {
	for {
		r := t.read()
		if r == '\n' || r == eof {
			break
		}
	}
}

func (t *scanner) scanComment() (tk token, lit string) {
	if t.read() != '/' {
		return tokILLEGAL, ""
	}

	t.skipToLF()

	return tokCOMMENT, ""
}

func (t *scanner) scanDash() (tk token, lit string) {
	tk, lit = t.scanNumber()
	if tk == tokINTEGER || tk == tokFLOAT {
		lit = "-" + lit
		return tk, lit
	}

	t.unread()
	tk, lit = t.scanDelimiter()

	return tk, lit
}

func (t *scanner) scanDelimiter() (tk token, lit string) {
	for i := 0; i < 2; i++ {
		if t.read() != '-' {
			return tokILLEGAL, ""
		}
	}

	t.skipToLF()

	return tokDELIMITER, ""
}

func (t *scanner) scanString() (tk token, lit string) {
	var b bytes.Buffer
	wrapper := rune(0)
	inString := false

	for {
		r := t.read()

		if r == eof || r == '\n' {
			if inString && wrapper != 0 {
				return tokILLEGAL, ""
			}
			break
		}

		if inString {
			if isWhitespace(r) && wrapper == 0 {
				break
			}
			if r == wrapper {
				break
			}
			b.WriteRune(r)
		} else {
			if isWhitespace(r) {
				continue
			}
			if isStringWrapper(r) {
				wrapper = r
			} else {
				b.WriteRune(r)
			}
			inString = true
		}
	}

	return tokSTRING, b.String()
}

func (t *scanner) scanUntilLF() string {
	var b bytes.Buffer

	for {
		r := t.read()
		if r == eof {
			t.unread()
			break
		}
		if r == '\n' {
			break
		}
		b.WriteRune(r)
	}

	return b.String()
}

func (t *scanner) scanSection() (tk token, lit string) {
	for i := 0; i < 2; i++ {
		if t.read() != '#' {
			return tokILLEGAL, ""
		}
	}

	for {
		r := t.read()
		if r != '#' {
			break
		}
	}

	t.unread()

	return tokSECTION, ""
}

func (t *scanner) scanIdent() (tk token, lit string) {
	var b bytes.Buffer
	b.WriteRune(t.read())

	for {
		if r := t.read(); r == eof {
			break
		} else if !isLetter(r) && !isDigit(r) && !isLiteralDelimiter(r) {
			t.unread()
			break
		} else {
			b.WriteRune(r)
		}
	}

	str := b.String()
	switch strings.ToLower(str) {
	case "use":
		return tokUSE, ""
	}

	return tokIDENT, str
}

func (t *scanner) scanCurlyBrace() (tk token, lit string) {
	r := t.read()
	if r == '{' {
		return t.scanParameter()
	}

	t.unread()

	return tokILLEGAL, ""
}

func (t *scanner) scanParameter() (tk token, lit string) {
	var b bytes.Buffer

	inStr := false
	strDelim := rune(0)
	level := 0

	for {
		r := t.read()

		if r == eof {
			return tokILLEGAL, ""
		}

		if !inStr && r == '{' {
			if rn := t.read(); rn == '{' {
				level++
			}
			t.unread()
		}

		if !inStr && r == '}' {
			if rn := t.read(); rn == '}' {
				if level == 0 {
					break
				}
				level--
			}
			t.unread()
		}

		if r == '"' || r == '`' {
			if inStr {
				if r == strDelim {
					inStr = false
				}
			} else {
				inStr = true
				strDelim = r
			}
		}

		b.WriteRune(r)
	}

	return tokPARAMETER, b.String()
}

func (t *scanner) scanNumber() (tk token, lit string) {
	var b bytes.Buffer
	tk = tokINTEGER

	for {
		r := t.read()

		if r == '.' {
			tk = tokFLOAT
		} else if r == '_' {
			continue
		} else if !isDigit(r) {
			t.unread()
			break
		}

		b.WriteRune(r)
	}

	if b.Len() == 0 {
		return tokILLEGAL, ""
	}

	return tk, b.String()
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t'
}

func isLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isLiteralDelimiter(r rune) bool {
	return r == '_' || r == '-'
}

func isStringWrapper(r rune) bool {
	return r == '"' || r == '\''
}
