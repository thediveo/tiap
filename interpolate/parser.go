// Copyright 2025 by Harald Albrecht
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy
// of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package interpolate

import (
	"errors"
	"fmt"
	"strings"
)

// Segment produces plain text upon request with all variables replaced by their
// values or alternate substitution values.
type Segment interface {
	Text(vars map[string]string) (string, error)
}

// Segments is a slice of Segment-implementing objects that produce plain text
// upon request while doing variable substitutions.
type Segments []Segment

// Text returns the plain text from the slice of segments, substituting variable
// values as necessary.
func (segs Segments) Text(vars map[string]string) (string, error) {
	var text strings.Builder
	for _, seg := range segs {
		segtext, err := seg.Text(vars)
		if err != nil {
			return "", err
		}
		text.WriteString(segtext)
	}
	return text.String(), nil
}

// PlainText is just what it says on the tin: plain text, no substitutes. In
// these days, we might call it organic, authentic, whatever.
type PlainText string

// Text returns plain text without any substitutions
func (pt PlainText) Text(map[string]string) (string, error) {
	return string(pt), nil
}

// Substitution represents a particular variable substitution.
type Substitution struct {
	VariableName string   // Name of the variable to substitute
	Operation    string   // either "" for a simple substitution, or one of "-", "?-", etc.
	AltValue     Segments // if non-zero, the alternative value to substitute the variable name with
}

// Text returns the plain text of this segment, substituting variable values
// recursively as necessary.
func (subst Substitution) Text(vars map[string]string) (string, error) {
	switch subst.Operation {
	case "":
		return vars[subst.VariableName], nil
	case "?":
		return subst.errorWhenUnset(vars)
	case ":?":
		return subst.errorWhenUnsetOrEmpty(vars)
	case "-":
		return subst.defaultWhenUnset(vars)
	case ":-":
		return subst.defaultWhenUnsetOrEmpty(vars)
	case "+":
		return subst.replaceWhenSet(vars)
	case ":+":
		return subst.replaceWhenSetAndNotEmpty(vars)
	}
	return "", fmt.Errorf("internal error: unknown interpolation operation '%s'", subst.Operation)
}

func (subst Substitution) errorWhenUnset(vars map[string]string) (string, error) {
	value, ok := vars[subst.VariableName]
	if !ok {
		errtext, err := subst.AltValue.Text(vars)
		if err != nil {
			return "", err
		}
		return "", errors.New(errtext)
	}
	return value, nil
}

func (subst Substitution) errorWhenUnsetOrEmpty(vars map[string]string) (string, error) {
	value, ok := vars[subst.VariableName]
	if !ok || value == "" {
		errtext, err := subst.AltValue.Text(vars)
		if err != nil {
			return "", err
		}
		return "", errors.New(errtext)
	}
	return value, nil
}

func (subst Substitution) defaultWhenUnset(vars map[string]string) (string, error) {
	value, ok := vars[subst.VariableName]
	if !ok {
		defaultValue, err := subst.AltValue.Text(vars)
		if err != nil {
			return "", err
		}
		return defaultValue, nil
	}
	return value, nil
}

func (subst Substitution) defaultWhenUnsetOrEmpty(vars map[string]string) (string, error) {
	value, ok := vars[subst.VariableName]
	if !ok || value == "" {
		defaultValue, err := subst.AltValue.Text(vars)
		if err != nil {
			return "", err
		}
		return defaultValue, nil
	}
	return value, nil
}

func (subst Substitution) replaceWhenSet(vars map[string]string) (string, error) {
	_, ok := vars[subst.VariableName]
	if !ok {
		return "", nil
	}
	return subst.AltValue.Text(vars)
}

func (subst Substitution) replaceWhenSetAndNotEmpty(vars map[string]string) (string, error) {
	value, ok := vars[subst.VariableName]
	if !ok || value == "" {
		return "", nil
	}
	return subst.AltValue.Text(vars)
}

// parse the specified string into a list of Segment objects if possible,
// otherwise return an error.
func parse(s string) (Segments, error) {
	segments, _, err := parseRecursive(s, false)
	return segments, err
}

func parseRecursive(s string, braced bool) (Segments, int, error) {
	segments := Segments{}
	var text strings.Builder
	for idx := 0; idx < len(s); idx++ {
		switch s[idx] {
		case '$':
			var err error
			idx, segments, err = parseVariable(s, idx, &text, segments)
			if err != nil {
				return nil, 0, err
			}
			continue
		case '}':
			if braced {
				if text.Len() != 0 {
					segments = append(segments, PlainText(text.String()))
				}
				return segments, idx, nil
			}
			fallthrough
		default:
			// ...copy character over to current text segment.
			text.WriteByte(s[idx])
		}
	}
	// If there is any pending text, add it as the final segment and then we're
	// done. Please note that if we reach the end of the string to parse in
	// braced mode, we've fallen off the string without the closing brace.
	if braced {
		return nil, 0, errors.New("unclosed braced variable substitution")
	}
	if text.Len() != 0 {
		segments = append(segments, PlainText(text.String()))
	}
	return segments, 0, nil
}

func parseVariable(s string, idx int, text *strings.Builder, segments Segments) (int, Segments, error) {
	// What is it: a $$, an unbraced/plain variable substitution, or a
	// braced variable substitution (which might recursively contain
	// further variable substitutions)?
	idx++
	if idx >= len(s) {
		// It's a lonely $ at the end of the string, treat it as an
		// error to warn the user.
		return 0, nil, errors.New("invalid stand-alone $")
	}
	ch := s[idx]
	if ch == '$' {
		// It's a $$ that boils down into a single $ that we add to the
		// current segment.
		text.WriteRune('$')
	} else if ch == '{' {
		var err error
		idx, segments, err = parseBraced(s, idx, text, segments)
		if err != nil {
			return 0, nil, err
		}
	} else if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' {
		idx, segments = parseVariableName(s, idx, text, segments)
	}
	return idx, segments, nil
}

func parseVariableName(s string, idx int, text *strings.Builder, segments Segments) (int, Segments) {
	// An unbraced name of a variable follows, so get its name. Note
	// that before we can emit a variable substitution segment, we
	// need to emit any pending text segment first.
	if text.Len() > 0 {
		segments = append(segments, PlainText(text.String()))
		text.Reset()
	}
	// Note: we already know there's at least one valid character in
	// the name.
	name := parseName(s[idx:])
	idx += len(name) - 1
	return idx, append(segments, Substitution{VariableName: name})
}

func parseBraced(s string, idx int, text *strings.Builder, segments Segments) (int, Segments, error) {
	// A braced name ${FOO} of a variable follows, so this is
	// getting a little bit more involved. First, get the name of
	// the variable.
	idx++
	name := parseName(s[idx:])
	if name == "" {
		// There's no (valid) variable name following, report an
		// error.
		return 0, nil, errors.New("missing variable name after ${")
	}
	idx += len(name)
	if idx >= len(s) {
		return 0, nil, errors.New("unterminated ${")
	}
	var op string
	switch ch := s[idx]; ch {
	case '}':
		// just a plain braced substitution, so emit the current
		// text segment where necessary and then add a variable
		// segment, and are done with it.
		if text.Len() > 0 {
			segments = append(segments, PlainText(text.String()))
			text.Reset()
		}
		return idx, append(segments, Substitution{VariableName: name}), nil
	case '?', '-', '+':
		op = string(ch)
	case ':':
		idx++
		if idx >= len(s) {
			return 0, nil, errors.New("incomplete variable substitution operation")
		}
		switch ch := s[idx]; ch {
		case '?', '-', '+':
			op = ":" + string(ch)
		default:
			return 0, nil, errors.New("invalid variable substitution operation")
		}
	default:
		return 0, nil, errors.New("invalid variable substitution operation")
	}
	// Get the substitution text, which might in turn contain more
	// substitutions...
	idx++
	segs, consumed, err := parseRecursive(s[idx:], true)
	if err != nil {
		return 0, nil, err
	}
	if text.Len() > 0 {
		segments = append(segments, PlainText(text.String()))
		text.Reset()
	}
	segments = append(segments, Substitution{
		VariableName: name,
		Operation:    op,
		AltValue:     segs,
	})
	idx += consumed
	return idx, segments, nil
}

// parseName returns the variable name; if the name is "" then no name could be
// found at the beginning of the specified string s.
func parseName(s string) string {
	for idx := 0; idx < len(s); idx++ {
		ch := s[idx]
		if ch == '_' {
			continue
		}
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= 'A' && ch <= 'Z' {
			continue
		}
		if idx > 0 && ch >= '0' && ch <= '9' {
			continue
		}
		return s[:idx]
	}
	return s
}
