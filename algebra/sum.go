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
	"fmt"

	"github.com/couchbaselabs/query/value"
)

type Sum struct {
	aggregateBase
}

func NewSum(parameter Expression) Aggregate {
	return &Sum{aggregateBase{parameter}}
}

var _DEFAULT_SUM = value.NewValue(0.0)

func (this *Sum) Default() value.Value {
	return _DEFAULT_SUM
}

func (this *Sum) Initial() InitialAggregate {
	return this
}

func (this *Sum) Intermediate() IntermediateAggregate {
	return this
}

func (this *Sum) Final() FinalAggregate {
	return this
}

func (this *Sum) CumulateInitial(item, cumulative value.Value, context Context) (value.Value, error) {
	return this.cumulate(item, cumulative, context)
}

func (this *Sum) CumulateIntermediate(item, cumulative value.Value, context Context) (value.Value, error) {
	return this.cumulate(item, cumulative, context)
}

func (this *Sum) CumulateFinal(item, cumulative value.Value, context Context) (value.Value, error) {
	return this.cumulate(item, cumulative, context)
}

func (this *Sum) cumulate(item, cumulative value.Value, context Context) (value.Value, error) {
	item, e := this.parameter.Evaluate(item, context)
	if e != nil {
		return nil, e
	}

	actual := item.Actual()
	switch actual := actual.(type) {
	case float64:
		sum := cumulative.Actual()
		switch sum := sum.(type) {
		case float64:
			return value.NewValue(sum + actual), nil
		default:
			return nil, fmt.Errorf("Invalid SUM %v of type %T.", sum, sum)
		}
	default:
		return cumulative, nil
	}
}