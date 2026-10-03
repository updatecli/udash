package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/sm"
)

// labelFilterParams holds parameters for applying a label filter to a query.
type labelFilterParams struct {
	Query  *bob.BaseQuery[*dialect.SelectQuery]
	Labels map[string]string
	Ctx    context.Context
}

func applyLabelFilter(params labelFilterParams) error {

	if params.Ctx == nil {
		params.Ctx = context.Background()
	}

	if len(params.Labels) == 0 {
		return nil
	}

	errs := []error{}
	for key, value := range params.Labels {
		if key == "" {
			errs = append(errs, fmt.Errorf("%w: label key cannot be empty", ErrInvalidParameter))
			continue
		}

		// The lookup is not bounded in time: last_pipeline_report_at only holds the
		// latest report carrying the label, so a window ending before it would hide a
		// label whose older reports are in range. The query being filtered already
		// applies its own time range.
		results, totalCounts, err := GetLabelRecords(params.Ctx, "", key, value, "", "", 0, 1)
		if err != nil {
			// Not a problem with the request, so it is reported alone rather than
			// alongside the others, which would hand it to the caller.
			return fmt.Errorf("getting label records: %w", err)
		}

		if totalCounts == 0 {
			if value == "" {
				errs = append(errs, fmt.Errorf("%w: label not found for key %s", ErrInvalidParameter, key))
			} else {
				errs = append(errs, fmt.Errorf("%w: label not found for %s=%s", ErrInvalidParameter, key, value))
			}
			continue
		}

		ids := make([]string, 0, len(results))
		for i := range results {
			ids = append(ids, results[i].ID.String())
		}

		if len(ids) == 0 {
			if value == "" {
				errs = append(errs, fmt.Errorf("%w: no label ids found for key %s", ErrInvalidParameter, key))
			} else {
				errs = append(errs, fmt.Errorf("%w: no label ids found for %s=%s", ErrInvalidParameter, key, value))
			}
			continue
		}

		params.Query.Apply(
			sm.Where(
				psql.Raw(`label_ids && ?`, fmt.Sprintf("{%s}", strings.Join(ids, ","))),
			),
		)
	}

	if len(errs) > 0 {
		return fmt.Errorf("applying label filter: %w", errors.Join(errs...))
	}

	return nil
}
