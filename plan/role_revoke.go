//  Copyright (c) 2014 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package plan

import (
	"encoding/json"

	"github.com/couchbase/query/algebra"
)

// Revoke role
type RevokeRole struct {
	readwrite
	node *algebra.RevokeRole
}

func NewRevokeRole(node *algebra.RevokeRole) *RevokeRole {
	return &RevokeRole{
		node: node,
	}
}

func (this *RevokeRole) Accept(visitor Visitor) (interface{}, error) {
	return visitor.VisitRevokeRole(this)
}

func (this *RevokeRole) New() Operator {
	return &RevokeRole{}
}

func (this *RevokeRole) Node() *algebra.RevokeRole {
	return this.node
}

func (this *RevokeRole) MarshalJSON() ([]byte, error) {
	return json.Marshal(this.MarshalBase(nil))
}

func (this *RevokeRole) MarshalBase(f func(map[string]interface{})) map[string]interface{} {
	r := map[string]interface{}{"#operator": "RevokeRole"}
	r["roles"] = this.node.Roles()
	r["users"] = this.node.Users()
	if f != nil {
		f(r)
	}
	return r
}

func (this *RevokeRole) UnmarshalJSON(body []byte) error {
	var _unmarshalled struct {
		_     string               `json:"#operator"`
		Roles algebra.RoleSpecList `json:"keyspace"`
		Users []string             `json:"namespace"`
	}

	this.node = algebra.NewRevokeRole(_unmarshalled.Roles, _unmarshalled.Users)
	return nil
}
