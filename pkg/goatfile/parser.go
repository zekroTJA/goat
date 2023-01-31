package goatfile

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/studio-b12/goat/pkg/errs"
)

// Parser parses a Goatfile.
type Parser struct {
	s       *scanner
	prevPos readerPos
	buf     struct {
		tok token  // last read token
		lit string // last read literal
		n   int    // buffer size
	}
}

// NewParser returns a new Parser scanning from
// the given Reader.
func NewParser(r io.Reader) *Parser {
	return &Parser{s: newScanner(r)}
}

// Parse parses a Goatfile from the specified source.
func (t *Parser) Parse() (gf Goatfile, err error) {
	defer func() {
		err = t.wrapErr(err)
	}()

	for {
		tok, lit := t.scan()
		_ = lit

		switch tok {
		case tokCOMMENT, tokWS, tokLF:
			continue

		case tokIDENT, tokSTRING:
			t.unscan()
			err = t.parseRequest(&gf.Tests)

		case tokUSE:
			err = t.parseUse(&gf)

		case tokSECTION:
			err = t.parseSection(&gf)
		case tokEOF:
			return gf, nil

		case tokBLOCKSTART:
			return Goatfile{}, ErrBlockOutOfRequest

		default:
			return Goatfile{}, ErrIllegalCharacter
		}

		if err != nil {
			return Goatfile{}, err
		}
	}
}

func (t *Parser) scan() (tok token, lit string) {
	if t.buf.n != 0 {
		t.buf.n = 0
		return t.buf.tok, t.buf.lit
	}

	t.prevPos = t.s.readerPos

	t.buf.tok, t.buf.lit = t.s.scan()
	if t.buf.tok == tokCOMMENT {
		t.buf.tok = tokLF
		t.buf.lit = ""
	}

	return t.buf.tok, t.buf.lit
}

func (t *Parser) unscan() {
	t.buf.n = 1
}

func (t *Parser) scanSkipWS() (tok token, lit string) {
	tok, lit = t.scan()
	if tok == tokWS {
		return t.scan()
	}

	return tok, lit
}

func (t *Parser) wrapErr(err error) error {
	if err == nil {
		return nil
	}

	pErr := ParseError{}
	pErr.Inner = err
	pErr.Line = t.prevPos.line
	pErr.LinePos = t.prevPos.linepos

	return pErr
}

func (t *Parser) parseUse(gf *Goatfile) error {
	tk, _ := t.scan()
	if tk != tokWS {
		return ErrInvalidStringLiteral
	}

	tk, lit := t.s.scanString()
	if tk == tokILLEGAL {
		return ErrInvalidStringLiteral
	}

	if lit == "" {
		return ErrEmptyUsePath
	}

	gf.Imports = append(gf.Imports, lit)

	return nil
}

func (t *Parser) parseSection(gf *Goatfile) error {
	name := strings.TrimSpace(t.s.readToLF())

	var r *[]Request

	switch strings.ToLower(name) {
	case sectionNameSetup:
		r = &gf.Setup
	case sectionNameSetupEach:
		r = &gf.SetupEach
	case sectionNameTests:
		r = &gf.Tests
	case sectionNameTeardown:
		r = &gf.Teardown
	case sectionNameTeardownEach:
		r = &gf.TeardownEach
	default:
		return ErrInvalidSection
	}

	for {
		tok, _ := t.scan()
		if tok == tokLF || tok == tokWS {
			continue
		}

		if tok == tokEOF || tok == tokSECTION {
			t.unscan()
			break
		}

		t.unscan()
		err := t.parseRequest(r)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Parser) parseRequest(section *[]Request) (err error) {
	req := newRequest()

	// parse header

	tok, lit := t.scan()
	if tok != tokIDENT && tok != tokSTRING || lit == "" {
		return ErrInvalidRequestMethod
	}
	req.Method = lit

	tok, _ = t.scan()
	if tok != tokWS && tok != tokLF {
		return ErrNoRequestURI
	}

	tok, lit = t.s.scanString()
	if tok != tokSTRING || lit == "" {
		return ErrNoRequestURI
	}
	req.URI = lit

loop:
	for {
		tok, _ = t.scan()

		switch tok {
		case tokBLOCKSTART:
			err = t.parseBlock(&req)

		case tokWS, tokLF:
			continue loop
		case tokEOF, tokSECTION:
			t.unscan()
			break loop
		case tokDELIMITER:
			break loop

		default:
			err = errs.WithSuffix(ErrInvalidToken, "(request)")
		}

		if err != nil {
			return err
		}
	}

	*section = append(*section, req)
	return nil
}

func (t *Parser) parseBlock(req *Request) error {
	var blockHeader string

	tok, lit := t.scanSkipWS()
	if tok != tokIDENT || lit == "" {
		return ErrInvalidBlockHeader
	}
	blockHeader = lit

	tok, _ = t.scan()
	if tok != tokBLOCKEND {
		return ErrInvalidBlockHeader
	}

	tok, _ = t.scanSkipWS()
	if tok != tokLF {
		return errs.WithSuffix(ErrInvalidToken, "(block)")
	}

	switch strings.ToLower(blockHeader) {

	case optionNameQueryParams:
		data, err := t.parseBlockEntries()
		if err != nil {
			return err
		}
		req.QueryParams = data

	case optionNameHeader, optionNameHeaders:
		err := t.parseHeaders(req.Header)
		if err != nil {
			return err
		}

	case optionNameBody:
		raw, err := t.parseRaw()
		if err != nil {
			return err
		}
		if raw != "" {
			req.Body = []byte(raw)
		}

	case optionNameScript:
		raw, err := t.parseRaw()
		if err != nil {
			return err
		}
		req.Script = raw

	case optionNameOptions:
		data, err := t.parseBlockEntries()
		if err != nil {
			return err
		}
		req.Options = data

	default:
		return errs.WithSuffix(ErrInvalidBlockHeader,
			fmt.Sprintf("('%s')", blockHeader))
	}

	return nil
}

func (t *Parser) parseBlockEntries() (map[string]any, error) {
	m := map[string]any{}

	for {
		tok, lit := t.scanSkipWS()
		if tok == tokLF {
			continue
		}
		if tok == tokDELIMITER || tok == tokEOF || tok == tokBLOCKSTART || tok == tokSECTION {
			t.unscan()
			break
		}

		if tok != tokIDENT {
			return nil, ErrInvalidBlockEntryAssignment
		}

		key := lit

		tok, _ = t.scanSkipWS()
		if tok != tokASSIGNMENT {
			return nil, ErrInvalidBlockEntryAssignment
		}

		val, err := t.parseValue()
		if err != nil {
			return nil, err
		}

		m[key] = val
	}

	return m, nil
}

func (t *Parser) parseHeaders(header http.Header) error {
	for {
		tok, lit := t.scanSkipWS()
		if tok == tokLF {
			continue
		}
		if tok == tokDELIMITER || tok == tokEOF || tok == tokBLOCKSTART || tok == tokSECTION {
			t.unscan()
			break
		}

		if tok != tokIDENT {
			return ErrInvalidHeaderKey
		}
		key := lit

		tok, _ = t.scanSkipWS()
		if tok != tokCOLON {
			return ErrInvalidHeaderSeparator
		}

		val := strings.TrimSpace(t.s.scanUntilLF())
		if val == "" {
			return ErrNoHeaderValue
		}

		header.Add(key, val)
	}

	return nil
}

func (t *Parser) parseRaw() (string, error) {
	var out bytes.Buffer

	inEscape := false

	for {
		r := t.s.read()

		if !inEscape {
			if out.Len() > 3 && string(out.Bytes()[out.Len()-4:]) == "\n---" {
				t.buf.tok = tokDELIMITER
				t.buf.lit = ""
				t.unscan()
				out.Truncate(out.Len() - 4)
				break
			}
			if out.Len() > 1 && string(out.Bytes()[out.Len()-2:]) == "\n[" {
				t.buf.tok = tokBLOCKSTART
				t.buf.lit = ""
				t.s.unread()
				t.unscan()
				out.Truncate(out.Len() - 2)
				break
			}
			if out.Len() > 3 && string(out.Bytes()[out.Len()-4:]) == "\n###" {
				t.buf.tok = tokSECTION
				t.buf.lit = ""
				t.unscan()
				out.Truncate(out.Len() - 4)
				break
			}
		}

		if r == eof {
			if inEscape {
				return "", ErrOpenEscapeBlock
			}
			t.s.unread()
			break
		}

		out.WriteRune(r)

		if out.Len() == 4 && out.String() == "```\n" ||
			out.Len() > 3 && string(out.Bytes()[out.Len()-4:]) == "\n```" {
			if inEscape {
				inEscape = false
			} else {
				inEscape = true
			}
			if out.Len() == 3 {
				out.Reset()
			} else {
				out.Truncate(out.Len() - 4)
			}
			continue
		}

	}

	return out.String(), nil
}

func (t *Parser) parseValue() (any, error) {
	tok, lit := t.scanSkipWS()

	switch tok {
	case tokINTEGER:
		return strconv.ParseInt(lit, 10, 64)
	case tokFLOAT:
		return strconv.ParseFloat(lit, 64)
	case tokSTRING:
		return lit, nil
	case tokIDENT:
		switch lit {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, errs.WithSuffix(ErrInvalidLiteral, "(boolean expression expected)")
		}
	case tokBLOCKSTART:
		return t.parseArray()
	case tokPARAMETER:
		return ParameterValue(lit), nil
	}

	return nil, errs.WithSuffix(ErrInvalidToken, "(value)")
}

func (t *Parser) parseArray() ([]any, error) {
	var arr []any

loop:
	for {
		tok, _ := t.scanSkipWS()
		switch tok {
		case tokBLOCKEND:
			break loop
		case tokCOMMA, tokLF:
			continue loop
		}

		t.unscan()

		val, err := t.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, val)
	}

	return arr, nil
}