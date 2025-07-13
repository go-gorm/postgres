package postgres

import "testing"

func TestColumnInRe(t *testing.T) {
	type validExample struct {
		e   string
		tbl string
		col string
	}
	validExamples := []validExample{
		{e: `c in (?)`, tbl: ``, col: `c`},
		{e: `c IN (?)`, tbl: ``, col: `c`},
		{e: ` c   in   (?) `, tbl: ``, col: `c`},
		{e: `"c" in (?)`, tbl: ``, col: `"c"`},
		{e: `tbl.c in (?)`, tbl: `tbl.`, col: `c`},
		{e: `tbl."c" in (?)`, tbl: `tbl.`, col: `"c"`},
		{e: `"tbl".c in (?)`, tbl: `"tbl".`, col: `c`},
		{e: `"tbl"."c" in (?)`, tbl: `"tbl".`, col: `"c"`},
		{e: `column_name in (?)`, tbl: ``, col: `column_name`},
		{e: `abc123 in (?)`, tbl: ``, col: `abc123`},
	}

	for _, e := range validExamples {
		t.Run(e.e, func(t *testing.T) {
			m := columnInRe.FindStringSubmatch(e.e)
			if m == nil {
				t.Fatalf("must be a valid IN expression: %q", e.e)
			}
			if len(m) != 1+6 {
				t.Fatalf("columnInRe is expected to have 6 capture groups")
			}
			if m[1] != e.tbl || m[4] != e.col {
				t.Fatalf("columnInRe fails to capture the table and column names")
			}
		})
	}

	invalidExamples := []string{
		`tbl c in (?)`,
		`c not in (?)`,
		`tbl.c = ANY(?)`,
		`column-name in (?)`,
		// NOTE: this one is a valid escaped column name (it may contain
		// any characters except NULL), but let us not handle this case.
		`"tbl.c" in (?)`,
	}

	for _, e := range invalidExamples {
		t.Run(e, func(t *testing.T) {
			if columnInRe.MatchString(e) {
				t.Fatalf("must be a invalid IN expression: %q", e)
			}
		})
	}
}

func TestColumnNotInRe(t *testing.T) {
	type validExample struct {
		e   string
		tbl string
		col string
	}
	validExamples := []validExample{
		{e: `c not in (?)`, tbl: ``, col: `c`},
		{e: `c not IN (?)`, tbl: ``, col: `c`},
		{e: ` c   NOT  in   (?) `, tbl: ``, col: `c`},
		{e: `"c" not in (?)`, tbl: ``, col: `"c"`},
		{e: `tbl.c not in (?)`, tbl: `tbl.`, col: `c`},
		{e: `tbl."c" not in (?)`, tbl: `tbl.`, col: `"c"`},
		{e: `"tbl".c not in (?)`, tbl: `"tbl".`, col: `c`},
		{e: `"tbl"."c" not in (?)`, tbl: `"tbl".`, col: `"c"`},
		{e: `column_name not in (?)`, tbl: ``, col: `column_name`},
		{e: `abc123 not in (?)`, tbl: ``, col: `abc123`},
	}

	for _, e := range validExamples {
		t.Run(e.e, func(t *testing.T) {
			m := columnNotInRe.FindStringSubmatch(e.e)
			if m == nil {
				t.Fatalf("must be a valid NOT IN expression: %q", e.e)
			}
			if len(m) != 1+6 {
				t.Fatalf("columnNotInRe is expected to have 6 capture groups")
			}
			if m[1] != e.tbl || m[4] != e.col {
				t.Fatalf("columnNotInRe fails to capture the table and column names")
			}
		})
	}

	invalidExamples := []string{
		`tbl c not in (?)`,
		`c in (?)`,
		`c not (?)`,
		`c in not (?)`,
		`tbl.c != ALL(?)`,
		`column-name not in (?)`,
		// NOTE: this one is a valid escaped column name (it may contain
		// any characters except NULL), but let us not handle this case.
		`"tbl.c" not in (?)`,
	}

	for _, e := range invalidExamples {
		t.Run(e, func(t *testing.T) {
			if columnNotInRe.MatchString(e) {
				t.Fatalf("must be a invalid NOT IN expression: %q", e)
			}
		})
	}
}
