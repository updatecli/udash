package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/sm"
)

// labelFilterParams holds parameters for applying a date range filter to a query.
type labelFilterParams struct {
	Query     *bob.BaseQuery[*dialect.SelectQuery]
	Labels    map[string]string
	StartTime string
	EndTime   string
	Ctx       context.Context
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
			errs = append(errs, fmt.Errorf("label key cannot be empty"))
			continue
		}

		results, totalCounts, err := GetLabelRecords(
			params.Ctx,
			"",
			key,
			value,
			params.StartTime,
			params.EndTime,
			0,
			1,
		)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed getting label records: %s", err))
			continue
		}

		if totalCounts == 0 {
			if value == "" {
				errs = append(errs, fmt.Errorf("label not found for key %s", key))
			} else {
				errs = append(errs, fmt.Errorf("label not found for %s=%s", key, value))
			}
			continue
		}

		ids := make([]string, 0, len(results))
		for i := range results {
			ids = append(ids, results[i].ID.String())
		}

		if len(ids) == 0 {
			if value == "" {
				errs = append(errs, fmt.Errorf("no label ids found for key %s", key))
			} else {
				errs = append(errs, fmt.Errorf("no label ids found for %s=%s", key, value))
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
		return fmt.Errorf("errors occurred while applying label filter: %v", errs)
	}

	return nil
}
