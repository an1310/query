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
	"github.com/couchbase/query/value"
)

/*
Comparison terms allow for comparing two expressions. For
EQUALS (= and ==) and NOT EQUALS (!= and <>) two forms
are supported to aid in compatibility with other query
languages.
*/
type Eq struct {
	CommutativeBinaryFunctionBase
}

func NewEq(first, second Expression) Function {
	rv := &Eq{
		*NewCommutativeBinaryFunctionBase("eq", first, second),
	}

	rv.expr = rv
	return rv
}

/*
Visitor pattern.
*/
func (this *Eq) Accept(visitor Visitor) (interface{}, error) {
	return visitor.VisitEq(this)
}

/*
It returns a value type BOOLEAN.
*/
func (this *Eq) Type() value.Type { return value.BOOLEAN }

func (this *Eq) Evaluate(item value.Value, context Context) (value.Value, error) {
	return this.BinaryEval(this, item, context)
}

/*
If this expression is in the WHERE clause of a partial index, lists
the Expressions that are implicitly covered.

For Eq, list either a static value, or this expression.
*/
func (this *Eq) FilterCovers(covers map[string]value.Value) map[string]value.Value {
	var static, other Expression
	if this.Second().Value() != nil {
		static = this.Second()
		other = this.First()
	} else if this.First().Value() != nil {
		static = this.First()
		other = this.Second()
	}

	if static != nil {
		covers[other.String()] = static.Value()
		return covers
	}

	covers[this.String()] = value.TRUE_VALUE
	return covers
}

func (this *Eq) Apply(context Context, first, second value.Value) (value.Value, error) {
	return first.Equals(second), nil
}

/*
Factory method pattern.
*/
func (this *Eq) Constructor() FunctionConstructor {
	return func(operands ...Expression) Function {
		return NewEq(operands[0], operands[1])
	}
}

/*
This function implements the NOT EQUALS comparison operation.
*/
func NewNE(first, second Expression) Expression {
	return NewNot(NewEq(first, second))
}
