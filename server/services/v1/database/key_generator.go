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

package database

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/buger/jsonparser"
	"github.com/tigrisdata/tigris/errors"
	"github.com/tigrisdata/tigris/keys"
	"github.com/tigrisdata/tigris/lib/uuid"
	"github.com/tigrisdata/tigris/schema"
	"github.com/tigrisdata/tigris/server/metadata"
	"github.com/tigrisdata/tigris/server/transaction"
	"github.com/tigrisdata/tigris/value"
)

var (
	zeroIntStringSlice  = []byte("0")
	zeroUUIDStringSlice = []byte(uuid.NullUUID.String())
	zeroTimeStringSlice = []byte(time.Time{}.Format(time.RFC3339Nano))
)

// keyGenerator is used to extract the keys from document and return keys.Key which will be used by Insert/Replace API.
// keyGenerator may need to modify the document in case autoGenerate is set for primary key fields. The keyGenerator
// makes the copy of the original document in case it needs to modify the document.
type keyGenerator struct {
	generator   *metadata.TableKeyGenerator
	document    []byte
	keysForResp []byte
	index       *schema.Index
	forceInsert bool
}

func newKeyGenerator(document []byte, generator *metadata.TableKeyGenerator, index *schema.Index) *keyGenerator {
	return &keyGenerator{
		document:  document,
		generator: generator,
		index:     index,
	}
}

func (k *keyGenerator) getKeysForResp() []byte {
	return []byte(fmt.Sprintf(`{%s}`, k.keysForResp))
}

// generate method also modifies the JSON document in case of autoGenerate primary key.
func (k *keyGenerator) generate(ctx context.Context, txMgr *transaction.Manager, encoder metadata.Encoder, table []byte) (keys.Key, error) {
	indexParts := make([]any, 0, len(k.index.Fields))
	for _, field := range k.index.Fields {
		jsonVal, dtp, _, err := jsonparser.Get(k.document, field.FieldName)
		autoGenerate := field.IsAutoGenerated() && (dtp == jsonparser.NotExist ||
			err == nil && (isNull(field.Type(), jsonVal) || dtp == jsonparser.Null))

		if !autoGenerate && err != nil {
			return nil, errors.InvalidArgument(fmt.Errorf("missing index key column(s) '%s': %w", field.FieldName, err).Error())
		}

		var v value.Value
		if autoGenerate {
			if jsonVal, v, err = k.get(ctx, txMgr, table, field); err != nil {
				return nil, err
			}
			if err = k.setKeyInDoc(field, jsonVal); err != nil {
				return nil, err
			}
			if field.Type() == schema.Int64Type || field.Type() == schema.DateTimeType {
				// if we have autogenerated pkey and if it is prone to conflict then force to use Insert API
				k.forceInsert = true
			}
		} else if v, err = value.NewValue(field.Type(), jsonVal); err != nil {
			return nil, err
		}

		k.addKeyToResp(field, jsonVal)
		indexParts = append(indexParts, v.AsInterface())
	}

	return encoder.EncodeKey(table, k.index, indexParts)
}

func (k *keyGenerator) setKeyInDoc(field *schema.Field, jsonVal []byte) error {
	jsonVal = k.getJsonQuotedValue(field.Type(), jsonVal)

	// as we are mutating the document, do not change original document.
	tmp := make([]byte, len(k.document))
	copy(tmp, k.document)
	k.document = tmp

	var err error
	k.document, err = jsonparser.Set(k.document, jsonVal, field.FieldName)
	return err
}

func (k *keyGenerator) addKeyToResp(field *schema.Field, jsonVal []byte) {
	jsonVal = k.getJsonQuotedValue(field.Type(), jsonVal)
	jsonKeyAndValue := []byte(fmt.Sprintf(`"%s":%s`, field.FieldName, jsonVal))

	if len(k.keysForResp) == 0 {
		k.keysForResp = jsonKeyAndValue
	} else {
		k.keysForResp = append(k.keysForResp, []byte(`,`)...)
		k.keysForResp = append(k.keysForResp, jsonKeyAndValue...)
	}
}

func (*keyGenerator) getJsonQuotedValue(fieldType schema.FieldType, jsonVal []byte) []byte {
	switch fieldType {
	case schema.StringType, schema.UUIDType, schema.ByteType, schema.DateTimeType:
		return []byte(fmt.Sprintf(`"%s"`, jsonVal))
	default:
		return jsonVal
	}
}

// isNull checks if the value is "zero" value of it's type.
func isNull(tp schema.FieldType, val []byte) bool {
	switch tp {
	case schema.Int32Type:
		return bytes.Equal(val, zeroIntStringSlice)
	case schema.Int64Type:
		return bytes.Equal(val, zeroIntStringSlice)
	case schema.UUIDType:
		return bytes.Equal(val, zeroUUIDStringSlice)
	case schema.DateTimeType:
		return bytes.Equal(val, zeroTimeStringSlice)
	case schema.StringType, schema.ByteType:
		return len(val) == 0
	}
	return false
}

// get returns generated id for the supported primary key fields. This method returns unquoted JSON values. This is to
// align with the json library that we are using as that returns unquoted strings as well. It is returning internal
// value as well so that we don't need to recalculate it from jsonVal.
func (k *keyGenerator) get(ctx context.Context, txMgr *transaction.Manager, table []byte, field *schema.Field) ([]byte, value.Value, error) {
	switch field.Type() {
	case schema.StringType, schema.UUIDType:
		val := value.NewStringValue(uuid.NewUUIDAsString(), nil)
		return []byte(val.Value), val, nil
	case schema.ByteType:
		val := value.NewBytesValue([]byte(uuid.NewUUIDAsString()))
		b64 := base64.StdEncoding.EncodeToString(*val)
		return []byte(b64), val, nil
	case schema.DateTimeType:
		// use timestamp nano to reduce the contention if multiple workers end up generating same timestamp.
		val := value.NewStringValue(time.Now().UTC().Format(time.RFC3339Nano), nil)
		return []byte(val.Value), val, nil
	case schema.Int64Type:
		// use timestamp nano to reduce the contention if multiple workers end up generating same timestamp.
		val := value.NewIntValue(time.Now().UTC().UnixNano())
		return []byte(fmt.Sprintf(`%d`, *val)), val, nil
	case schema.Int32Type:
		valueI32, err := k.generator.GenerateCounter(ctx, txMgr, table)
		if err != nil {
			return nil, nil, err
		}

		val := value.NewIntValue(int64(valueI32))
		return []byte(fmt.Sprintf(`%d`, *val)), val, nil
	}
	return nil, nil, errors.InvalidArgument("unsupported type found in auto-generator")
}
