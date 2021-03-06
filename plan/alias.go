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
)

type Alias struct {
	readonly
	alias string
}

func NewAlias(alias string) *Alias {
	return &Alias{
		alias: alias,
	}
}

func (this *Alias) Accept(visitor Visitor) (interface{}, error) {
	return visitor.VisitAlias(this)
}

func (this *Alias) New() Operator {
	return &Alias{}
}

func (this *Alias) Alias() string {
	return this.alias
}

func (this *Alias) MarshalJSON() ([]byte, error) {
	return json.Marshal(this.MarshalBase(nil))
}

func (this *Alias) MarshalBase(f func(map[string]interface{})) map[string]interface{} {
	r := map[string]interface{}{"#operator": "Alias"}
	r["as"] = this.alias
	if f != nil {
		f(r)
	}
	return r
}

func (this *Alias) UnmarshalJSON(body []byte) error {
	var _unmarshalled struct {
		_  string `json:"#operator"`
		As string `json:"as"`
	}
	err := json.Unmarshal(body, &_unmarshalled)
	this.alias = _unmarshalled.As
	return err
}
