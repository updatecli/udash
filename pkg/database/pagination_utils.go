package database

import (
	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/sm"
)

// applyPagination restricts the given query to a single page of results.
//
// Pagination is opt in: a limit lower than one returns every matching row, which is what
// the callers asking for a complete dataset rely on.
//
// Pages are one based. A page lower than one is treated as the first one rather than
// building a negative offset: Postgres rejects those with "OFFSET must not be negative",
// and because that error only surfaces when the rows are read it used to turn into an
// empty dataset reported as a success. A request carrying a limit but no page, which is
// what every JSON search endpoint receives by default, went down exactly that path.
func applyPagination(query *bob.BaseQuery[*dialect.SelectQuery], limit, page int) {
	if limit < 1 {
		return
	}

	if page < 1 {
		page = 1
	}

	query.Apply(
		sm.Limit(limit),
		sm.Offset((page-1)*limit),
	)
}
