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

package billing

import (
	"context"

	"github.com/google/uuid"
	"github.com/tigrisdata/tigris/errors"
)

type noop struct{}

func (n *noop) CreateAccount(_ context.Context, _ string, _ string) (MetronomeId, error) {
	return uuid.Nil, errors.Unimplemented("billing not enabled on this server")
}

func (n *noop) AddDefaultPlan(ctx context.Context, accountId MetronomeId) (bool, error) {
	return n.AddPlan(ctx, accountId, uuid.New())
}

func (*noop) AddPlan(_ context.Context, _ MetronomeId, _ uuid.UUID) (bool, error) {
	return false, errors.Unimplemented("billing not enabled on this server")
}
