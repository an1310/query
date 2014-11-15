//  Copyright (c) 2014 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package algebra

import (
	"encoding/json"

	"github.com/couchbaselabs/query/expression"
	"github.com/couchbaselabs/query/value"
)

type DropIndex struct {
	keyspace *KeyspaceRef `json:"keyspace"`
	name     string       `json:"name"`
}

func NewDropIndex(keyspace *KeyspaceRef, name string) *DropIndex {
	return &DropIndex{
		keyspace: keyspace,
		name:     name,
	}
}

func (this *DropIndex) Accept(visitor Visitor) (interface{}, error) {
	return visitor.VisitDropIndex(this)
}

func (this *DropIndex) Signature() value.Value {
	return nil
}

func (this *DropIndex) Formalize() error {
	return nil
}

func (this *DropIndex) MapExpressions(mapper expression.Mapper) error {
	return nil
}

func (this *DropIndex) Keyspace() *KeyspaceRef {
	return this.keyspace
}

func (this *DropIndex) Name() string {
	return this.name
}

func (this *DropIndex) MarshalJSON() ([]byte, error) {
	r := map[string]interface{}{"type": "dropIndex"}
	r["keyspaceRef"] = this.keyspace
	r["name"] = this.name
	return json.Marshal(r)
}
