package gurlfile

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

func applyTemplate(raw string, params any) (string, error) {
	tmpl, err := template.New("").
		Funcs(builtinFuncsMap).
		Option("missingkey=error").
		Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parsing template failed: %s", err.Error())
	}

	var out bytes.Buffer
	err = tmpl.Execute(&out, params)
	if err != nil {
		return "", fmt.Errorf("executing template failed: %s", err.Error())
	}

	return out.String(), nil
}

func applyTemplateToArray(arr []any, params any) (err error) {
	for i, v := range arr {
		switch vt := v.(type) {
		case string:
			arr[i], err = applyTemplate(vt, params)
		case []any:
			err = applyTemplateToArray(vt, params)
		default:
			continue
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func removeComments(raw string) string {
	lines := strings.Split(raw, "\n")

	for i, line := range lines {
		cidx := strings.Index(line, "//")
		if cidx == -1 {
			continue
		}

		if cidx > 0 {
			if line[cidx-1] == ' ' {
				cidx -= 1
			} else {
				continue
			}
		}

		lines[i] = line[:cidx]
	}

	return strings.Join(lines, "\n")
}

func unquote(v string) string {
	if len(v) > 1 && (v[0] == '"' && v[len(v)-1] == '"' ||
		v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}

	return v
}

func extend(v string, ext string) string {
	if filepath.Ext(v) == "" {
		return v + "." + ext
	}

	return v
}

func crlf2lf(v string) string {
	return strings.ReplaceAll(v, "\r\n", "\n")
}
