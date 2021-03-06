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
	"math"

	"github.com/couchbase/query/value"
)

///////////////////////////////////////////////////
//
// Greatest
//
///////////////////////////////////////////////////

/*
This represents the comparison function GREATEST(expr1, expr2, ...).
It returns the largest non-NULL, non-MISSING input value.
*/
type Greatest struct {
	FunctionBase
}

func NewGreatest(operands ...Expression) Function {
	rv := &Greatest{
		*NewFunctionBase("greatest", operands...),
	}

	rv.expr = rv
	return rv
}

/*
Visitor pattern.
*/
func (this *Greatest) Accept(visitor Visitor) (interface{}, error) {
	return visitor.VisitFunction(this)
}

func (this *Greatest) Type() value.Type { return value.JSON }

func (this *Greatest) Evaluate(item value.Value, context Context) (value.Value, error) {
	return this.Eval(this, item, context)
}

func (this *Greatest) Apply(context Context, args ...value.Value) (value.Value, error) {
	rv := value.NULL_VALUE
	for _, a := range args {
		if a.Type() <= value.NULL {
			continue
		} else if rv == value.NULL_VALUE {
			rv = a
		} else if a.Collate(rv) > 0 {
			rv = a
		}
	}

	return rv, nil
}

/*
Minimum input arguments required for the defined function
GREATEST is 2.
*/
func (this *Greatest) MinArgs() int { return 2 }

/*
Maximum number of input arguments defined for the GREATEST
function is MaxInt16  = 1<<15 - 1.
*/
func (this *Greatest) MaxArgs() int { return math.MaxInt16 }

/*
Factory method pattern.
*/
func (this *Greatest) Constructor() FunctionConstructor {
	return NewGreatest
}

///////////////////////////////////////////////////
//
// Least
//
///////////////////////////////////////////////////

/*
This represents the comparison function LEAST(expr1, expr2, ...). It
returns the smallest non-NULL, non-MISSING input value.
*/
type Least struct {
	FunctionBase
}

func NewLeast(operands ...Expression) Function {
	rv := &Least{
		*NewFunctionBase("least", operands...),
	}

	rv.expr = rv
	return rv
}

/*
Visitor pattern.
*/
func (this *Least) Accept(visitor Visitor) (interface{}, error) {
	return visitor.VisitFunction(this)
}

func (this *Least) Type() value.Type { return value.JSON }

func (this *Least) Evaluate(item value.Value, context Context) (value.Value, error) {
	return this.Eval(this, item, context)
}

func (this *Least) Apply(context Context, args ...value.Value) (value.Value, error) {
	rv := value.NULL_VALUE

	for _, a := range args {
		if a.Type() <= value.NULL {
			continue
		} else if rv == value.NULL_VALUE {
			rv = a
		} else if a.Collate(rv) < 0 {
			rv = a
		}
	}

	return rv, nil
}

/*
Minimum input arguments required for the defined function
LEAST is 2.
*/
func (this *Least) MinArgs() int { return 2 }

/*
Maximum number of input arguments defined for the LEAST
function is MaxInt16  = 1<<15 - 1.
*/
func (this *Least) MaxArgs() int { return math.MaxInt16 }

/*
Factory method pattern.
*/
func (this *Least) Constructor() FunctionConstructor {
	return NewLeast
}

///////////////////////////////////////////////////
//
// Successor
//
///////////////////////////////////////////////////

/*
This Expression is primarily for internal use. It returns a successor
to the input argument, in N1QL collation order.
*/
type Successor struct {
	UnaryFunctionBase
}

func NewSuccessor(operand Expression) Function {
	rv := &Successor{
		*NewUnaryFunctionBase("successor", operand),
	}

	rv.expr = rv
	return rv
}

/*
Visitor pattern.
*/
func (this *Successor) Accept(visitor Visitor) (interface{}, error) {
	return visitor.VisitFunction(this)
}

func (this *Successor) Type() value.Type {
	return this.Operand().Type().Successor()
}

func (this *Successor) Evaluate(item value.Value, context Context) (value.Value, error) {
	return this.UnaryEval(this, item, context)
}

func (this *Successor) Apply(context Context, arg value.Value) (value.Value, error) {
	return arg.Successor(), nil
}

/*
Factory method pattern.
*/
func (this *Successor) Constructor() FunctionConstructor {
	return func(operands ...Expression) Function {
		return NewSuccessor(operands[0])
	}
}
