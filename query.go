package postgres

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
)

func Query(db *gorm.DB) {
	if db.Error == nil {
		if db.Statement.Schema != nil && !db.Statement.Unscoped {
			for _, c := range db.Statement.Schema.QueryClauses {
				db.Statement.AddClause(c)
			}
		}

		if db.Statement.SQL.String() == "" {
			// PostgreSQL doesn't like Count with ORDER BY when there are no GROUP BY
			if orderByClause, ok := db.Statement.Clauses["ORDER BY"]; ok {
				if _, ok := db.Statement.Clauses["GROUP BY"]; !ok {
					if selectClause, ok := db.Statement.Clauses["SELECT"]; ok {
						if expr, ok := selectClause.Expression.(clause.Expr); ok && len(expr.SQL) > 7 && strings.EqualFold(expr.SQL[0:5], "count") { // count(1)
							delete(db.Statement.Clauses, "ORDER BY")
							defer func() {
								db.Statement.Clauses["ORDER BY"] = orderByClause
							}()
						}
					}
				}
			}

			callbacks.BuildQuerySQL(db)
		}

		if !db.DryRun && db.Error == nil {
			rows, err := db.Statement.ConnPool.QueryContext(db.Statement.Context, db.Statement.SQL.String(), db.Statement.Vars...)
			if err != nil {
				db.AddError(err)
				return
			}
			defer rows.Close()

			gorm.Scan(rows, db, false)
		}
	}
}
