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

package keys

import (
	"bytes"
	"fmt"
	"unsafe"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
)

// Key is an interface that provides an encoded key which will be used for storing Key, Value in FDB. The Key has two
// elements, the first set of bytes is the encoded table name and the remaining is the actual index values.
type Key interface {
	fmt.Stringer

	// Table is logical representation of namespace, database, and collection. Table can also be used as a random
	// bytes to group related data together.
	Table() []byte
	// IndexParts is the remaining parts of the key which is appended to the table to form a FDB key. Different packages
	// may be using this differently. The encoder in metadata package use it by adding the index identifier and value(s)
	// associated with a single or composite index. The identifier is used to differentiate whether encoding is for primary
	// key index or some other user defined index. Essentially to encode key for a given row. Some other internal packages
	// may use this as simply to form a key without any significance of index identifier.
	IndexParts() []interface{}
	// SerializeToBytes follows the ordering of how the Key is persisted in database so to compare a Key call this method
	// get bytes and compare it with raw bytes stored in database.
	SerializeToBytes() []byte
	// CompareBytes compares the serialized form of keys. It returns 0 if p == input, -1 if p < input, and +1 if p > input.
	// A nil argument is equivalent to an empty slice.
	CompareBytes(input []byte) int
}

type tableKey struct {
	table      []byte
	indexParts []interface{}
}

// NewKey returns the Key.
func NewKey(table []byte, indexParts ...interface{}) Key {
	return &tableKey{
		table:      table,
		indexParts: indexParts,
	}
}

func (p *tableKey) Table() []byte {
	return p.table
}

func (p *tableKey) IndexParts() []interface{} {
	return p.indexParts
}

func (p *tableKey) String() string {
	return fmt.Sprintf("table:%v, indexKeyAndValues:%v", string(p.table), p.indexParts)
}

// SerializeToBytes follows the ordering of how the Key is persisted in database so to compare a Key call this method
// get bytes and compare it with raw bytes stored in database.
func (p *tableKey) SerializeToBytes() []byte {
	if len(p.indexParts) == 0 {
		return p.table
	}

	sb := subspace.FromBytes(p.table)
	return sb.Pack(*(*tuple.Tuple)(unsafe.Pointer(&p.indexParts)))
}

// CompareBytes compares the serialized form of keys. It returns 0 if p == input, -1 if p < input, and +1 if p > input.
// A nil argument is equivalent to an empty slice.
func (p *tableKey) CompareBytes(input []byte) int {
	return bytes.Compare(p.SerializeToBytes(), input)
}

func FromBinary(table []byte, fdbKey []byte) (Key, error) {
	sb := subspace.FromBytes(table)
	tp, err := sb.Unpack(fdb.Key(fdbKey))
	if err != nil {
		return nil, err
	}

	return NewKey(table, *(*[]interface{})(unsafe.Pointer(&tp))...), nil
}
