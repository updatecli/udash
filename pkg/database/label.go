package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/model"

	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/sm"
)

// InsertLabel creates a new label and inserts it into the database.
//
// It returns the ID of the newly created label.
func InsertLabel(ctx context.Context, key, value string) (string, error) {
	query := psql.Insert(
		im.Into("labels", "key", "value"),
		im.Values(psql.Arg(key), psql.Arg(value)),
		im.Returning("id"),
	)

	queryString, args, err := query.Build(ctx)

	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return "", err
	}

	var id uuid.UUID
	err = DB.QueryRow(ctx, queryString, args...).Scan(
		&id,
	)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return "", err
	}

	return id.String(), nil
}

// GetLabelKeyOnlyRecords returns a list of labels from the labels database table.
func GetLabelKeyOnlyRecords(ctx context.Context, startTime, endTime string, limit, page int) ([]string, int, error) {

	query := psql.Select(
		sm.Columns("id", "key", "created_at", "updated_at", "last_pipeline_report_at"),
		sm.From("labels"),
		sm.OrderBy("key"),
		sm.Distinct("key"),
	)

	if err := applyRangeFilter(
		"last_pipeline_report_at",
		DateRangeFilterParams{
			Query:         &query,
			DateRangeDays: 0,
			StartTime:     startTime,
			EndTime:       endTime,
		}); err != nil {
		return nil, 0, fmt.Errorf("applying last_pipeline_report_at range filter: %w", err)
	}

	totalCount := 0
	totalQuery := psql.Select(sm.From(query), sm.Columns("count(*)"))
	totalQueryString, totalArgs, err := totalQuery.Build(ctx)
	if err != nil {
		logrus.Errorf("building total count query failed: %s\n\t%s", totalQueryString, err)
		return nil, 0, err
	}

	if err = DB.QueryRow(ctx, totalQueryString, totalArgs...).Scan(
		&totalCount,
	); err != nil {
		logrus.Errorf("parsing total count result: %s", err)
	}

	if limit < totalCount && limit > 0 {
		query.Apply(
			sm.Limit(limit),
			sm.Offset((page-1)*limit),
		)
	}

	queryString, args, err := query.Build(ctx)

	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, 0, err
	}

	rows, err := DB.Query(ctx, queryString, args...)
	if err != nil {
		logrus.Errorf("query failed: %s\n\t%s", queryString, err)
		return nil, 0, err
	}
	defer rows.Close()

	results := []string{}

	for rows.Next() {
		r := model.Label{}

		err = rows.Scan(&r.ID, &r.Key, &r.CreatedAt, &r.UpdatedAt, &r.LastPipelineReportAt)
		if err != nil {
			logrus.Errorf("scanning label row failed: %s", err)
			continue
		}

		results = append(results, r.Key)
	}

	if err := rows.Err(); err != nil {
		logrus.Errorf("iterating label rows failed: %s", err)
		return nil, 0, err
	}

	return results, totalCount, nil
}

// GetLabelRecords returns a list of labels from the labels database table.
func GetLabelRecords(ctx context.Context, id, key, value, startTime, endTime string, limit, page int) ([]model.Label, int, error) {

	query := psql.Select(
		sm.Columns("id", "key", "value", "created_at", "updated_at", "last_pipeline_report_at"),
		sm.From("labels"),
		sm.OrderBy("key"),
	)

	if key != "" {
		query.Apply(
			sm.Where(psql.Quote("key").EQ(psql.Arg(key))),
		)
	}

	if value != "" {
		query.Apply(
			sm.Where(psql.Quote("value").EQ(psql.Arg(value))),
		)
	}

	if id != "" {
		query.Apply(
			sm.Where(psql.Quote("id").EQ(psql.Arg(id))),
		)
	}

	if err := applyRangeFilter(
		"last_pipeline_report_at",
		DateRangeFilterParams{
			Query:         &query,
			DateRangeDays: 0,
			StartTime:     startTime,
			EndTime:       endTime,
		}); err != nil {
		return nil, 0, fmt.Errorf("applying last_pipeline_report_at range filter: %w", err)
	}

	totalCount := 0
	totalQuery := psql.Select(sm.From(query), sm.Columns("count(*)"))
	totalQueryString, totalArgs, err := totalQuery.Build(ctx)
	if err != nil {
		logrus.Errorf("building total count query failed: %s\n\t%s", totalQueryString, err)
		return nil, 0, err
	}

	if err = DB.QueryRow(ctx, totalQueryString, totalArgs...).Scan(
		&totalCount,
	); err != nil {
		logrus.Errorf("parsing total count result: %s", err)
	}

	if limit < totalCount && limit > 0 {
		query.Apply(
			sm.Limit(limit),
			sm.Offset((page-1)*limit),
		)
	}

	queryString, args, err := query.Build(ctx)

	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, 0, err
	}

	rows, err := DB.Query(ctx, queryString, args...)
	if err != nil {
		logrus.Errorf("query failed: %s\n\t%s", queryString, err)
		return nil, 0, err
	}
	defer rows.Close()

	results := []model.Label{}

	for rows.Next() {
		r := model.Label{}

		err = rows.Scan(&r.ID, &r.Key, &r.Value, &r.CreatedAt, &r.UpdatedAt, &r.LastPipelineReportAt)
		if err != nil {
			logrus.Errorf("scanning label row failed: %s", err)
			continue
		}

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		logrus.Errorf("iterating label rows failed: %s", err)
		return nil, 0, err
	}

	return results, totalCount, nil
}

// InitLabels takes a map of labels and ensures that they exist in the database, creating them if necessary.
func InitLabels(ctx context.Context, labels map[string]string) ([]string, error) {
	errs := []error{}
	labelIDs := []string{}

	for labelKey, labelValue := range labels {
		if labelKey == "" {
			errs = append(errs, fmt.Errorf("missing key, ignoring label:\t%q:%q", labelKey, labelValue))
			continue
		}

		if labelValue == "" {
			errs = append(errs, fmt.Errorf("missing value, ignoring label:\t%q:%q", labelKey, labelValue))
			continue
		}

		labelRecords, totalCount, err := GetLabelRecords(ctx, "", labelKey, labelValue, "", "", 0, 1)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get labels: %s", err))
			continue
		}

		switch totalCount {
		case 0:
			id, err := InsertLabel(ctx, labelKey, labelValue)
			if err != nil {
				err := fmt.Errorf("insert label data: %s", err)
				errs = append(errs, err)
				continue
			}

			parsedID, err := uuid.Parse(id)
			if err != nil {
				errs = append(errs, fmt.Errorf("parsing id: %s", err))
				continue
			}

			labelIDs = append(labelIDs, parsedID.String())
		case 1:
			if labelValue == labelRecords[0].Value {
				labelIDs = append(labelIDs, labelRecords[0].ID.String())
			}
		default:
			errMsg := fmt.Errorf("something went wrong multiple labels found for key %s", labelKey)
			logrus.Error(errMsg)
			errs = append(errs, errMsg)
		}
	}

	if len(errs) > 0 {
		for i := range errs {
			logrus.Errorln(errs[i])
		}
		return nil, fmt.Errorf("something went wrong during label creation")
	}

	return labelIDs, nil
}
