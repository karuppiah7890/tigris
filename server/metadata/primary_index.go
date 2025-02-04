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

package metadata

import (
	"context"

	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog/log"
	"github.com/tigrisdata/tigris/errors"
	"github.com/tigrisdata/tigris/internal"
	"github.com/tigrisdata/tigris/keys"
	"github.com/tigrisdata/tigris/server/transaction"
	ulog "github.com/tigrisdata/tigris/util/log"
)

// PrimaryIndexSubspace is used to store metadata about Tigris primary key indexes.
type PrimaryIndexSubspace struct {
	metadataSubspace
}

// PrimaryIndexMetadata contains index wide metadata.
type PrimaryIndexMetadata struct {
	ID   uint32 `json:"id"`
	Name string `json:"name"`
}

const indexMetaValueVersion int32 = 1

func newPrimaryIndexStore(nameRegistry *NameRegistry) *PrimaryIndexSubspace {
	return &PrimaryIndexSubspace{
		metadataSubspace{
			SubspaceName: nameRegistry.EncodingSubspaceName(),
			KeyVersion:   []byte{encKeyVersion},
		},
	}
}

func (c *PrimaryIndexSubspace) getKey(nsID uint32, dbID uint32, collID uint32, name string) keys.Key {
	if name == "" {
		return keys.NewKey(c.SubspaceName, c.KeyVersion, UInt32ToByte(nsID), UInt32ToByte(dbID), UInt32ToByte(collID), indexKey)
	}

	return keys.NewKey(c.SubspaceName, c.KeyVersion, UInt32ToByte(nsID), UInt32ToByte(dbID), UInt32ToByte(collID), indexKey, name, keyEnd)
}

func (c *PrimaryIndexSubspace) insert(ctx context.Context, tx transaction.Tx, nsID uint32, dbID uint32, collID uint32, name string, metadata *PrimaryIndexMetadata) error {
	return c.insertMetadata(ctx, tx,
		c.validateArgs(nsID, dbID, collID, name, &metadata),
		c.getKey(nsID, dbID, collID, name),
		indexMetaValueVersion,
		metadata,
	)
}

func (c *PrimaryIndexSubspace) decodeMetadata(_ string, payload *internal.TableData) (*PrimaryIndexMetadata, error) {
	if payload.Ver == 0 {
		return &PrimaryIndexMetadata{ID: ByteToUInt32(payload.RawData)}, nil
	}

	var metadata PrimaryIndexMetadata

	if err := jsoniter.Unmarshal(payload.RawData, &metadata); ulog.E(err) {
		return nil, errors.Internal("failed to unmarshal collection metadata")
	}

	return &metadata, nil
}

func (c *PrimaryIndexSubspace) Get(ctx context.Context, tx transaction.Tx, nsID uint32, dbID uint32, collID uint32, name string) (*PrimaryIndexMetadata, error) {
	payload, err := c.getPayload(ctx, tx,
		c.validateArgs(nsID, dbID, collID, name, nil),
		c.getKey(nsID, dbID, collID, name),
	)
	if err != nil {
		return nil, err
	}

	if payload == nil {
		return nil, errors.ErrNotFound
	}

	return c.decodeMetadata(name, payload)
}

func (c *PrimaryIndexSubspace) Update(ctx context.Context, tx transaction.Tx, nsID uint32, dbID uint32, collID uint32, name string, metadata *PrimaryIndexMetadata) error {
	return c.updateMetadata(ctx, tx,
		c.validateArgs(nsID, dbID, collID, name, &metadata),
		c.getKey(nsID, dbID, collID, name),
		indexMetaValueVersion,
		metadata,
	)
}

func (c *PrimaryIndexSubspace) delete(ctx context.Context, tx transaction.Tx, nsID uint32, dbID uint32, collID uint32, name string) error {
	return c.deleteMetadata(ctx, tx,
		c.validateArgs(nsID, dbID, collID, name, nil),
		c.getKey(nsID, dbID, collID, name),
	)
}

func (c *PrimaryIndexSubspace) softDelete(ctx context.Context, tx transaction.Tx, nsID uint32, dbID uint32, collID uint32, name string) error {
	newKey := keys.NewKey(c.SubspaceName, c.KeyVersion, UInt32ToByte(nsID), UInt32ToByte(dbID), UInt32ToByte(collID), indexKey, name, keyDroppedEnd)

	return c.softDeleteMetadata(ctx, tx,
		c.validateArgs(nsID, dbID, collID, name, nil),
		c.getKey(nsID, dbID, collID, name),
		newKey,
	)
}

func (_ *PrimaryIndexSubspace) validateArgs(nsID uint32, dbID uint32, collID uint32, name string, metadata **PrimaryIndexMetadata) error {
	if nsID == 0 || dbID == 0 || collID == 0 {
		return errors.InvalidArgument("invalid id")
	}

	if name == "" {
		return errors.InvalidArgument("empty index name")
	}

	if metadata != nil && *metadata == nil {
		return errors.InvalidArgument("invalid nil payload")
	}

	return nil
}

func (c *PrimaryIndexSubspace) list(ctx context.Context, tx transaction.Tx, namespaceId uint32, dbID uint32, collId uint32,
) (map[string]*PrimaryIndexMetadata, error) {
	indexes := make(map[string]*PrimaryIndexMetadata)
	droppedIndexes := make(map[string]uint32)

	if err := c.listMetadata(ctx, tx, c.getKey(namespaceId, dbID, collId, ""), 7,
		func(dropped bool, name string, data *internal.TableData) error {
			m, err := c.decodeMetadata(name, data)
			if err != nil {
				return err
			}

			if dropped {
				droppedIndexes[name] = m.ID
			} else {
				indexes[name] = m
			}

			return nil
		},
	); err != nil {
		return nil, err
	}

	log.Debug().Uint32("db", dbID).Uint32("coll", collId).Interface("list", droppedIndexes).Msg("dropped indexes")
	log.Debug().Uint32("db", dbID).Uint32("coll", collId).Interface("list", indexes).Msg("created indexes")

	// retrogression check
	for droppedC, droppedValue := range droppedIndexes {
		if createdValue, ok := indexes[droppedC]; ok && droppedValue >= createdValue.ID {
			return nil, errors.Internal(
				"retrogression found in indexes assigned value index [%s] droppedValue [%d] createdValue [%d]",
				droppedC, droppedValue, createdValue.ID)
		}
	}

	return indexes, nil
}
