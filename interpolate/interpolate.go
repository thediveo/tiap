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
	"fmt"
	"strconv"
)

// Variables interpolates all string values in the passed (recursive) map with
// values from the pass variables as necessary. It returns a new (recursive) map
// with the interpolated results.
func Variables(data map[string]any, vars map[string]string) (map[string]any, error) {
	result := map[string]any{}
	for key, value := range data {
		interpolValue, err := recursively(value, "", vars)
		if err != nil {
			return nil, err
		}
		result[key] = interpolValue
	}
	return result, nil
}

// recursively interpolate string values, string values inside mappings, and
// string values inside sequences.
func recursively(data any, path Path, vars map[string]string) (any, error) {
	switch value := data.(type) {
	case string:
		return interpolateString(value, path, vars)
	case map[string]any:
		return interpolateMapping(value, path, vars)
	case []any:
		return interpolateSequence(value, path, vars)
	default:
		return value, nil
	}
}

// interpolateString returns the interpolated string, or an error.
func interpolateString(value string, path Path, vars map[string]string) (string, error) {
	segments, err := parse(value)
	if err != nil {
		return "", fmt.Errorf("error in '%s': %w", string(path), err)
	}
	return segments.Text(vars)
}

// interpolateMapping recursively interpolates the values in the mapping.
func interpolateMapping(values map[string]any, path Path, vars map[string]string) (map[string]any, error) {
	result := map[string]any{}
	for key, value := range values {
		interpolValue, err := recursively(value, path.Append(key), vars)
		if err != nil {
			return nil, err
		}
		result[key] = interpolValue
	}
	return result, nil
}

// interpolateSequence recursively interpolates the values of the sequence.
func interpolateSequence(values []any, path Path, vars map[string]string) ([]any, error) {
	result := make([]any, 0, len(values))
	for idx, value := range values {
		interpolValue, err := recursively(value, path.AppendIndex(idx), vars)
		if err != nil {
			return nil, err
		}
		result = append(result, interpolValue)
	}
	return result, nil
}

// Path represents the path to a scalar.
type Path string

// Append the name of a mapping key or a scalar to the path, returning the new
// Path.
func (p Path) Append(name string) Path {
	if p == "" {
		return Path(name)
	}
	return Path(string(p) + "." + name)
}

// Append the index of an element to the path, returning the new Path.
func (p Path) AppendIndex(idx int) Path {
	if p == "" {
		return Path("[" + strconv.FormatInt(int64(idx), 10) + "]")
	}
	return Path(string(p) + "[" + strconv.FormatInt(int64(idx), 10) + "]")
}
