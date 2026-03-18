package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
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

	labelIDs := []string{}
	for key, value := range params.Labels {
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
			logrus.Errorf("failed getting label records: %s", err)
			continue
		}
		switch totalCounts {
		case 0:
			// Normally we should never end up here as the labels are inserted when the report is created,
			// but in case of a manual deletion of a label, we log an error and skip the label filter for this key-value pair.
			logrus.Errorf("label not found for %s=%s", key, value)
		case 1:
			labelIDs = append(labelIDs, results[0].ID.String())
		default:
			logrus.Warningf("multiple labels found for %s=%s", key, value)
		}
	}

	if labelIDs == nil {
		// Unless a label was manually deleted, this should never happen as the labels are inserted when the report is created
		return fmt.Errorf("no label found for the provided labels filter")
	}

	params.Query.Apply(
		sm.Where(
			psql.Raw(`label_ids @> ?`, fmt.Sprintf("{%s}", strings.Join(labelIDs, ","))),
		),
	)

	return nil
}
