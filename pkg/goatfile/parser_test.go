package goatfile

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/studio-b12/goat/pkg/errs"
)

// TODO: Add more unit tests

func TestParse_Simple(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		const raw = `GET https://example.com`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, 1, len(res.Tests))
		assert.Equal(t, "GET", res.Tests[0].(Request).Method)
		assert.Equal(t, "https://example.com", res.Tests[0].(Request).URI)
	})

	t.Run("multi", func(t *testing.T) {
		const raw = `
GET https://example1.com

---

POST https://example2.com
---
LOGIN https://example3.com
-----------------------
		
CHECK https://example4.com

[Body]
abc
		
---

CHECK https://example5.com

[Body]
abc
		
------
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)

		assert.Equal(t, 5, len(res.Tests))

		assert.Equal(t, "GET", res.Tests[0].(Request).Method)
		assert.Equal(t, "https://example1.com", res.Tests[0].(Request).URI)

		assert.Equal(t, "POST", res.Tests[1].(Request).Method)
		assert.Equal(t, "https://example2.com", res.Tests[1].(Request).URI)

		assert.Equal(t, "LOGIN", res.Tests[2].(Request).Method)
		assert.Equal(t, "https://example3.com", res.Tests[2].(Request).URI)

		assert.Equal(t, "CHECK", res.Tests[3].(Request).Method)
		assert.Equal(t, "https://example4.com", res.Tests[3].(Request).URI)

		assert.Equal(t, "CHECK", res.Tests[4].(Request).Method)
		assert.Equal(t, "https://example5.com", res.Tests[4].(Request).URI)
	})
}

func TestParse_Blocks(t *testing.T) {
	t.Run("single-single-block", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Header]
Key-1: value 1
key-2: value 2
		
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, 1, len(res.Tests))
		assert.Equal(t, "GET", res.Tests[0].(Request).Method)
		assert.Equal(t, "https://example.com", res.Tests[0].(Request).URI)
		assert.Equal(t, http.Header{
			"Key-1": []string{"value 1"},
			"Key-2": []string{"value 2"},
		}, res.Tests[0].(Request).Header)
	})

	t.Run("single-multi-block", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Header]
Key-1: value 1
key-2: value 2

[Body]
some
body

[queryparams]
keyInt = 2
keyString = "some string"
		
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, 1, len(res.Tests))
		assert.Equal(t, "GET", res.Tests[0].(Request).Method)
		assert.Equal(t, "https://example.com", res.Tests[0].(Request).URI)
		assert.Equal(t, http.Header{
			"Key-1": []string{"value 1"},
			"Key-2": []string{"value 2"},
		}, res.Tests[0].(Request).Header)
		assert.Equal(t, StringContent("some\nbody\n"), res.Tests[0].(Request).Body)
		assert.Equal(t, map[string]any{
			"keyInt":    int64(2),
			"keyString": "some string",
		}, res.Tests[0].(Request).QueryParams)
	})

	t.Run("single-invalidblockheader", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[invalidblock]
Key-1: value 1
key-2: value 2
		
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidBlockHeader, err)
	})

	t.Run("single-emptyblockheader", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[]
Key-1: value 1
key-2: value 2
		
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidBlockHeader)
	})

	t.Run("single-openblockheader", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams
Key-1: value 1
key-2: value 2
		
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidBlockHeader, err)
	})
}

func TestParse_BlockHeaders(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Header]
		
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, http.Header{}, res.Tests[0].(Request).Header)
	})

	t.Run("values", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Header]
key: value
key-2:  value 2
Some-Key-3: 		some value 3
SOME_KEY_4: 		§$%&/()=!§

multiple-1: value 1
multiple-1: value 2

		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, http.Header{
			"Key":        []string{"value"},
			"Key-2":      []string{"value 2"},
			"Some-Key-3": []string{"some value 3"},
			"Some_key_4": []string{"§$%&/()=!§"},
			"Multiple-1": []string{"value 1", "value 2"},
		}, res.Tests[0].(Request).Header)
	})

	t.Run("no-separator", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Header]
invalid
		
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidHeaderSeparator, err)
	})

	t.Run("invalid-key-format", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Header]
some key: value
		
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidHeaderSeparator, err)
	})

	t.Run("no-value", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Header]
some-key:
some-key-2:
		
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrNoHeaderValue, err)
	})
}

func TestParse_BlockRaw(t *testing.T) {
	t.Run("unescaped-empty", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, NoContent{}, res.Tests[0].(Request).Body)
	})

	t.Run("unescaped-EOF", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
some body content
some more content
`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\nsome more content\n"),
			res.Tests[0].(Request).Body)
	})

	t.Run("unescaped-newblock", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
some body content
some more content

[QueryParams]
`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\nsome more content\n"),
			res.Tests[0].(Request).Body)
	})

	t.Run("unescaped-newrequest", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
some body content
some more content

---
`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\nsome more content\n"),
			res.Tests[0].(Request).Body)
	})

	t.Run("unescaped-finaldelim", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
some body content
some more content
---`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\nsome more content"),
			res.Tests[0].(Request).Body)
	})

	t.Run("unescaped-section", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
some body content
some more content
### tests
`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\nsome more content"),
			res.Tests[0].(Request).Body)
	})

	t.Run("unescaped-logsection", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
some body content
some more content
##### some log section
`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\nsome more content"),
			res.Tests[0].(Request).Body)
	})

	t.Run("escaped-empty", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
´´´
´´´
`

		p := stringParser(swapTicks(raw))
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, NoContent{}, res.Tests[0].(Request).Body)
	})

	t.Run("escaped-EOF", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
´´´
some body content
some more content
´´´
`

		p := stringParser(swapTicks(raw))
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\nsome more content\n"),
			res.Tests[0].(Request).Body)
	})

	t.Run("escaped-newblock", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
´´´
some body content

[QueryParams]
some more content
´´´

[QueryParams]
`

		p := stringParser(swapTicks(raw))
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\n\n[QueryParams]\nsome more content\n"),
			res.Tests[0].(Request).Body)
	})

	t.Run("escaped-newrequest", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
´´´
some body content

---

some more content
´´´

---
`

		p := stringParser(swapTicks(raw))
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\n\n---\n\nsome more content\n"),
			res.Tests[0].(Request).Body)
	})

	t.Run("escaped-section", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
´´´
some body content

### setup

some more content
´´´

### tests
`

		p := stringParser(swapTicks(raw))
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent("some body content\n\n### setup\n\nsome more content\n"),
			res.Tests[0].(Request).Body)
	})

	t.Run("escaped-open", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Body]
´´´
some body content

---
`

		p := stringParser(swapTicks(raw))
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrOpenEscapeBlock, err)
	})

	t.Run("script general", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[Script]
assert(response.StatusCode == 200, "invalid status code");
var id = response.BodyJson.id;

---

`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t,
			StringContent(`assert(response.StatusCode == 200, "invalid status code");`+
				"\nvar id = response.BodyJson.id;\n"),
			res.Tests[0].(Request).Script)
	})
}

func TestParse_BlockValues(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
		
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, map[string]any{}, res.Tests[0].(Request).QueryParams)
	})

	t.Run("value-strings", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
string1 = "some string 1"
string2 =     "some string 2"
string3 = 		"some string 3" 
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, map[string]any{
			"string1": "some string 1",
			"string2": "some string 2",
			"string3": "some string 3",
		}, res.Tests[0].(Request).QueryParams)
	})

	t.Run("value-integer", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
int1 = 1
int2 = 1_000
int3 = -123
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, map[string]any{
			"int1": int64(1),
			"int2": int64(1000),
			"int3": int64(-123),
		}, res.Tests[0].(Request).QueryParams)
	})

	t.Run("value-float", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
float1 = 1.234
float2 = 1_000.234
float3 = 0.12
float4 = -12.34
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, map[string]any{
			"float1": float64(1.234),
			"float2": float64(1000.234),
			"float3": float64(0.12),
			"float4": float64(-12.34),
		}, res.Tests[0].(Request).QueryParams)
	})

	t.Run("value-boolean", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
bool1 = true
bool2 = false
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, map[string]any{
			"bool1": true,
			"bool2": false,
		}, res.Tests[0].(Request).QueryParams)
	})

	t.Run("value-array", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
arrayEmpty1 = []
arrayEmpty2 = [  	]

arrayString1 = ["some string"]
arrayString2 = ["some string", "another string","and another one"]

arrayInt1 = [1]
arrayInt2 = [1, 2,-3,	4_000]

arrayFloat1 = [1.23]
arrayFloat2 = [1.0, -1.1,1.234]

arrayMixed = ["a string", 2, 3.456, true]

arrayNested = [[1,2], [[true, false], "foo"]]

arrayMultiline = [
	"foo",
	"bar"
]

arrayLeadingComma1 = [true, false,]
arrayLeadingComma2 = [
	true, 
	false,
]
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, map[string]any{
			"arrayEmpty1":        []any(nil),
			"arrayEmpty2":        []any(nil),
			"arrayString1":       []any{"some string"},
			"arrayString2":       []any{"some string", "another string", "and another one"},
			"arrayInt1":          []any{int64(1)},
			"arrayInt2":          []any{int64(1), int64(2), int64(-3), int64(4_000)},
			"arrayFloat1":        []any{1.23},
			"arrayFloat2":        []any{1.0, -1.1, 1.234},
			"arrayMixed":         []any{"a string", int64(2), 3.456, true},
			"arrayNested":        []any{[]any{int64(1), int64(2)}, []any{[]any{true, false}, "foo"}},
			"arrayMultiline":     []any{"foo", "bar"},
			"arrayLeadingComma1": []any{true, false},
			"arrayLeadingComma2": []any{true, false},
		}, res.Tests[0].(Request).QueryParams)
	})

	t.Run("value-invalid-entry", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
invalid
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidBlockEntryAssignment, err)
	})

	t.Run("value-invalid-assignment", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
invalid = 
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidToken, err)
	})

	t.Run("value-invalid-string", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
invalid = "
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidToken, err)
	})

	t.Run("value-invalid-array-1", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
invalid = [
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidToken, err)
	})

	t.Run("value-invalid-array-2", func(t *testing.T) {
		const raw = `
		
GET https://example.com

[QueryParams]
invalid = [1, 2
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidToken, err)
	})
}

func TestParse_Comments(t *testing.T) {
	t.Run("uri", func(t *testing.T) {
		const raw = `
// Some comment
    // Some comment
GET https://example.com //another comment
// comment
// heyo
			`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, "GET", res.Tests[0].(Request).Method)
		assert.Equal(t, "https://example.com", res.Tests[0].(Request).URI)
	})

	t.Run("blocks", func(t *testing.T) {
		const raw = `
GET https://example.com

// some comment
[QueryParams] // block hader comment
key1 = "value" // another comment
key2 = 1.23 // comment
// in betweeny
arr = [ // comment
	1, // comment
	// another comment
	2 // comment
] // comment
			`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, map[string]any{
			"key1": "value",
			"key2": 1.23,
			"arr":  []any{int64(1), int64(2)},
		}, res.Tests[0].(Request).QueryParams)
	})

	t.Run("invlid-1", func(t *testing.T) {
		const raw = `
GET https://example.com

 /
			`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("invlid", func(t *testing.T) {
		const raw = `
GET https://example.com / foo
			`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidToken)
	})
}

func TestParse_Sections(t *testing.T) {
	t.Run("general", func(t *testing.T) {
		const raw = `
### Setup

GET https://example1.com
---
GET https://example2.com

### Setup-Each
GET https://example3.com
---
GET https://example4.com

###   	tests

GET https://example5.com

---

GET https://example6.com

	### teardown

GET https://example7.com
---
GET https://example8.com

---

### Teardown-Each

GET https://example9.com
---
GET https://example10.com

			`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)

		assert.Equal(t, "https://example1.com", res.Setup[0].(Request).URI)
		assert.Equal(t, "https://example2.com", res.Setup[1].(Request).URI)

		assert.Equal(t, "https://example3.com", res.SetupEach[0].(Request).URI)
		assert.Equal(t, "https://example4.com", res.SetupEach[1].(Request).URI)

		assert.Equal(t, "https://example5.com", res.Tests[0].(Request).URI)
		assert.Equal(t, "https://example6.com", res.Tests[1].(Request).URI)

		assert.Equal(t, "https://example7.com", res.Teardown[0].(Request).URI)
		assert.Equal(t, "https://example8.com", res.Teardown[1].(Request).URI)

		assert.Equal(t, "https://example9.com", res.TeardownEach[0].(Request).URI)
		assert.Equal(t, "https://example10.com", res.TeardownEach[1].(Request).URI)
	})

	t.Run("invalid-1", func(t *testing.T) {
		const raw = `
## Tests

GET https://example.com
			`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrIllegalCharacter, err)
	})

	t.Run("invalid-2", func(t *testing.T) {
		const raw = `
###

GET https://example.com
			`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidSection, err)
	})

	t.Run("invalid-3", func(t *testing.T) {
		const raw = `
### invalid-section

GET https://example.com
			`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidSection, err)
	})

	t.Run("invalid-4", func(t *testing.T) {
		const raw = `
### Tests Invalid

GET https://example.com
			`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidSection, err)
	})
}

func TestParse_Use(t *testing.T) {
	t.Run("general", func(t *testing.T) {
		const raw = `
use file1

use file2
use ../file3 // hey, a comment!

use "some file"

use 	  ../another/file
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, []string{
			"file1",
			"file2",
			"../file3",
			"some file",
			"../another/file",
		}, res.Imports)
	})

	t.Run("invalid-inclomplete", func(t *testing.T) {
		const raw = `
use
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidStringLiteral, err)
	})

	t.Run("invalid-empty-1", func(t *testing.T) {
		const raw = `
use   
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrEmptyUsePath, err)
	})

	t.Run("invalid-empty-2", func(t *testing.T) {
		const raw = `
use ""
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrEmptyUsePath, err)
	})

	t.Run("invalid-openstring", func(t *testing.T) {
		const raw = `
use "
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidStringLiteral, err)
	})

	t.Run("invalid-keyword", func(t *testing.T) {
		const raw = `
use"test"
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrInvalidStringLiteral, err)
	})
}

// --- Special Tests --------------------------------------

func TestParse_BlockTemplateValues(t *testing.T) {
	t.Run("variable-1", func(t *testing.T) {
		const raw = `
GET https://example.com

[Options]
someoption = {{.param}}
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, ParameterValue(".param"), res.Tests[0].(Request).Options["someoption"])
	})

	t.Run("variable-2", func(t *testing.T) {
		const raw = `
GET https://example.com

[Options]
someoption = {{ .param }}
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, ParameterValue(" .param "), res.Tests[0].(Request).Options["someoption"])
	})

	t.Run("wrapped", func(t *testing.T) {
		const raw = `
GET https://example.com

[Options]
someoption = {{ print {{.param1}} {{.param2}} }}
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, ParameterValue(" print {{.param1}} {{.param2}} "), res.Tests[0].(Request).Options["someoption"])
	})

	t.Run("instring-1", func(t *testing.T) {
		const raw = `
GET https://example.com

[Options]
someoption = {{ print "}}" }}
		`

		p := stringParser(raw)
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, ParameterValue(` print "}}" `), res.Tests[0].(Request).Options["someoption"])
	})

	t.Run("instring-2", func(t *testing.T) {
		const raw = `
GET https://example.com

[Options]
someoption = {{ print ´}}´ }}
		`

		p := stringParser(swapTicks(raw))
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, ParameterValue(" print `}}` "), res.Tests[0].(Request).Options["someoption"])
	})

	t.Run("instring-wrapped", func(t *testing.T) {
		const raw = `
GET https://example.com

[Options]
someoption = {{ print {{ "}}" }} }}
		`

		p := stringParser(swapTicks(raw))
		res, err := p.Parse()

		assert.Nil(t, err, err)
		assert.Equal(t, ParameterValue(` print {{ "}}" }} `), res.Tests[0].(Request).Options["someoption"])
	})
}

// See https://github.com/studio-b12/goat/issues/19
func TestParseMultipleSectionsCheck(t *testing.T) {
	t.Run("multiple-options", func(t *testing.T) {
		const raw = `
GET https://example.com

[Options]
someoption = "a"

[Header]
some: header

[Options]
anotheroption = "b"
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrSectionDefinedMultiple, err)
		var ewd errs.ErrorWithDetails
		assert.True(t, errors.As(err, &ewd))
		assert.Equal(t, fmt.Sprintf("[%s]:", optionNameOptions), ewd.Details.(string))
	})

	t.Run("multiple-header", func(t *testing.T) {
		const raw = `
GET https://example.com
[Header]
some: header

[Options]
someoption = "a"

[Header]
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrSectionDefinedMultiple, err)
		var ewd errs.ErrorWithDetails
		assert.True(t, errors.As(err, &ewd))
		assert.Equal(t, fmt.Sprintf("[%s]:", optionNameHeader), ewd.Details.(string))
	})

	t.Run("multiple-script", func(t *testing.T) {
		const raw = `
GET https://example.com
[Header]
some: header

[Options]
someoption = "a"

[Script]

[Script]

		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrSectionDefinedMultiple, err)
		var ewd errs.ErrorWithDetails
		assert.True(t, errors.As(err, &ewd))
		assert.Equal(t, fmt.Sprintf("[%s]:", optionNameScript), ewd.Details.(string))
	})

	t.Run("multiple-body", func(t *testing.T) {
		const raw = `
GET https://example.com
[Header]
some: header

[Options]
someoption = "a"

[Body]
foobar

[Script]

[Body]
barbazz
		`

		p := stringParser(raw)
		_, err := p.Parse()

		assert.ErrorIs(t, err, ErrSectionDefinedMultiple, err)
		var ewd errs.ErrorWithDetails
		assert.True(t, errors.As(err, &ewd))
		assert.Equal(t, fmt.Sprintf("[%s]:", optionNameBody), ewd.Details.(string))
	})
}

func TestLogSections(t *testing.T) {
	t.Run("general", func(t *testing.T) {
		const raw = `
##### Log section 1

GET https://example.com

[Script]
script stuff 1

---

##### Log section 2

GET https://example.com

[Script]
script stuff 2

##### Log section 3

GET https://example.com

[Script]
script stuff 3`

		p := stringParser(raw)
		gf, err := p.Parse()
		assert.Nil(t, err, err)
		assert.Equal(t, LogSection("Log section 1"), gf.Tests[0].(LogSection))
		assert.Equal(t, StringContent("script stuff 1\n"), gf.Tests[1].(Request).Script)
		assert.Equal(t, LogSection("Log section 2"), gf.Tests[2].(LogSection))
		assert.Equal(t, StringContent("script stuff 2\n"), gf.Tests[3].(Request).Script)
		assert.Equal(t, LogSection("Log section 3"), gf.Tests[4].(LogSection))
		assert.Equal(t, StringContent("script stuff 3"), gf.Tests[5].(Request).Script)
	})
}

// --- Helpers --------------------------------------------

func stringParser(raw string) *Parser {
	return NewParser(strings.NewReader(raw), ".")
}

func swapTicks(v string) string {
	return strings.ReplaceAll(v, "´", "`")
}
