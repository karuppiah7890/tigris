// Copyright 2022-2023 Tigris Data, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"bytes"
	"io"
	"strings"
	"text/template"

	jsoniter "github.com/json-iterator/go"
	"github.com/tigrisdata/tigris/lib/container"
	ulog "github.com/tigrisdata/tigris/util/log"
)

const (
	ObjFlattenDelimiter = "."
)

// Version of this build.
var Version string

// Service program name used in logging and monitoring.
var Service string = "tigris-server"

func ExecTemplate(w io.Writer, tmpl string, vars interface{}) error {
	t, err := template.New("exec_template").Funcs(template.FuncMap{"repeat": strings.Repeat}).Parse(tmpl)
	if ulog.E(err) {
		return err
	}

	if err = t.Execute(w, vars); ulog.E(err) {
		return err
	}

	return nil
}

func MapToJSON(data map[string]any) ([]byte, error) {
	var buffer bytes.Buffer

	encoder := jsoniter.NewEncoder(&buffer)

	if err := encoder.Encode(data); ulog.E(err) {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func JSONToMap(data []byte) (map[string]any, error) {
	var decoded map[string]any

	decoder := jsoniter.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := decoder.Decode(&decoded); ulog.E(err) {
		return nil, err
	}

	return decoded, nil
}

func FlatMap(data map[string]any, notFlat container.HashSet) map[string]any {
	resp := make(map[string]any)
	flatMap("", data, resp, notFlat)
	return resp
}

func flatMap(key string, obj map[string]any, resp map[string]any, notFlat container.HashSet) {
	if key != "" {
		key += ObjFlattenDelimiter
	}

	for k, v := range obj {
		switch vMap := v.(type) {
		case map[string]any:
			if notFlat.Contains(key + k) {
				resp[key+k] = v
			} else {
				flatMap(key+k, vMap, resp, notFlat)
			}
		default:
			resp[key+k] = v
		}
	}
}

func UnFlatMap(flat map[string]any) map[string]any {
	result := make(map[string]any)

	for k, v := range flat {
		keys := strings.Split(k, ObjFlattenDelimiter)
		m := result

		for i := 0; i < len(keys)-1; i++ {
			if m[keys[i]] == nil {
				m[keys[i]] = make(map[string]any)
			}

			m = m[keys[i]].(map[string]any)
		}

		if v != nil {
			m[keys[len(keys)-1]] = v
		}
	}

	return result
}
