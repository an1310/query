//  Copyright (c) 2014 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package execution

import (
	"encoding/json"

	"github.com/couchbase/query/plan"
	"github.com/couchbase/query/value"
)

type CreateIndex struct {
	base
	plan *plan.CreateIndex
}

func NewCreateIndex(plan *plan.CreateIndex) *CreateIndex {
	rv := &CreateIndex{
		base: newBase(),
		plan: plan,
	}

	rv.output = rv
	return rv
}

func (this *CreateIndex) Accept(visitor Visitor) (interface{}, error) {
	return visitor.VisitCreateIndex(this)
}

func (this *CreateIndex) Copy() Operator {
	return &CreateIndex{this.base.copy(), this.plan}
}

func (this *CreateIndex) RunOnce(context *Context, parent value.Value) {
	this.once.Do(func() {
		defer context.Recover()       // Recover from any panic
		defer close(this.itemChannel) // Broadcast that I have stopped
		defer this.notify()           // Notify that I have stopped

		if context.Readonly() {
			return
		}

		// Actually create index
		node := this.plan.Node()
		indexer, err := this.plan.Keyspace().Indexer(node.Using())
		if err != nil {
			context.Error(err)
			return
		}

		_, err = indexer.CreateIndex(context.RequestId(), node.Name(), node.SeekKeys(),
			node.RangeKeys(), node.Where(), node.With())
		if err != nil {
			context.Error(err)
		}
	})
}

func (this *CreateIndex) MarshalJSON() ([]byte, error) {
	r := this.plan.MarshalBase(func(r map[string]interface{}) {
		this.marshalTimes(r)
	})
	return json.Marshal(r)
}
