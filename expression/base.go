//  Copyright (c) 2014 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package expression

import (
	"reflect"

	"github.com/couchbaselabs/query/value"
)

type expressionBase struct {
}

func (this *expressionBase) Evaluate(item value.Value, context Context) (value.Value, error) {
	panic("Must override.")
}

func (this *expressionBase) EquivalentTo(other Expression) bool {
	if reflect.TypeOf(this) != reflect.TypeOf(other) {
		return false
	}

	ours := this.Dependencies()
	theirs := other.Dependencies()
	if len(ours) != len(theirs) {
		return false
	}

	for i, o := range ours {
		if !o.EquivalentTo(theirs[i]) {
			return false
		}
	}

	return true
}

func (this *expressionBase) Dependencies() Expressions {
	return nil
}

func (this *expressionBase) Alias() string {
	return ""
}

func (this *expressionBase) Fold() Expression {
	panic("Must override.")
}

func (this *expressionBase) Formalize() {
	for _, d := range this.Dependencies() {
		d.Formalize()
	}
}

func (this *expressionBase) SubsetOf(other Expression) bool {
	return this.EquivalentTo(other)
}

func (this *expressionBase) Spans(index Index) Spans {
	return nil
}

var _MISSING_VALUE = value.NewMissingValue()
var _NULL_VALUE = value.NewValue(nil)
