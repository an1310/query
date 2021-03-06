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
	"github.com/couchbase/query/datastore"
	"github.com/couchbase/query/expression"
	"github.com/couchbase/query/expression/parser"
	"github.com/couchbase/query/value"
)

type IndexJoin struct {
	readonly
	keyspace     datastore.Keyspace
	term         *algebra.KeyspaceTerm
	outer        bool
	keyFor       string
	idExpr       expression.Expression
	index        datastore.Index
	covers       expression.Covers
	filterCovers map[*expression.Cover]value.Value
}

func NewIndexJoin(keyspace datastore.Keyspace, join *algebra.IndexJoin,
	index datastore.Index, covers expression.Covers,
	filterCovers map[*expression.Cover]value.Value) *IndexJoin {
	rv := &IndexJoin{
		keyspace:     keyspace,
		term:         join.Right(),
		outer:        join.Outer(),
		keyFor:       join.For(),
		index:        index,
		covers:       covers,
		filterCovers: filterCovers,
	}

	rv.idExpr = expression.NewField(
		expression.NewMeta(expression.NewIdentifier(rv.keyFor)),
		expression.NewFieldName("id", false))
	return rv
}

func (this *IndexJoin) Accept(visitor Visitor) (interface{}, error) {
	return visitor.VisitIndexJoin(this)
}

func (this *IndexJoin) New() Operator {
	return &IndexJoin{}
}

func (this *IndexJoin) Keyspace() datastore.Keyspace {
	return this.keyspace
}

func (this *IndexJoin) Term() *algebra.KeyspaceTerm {
	return this.term
}

func (this *IndexJoin) Outer() bool {
	return this.outer
}

func (this *IndexJoin) For() string {
	return this.keyFor
}

func (this *IndexJoin) IdExpr() expression.Expression {
	return this.idExpr
}

func (this *IndexJoin) Index() datastore.Index {
	return this.index
}

func (this *IndexJoin) Covers() expression.Covers {
	return this.covers
}

func (this *IndexJoin) FilterCovers() map[*expression.Cover]value.Value {
	return this.filterCovers
}

func (this *IndexJoin) Covering() bool {
	return len(this.covers) > 0
}

func (this *IndexJoin) MarshalJSON() ([]byte, error) {
	return json.Marshal(this.MarshalBase(nil))
}

func (this *IndexJoin) MarshalBase(f func(map[string]interface{})) map[string]interface{} {
	r := map[string]interface{}{"#operator": "IndexJoin"}
	r["namespace"] = this.term.Namespace()
	r["keyspace"] = this.term.Keyspace()
	r["on_key"] = expression.NewStringer().Visit(this.term.Keys())
	r["for"] = this.keyFor

	if this.outer {
		r["outer"] = this.outer
	}

	if this.term.As() != "" {
		r["as"] = this.term.As()
	}

	scan := map[string]interface{}{
		"index":    this.index.Name(),
		"index_id": this.index.Id(),
		"using":    this.index.Type(),
	}

	if this.covers != nil {
		scan["covers"] = this.covers
	}

	if len(this.filterCovers) > 0 {
		fc := make(map[string]value.Value, len(this.filterCovers))
		for c, v := range this.filterCovers {
			fc[c.String()] = v
		}

		scan["filter_covers"] = fc
	}

	r["scan"] = scan
	if f != nil {
		f(r)
	}
	return r
}

func (this *IndexJoin) UnmarshalJSON(body []byte) error {
	var _unmarshalled struct {
		_     string `json:"#operator"`
		Names string `json:"namespace"`
		Keys  string `json:"keyspace"`
		On    string `json:"on_key"`
		Outer bool   `json:"outer"`
		As    string `json:"as"`
		For   string `json:"for"`
		Scan  struct {
			Index        string                     `json:"index"`
			IndexId      string                     `json:"index_id"`
			Using        datastore.IndexType        `json:"using"`
			Covers       []string                   `json:"covers"`
			FilterCovers map[string]json.RawMessage `json:"filter_covers"`
		} `json:"scan"`
	}

	err := json.Unmarshal(body, &_unmarshalled)
	if err != nil {
		return err
	}

	var keys_expr expression.Expression
	if _unmarshalled.On != "" {
		keys_expr, err = parser.Parse(_unmarshalled.On)
		if err != nil {
			return err
		}
	}

	this.outer = _unmarshalled.Outer
	this.keyFor = _unmarshalled.For
	this.idExpr = expression.NewField(
		expression.NewMeta(expression.NewIdentifier(this.keyFor)),
		expression.NewFieldName("id", false))

	this.term = algebra.NewKeyspaceTerm(_unmarshalled.Names, _unmarshalled.Keys, _unmarshalled.As, keys_expr, nil)
	this.keyspace, err = datastore.GetKeyspace(_unmarshalled.Names, _unmarshalled.Keys)
	if err != nil {
		return err
	}

	indexer, err := this.keyspace.Indexer(_unmarshalled.Scan.Using)
	if err != nil {
		return err
	}

	this.index, err = indexer.IndexById(_unmarshalled.Scan.IndexId)
	if err != nil {
		return err
	}

	if _unmarshalled.Scan.Covers != nil {
		this.covers = make(expression.Covers, len(_unmarshalled.Scan.Covers))
		for i, c := range _unmarshalled.Scan.Covers {
			expr, err := parser.Parse(c)
			if err != nil {
				return err
			}

			this.covers[i] = expression.NewCover(expr)
		}
	}

	if len(_unmarshalled.Scan.FilterCovers) > 0 {
		this.filterCovers = make(map[*expression.Cover]value.Value, len(_unmarshalled.Scan.FilterCovers))
		for k, v := range _unmarshalled.Scan.FilterCovers {
			expr, err := parser.Parse(k)
			if err != nil {
				return err
			}

			c := expression.NewCover(expr)
			this.filterCovers[c] = value.NewValue(v)
		}
	}

	return nil
}
