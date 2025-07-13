package postgres

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"regexp"

	"gorm.io/gorm/clause"
)

// Rewrites conditions of WHERE clauses to replace `col IN (?)` and `col NOT IN (?)`
// with `col = ANY(?)` and `col != ALL(?)`, respectively. The difference between
// the two forms is in their interplay with prepared statements:
//
//  1. A condition .Where("col IN (?)", values) expands to `col IN ($1,$2,...)`
//     where the list has len(values) items. Every value[i] is sent to postgres
//     as a separate query argument.
//  2. A condition .Where("col = ANY(?)", values) always expands to `col = ANY($1)`,
//     and values are sent to postgres as exactly one query argument (of array type).
//
// The option 1 does not iteract well with prepared statements. It produces
// a different query for different len(values). Option 2, on the other hand,
// needs only one prepared statement for any len(values).
func rewriteWhereClauses(e clause.Expression) clause.Expression {
	var r inClausesRewriter
	return r.rewriteExpression(e)
}

type inClausesRewriter struct{}

func (r inClausesRewriter) rewriteExpression(e clause.Expression) clause.Expression {
	switch e := e.(type) {
	case clause.Expr:
		return r.rewriteExpr(e)
	case clause.NamedExpr:
		return r.rewriteNamedExpr(e)
	case clause.IN:
		return r.rewriteInExpr(e)

	case clause.Where:
		return clause.Where{Exprs: r.rewriteArray(e.Exprs)}
	case clause.OrConditions:
		return clause.OrConditions{Exprs: r.rewriteArray(e.Exprs)}
	case clause.AndConditions:
		return clause.AndConditions{Exprs: r.rewriteArray(e.Exprs)}
	case clause.NotConditions:
		return clause.NotConditions{Exprs: r.rewriteArray(e.Exprs)}

	default:
		return e
	}
}

func (r inClausesRewriter) rewriteArray(in []clause.Expression) (out []clause.Expression) {
	out = make([]clause.Expression, len(in))
	for i := range in {
		out[i] = r.rewriteExpression(in[i])
	}
	return out
}

var (
	// NOTE: this does not exactly follow the SQL syntax. Quoted identifier names
	// may contain any non-NULL characters, but we only allow \w = [a-z0-9_].
	// Also, these regexps allow column names like "123abc". This does not matter
	// because postgres will check the syntax, anyway.
	columnInRe    = regexp.MustCompile(`(?i)^\s*((\w+\.)|("\w+"\.))?((\w+)|("\w+"))\s+in\s*\(\?\)\s*$`)
	columnNotInRe = regexp.MustCompile(`(?i)^\s*((\w+\.)|("\w+"\.))?((\w+)|("\w+"))\s+not\s+in\s*\(\?\)\s*$`)
)

func (r inClausesRewriter) rewriteExpr(in clause.Expr) (out clause.Expr) {
	mIn := columnInRe.FindStringSubmatch(in.SQL)
	mNotIn := columnNotInRe.FindStringSubmatch(in.SQL)
	if mIn == nil && mNotIn == nil {
		return in
	}
	if len(in.Vars) != 1 {
		return in
	}

	vars := r.rewriteExprVar(in.Vars[0])
	if vars == nil {
		return in
	}

	if mIn != nil {
		return clause.Expr{
			SQL:                fmt.Sprintf("%s%s = ANY(?)", mIn[1], mIn[4]),
			Vars:               []any{passthroughValuer{vars}},
			WithoutParentheses: in.WithoutParentheses,
		}
	} else {
		return clause.Expr{
			SQL:                fmt.Sprintf("%s%s != ALL(?)", mNotIn[1], mNotIn[4]),
			Vars:               []any{passthroughValuer{vars}},
			WithoutParentheses: in.WithoutParentheses,
		}
	}
}

func (r inClausesRewriter) rewriteNamedExpr(in clause.NamedExpr) (out clause.NamedExpr) {
	e := r.rewriteExpr(clause.Expr{SQL: in.SQL, Vars: in.Vars})
	return clause.NamedExpr{SQL: e.SQL, Vars: e.Vars}
}

func (r inClausesRewriter) rewriteInExpr(in clause.IN) (out clause.Expression) {
	values := r.rewriteInValues(in.Values)
	if values != nil {
		return EqANY{Column: in.Column, Values: passthroughValuer{values}}
	} else {
		return in
	}
}

func (r inClausesRewriter) rewriteExprVar(in any) (out any) {
	v := reflect.ValueOf(in)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return nil
	}

	if r.isSupportedArrayElementType(v.Type().Elem()) {
		return in
	} else {
		return nil
	}
}

func (r inClausesRewriter) rewriteInValues(in []any) (out []any) {
	if len(in) == 0 {
		return in
	}

	et := reflect.TypeOf(in[0])
	for _, v := range in {
		if reflect.TypeOf(v) != et {
			return nil
		}
	}

	if r.isSupportedArrayElementType(et) {
		return in
	} else {
		return nil
	}
}

func (r inClausesRewriter) isSupportedArrayElementType(t reflect.Type) bool {
	k := t.Kind()
	if k == reflect.Bool ||
		k == reflect.Int || k == reflect.Int8 || k == reflect.Int16 || k == reflect.Int32 || k == reflect.Int64 ||
		k == reflect.Uint || k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64 ||
		k == reflect.Float32 || k == reflect.Float64 ||
		k == reflect.String {
		return true
	}
	if t.Implements(reflect.TypeFor[driver.Valuer]()) {
		return true
	}
	return false
}

// This driver.Valuer hides arrays from GORM. Statement's AddVar() expands arrays
// into multiple values in the query, and binds each of them as a separate query
// argument. We need the whole array to be one query argument.
type passthroughValuer struct {
	val any
}

func (v passthroughValuer) Value() (driver.Value, error) {
	return v.val, nil
}

type EqANY struct {
	Column any
	Values any
}

func (eqANY EqANY) Build(builder clause.Builder) {
	builder.WriteQuoted(eqANY.Column)

	builder.WriteString(" = ANY(")
	builder.AddVar(builder, eqANY.Values)
	builder.WriteByte(')')
}
