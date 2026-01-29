package database

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sirupsen/logrus"
	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/dm"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	"github.com/updatecli/udash/pkg/model"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

// SearchLatestReportData represents a report.
type SearchLatestReportData struct {
	// ID represents the unique identifier of the report.
	ID string
	// Name represents the name of the report.
	Name string
	// Result represents the result of the report.
	Result string
	// Report contains the report data.
	Report reports.Report
	// FilteredResourceID contains the resource config ID that was filtered
	// It allows to identify in the report which resource was used to filter the report.
	FilteredResourceID string
	// CreatedAt represents the creation date of the report.
	CreatedAt string
	// UpdatedAt represents the last update date of the report.
	UpdatedAt string
}

// ReportSearchOptions contains options for searching reports.
type ReportSearchOptions struct {
	// Days is the how far to look back for reports from today.
	Days int
}

// SearchReport searches a report by its database record id.
func SearchReport(ctx context.Context, id string) (*model.PipelineReport, error) {
	report := model.PipelineReport{}

	// "SELECT id,data,created_at,updated_at FROM pipelineReports WHERE id=$1"
	query := psql.Select(
		sm.Columns("id", "data", "created_at", "updated_at"),
		sm.From("pipelineReports"),
		sm.Where(psql.Quote("id").EQ(psql.Arg(id))),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	err = DB.QueryRow(ctx, queryString, args...).Scan(
		&report.ID,
		&report.Pipeline,
		&report.Created_at,
		&report.Updated_at,
	)
	if err != nil {
		logrus.Errorf("querying for report: %s", err)
		return nil, err
	}

	return &report, nil
}

// SearchLatestReports searches the latest reports according some parameters.
func SearchLatestReports(ctx context.Context, scmID, sourceID, conditionID, targetID string, options ReportSearchOptions, limit, page int, startTime, endTime string, latest bool) ([]SearchLatestReportData, int, error) {
	queryString := ""
	var args []any

	query := psql.Select(
		sm.From("pipelineReports"),
		sm.Columns(
			"data -> 'ID'",
			"ID",
			"data -> 'PipelineID'",
			"data -> 'Result'",
			"data",
			"created_at",
			"updated_at"),
	)

	if latest {
		query.Apply(
			sm.Distinct("data -> 'ID'"),
			sm.OrderBy("data -> 'ID'"),
		)
	}

	query.Apply(
		sm.OrderBy(psql.Quote("updated_at")).Desc(),
	)

	if err := applyUpdatedAtRangeFilter(DateRangeFilterParams{
		Query:         &query,
		DateRangeDays: options.Days,
		StartTime:     startTime,
		EndTime:       endTime,
	}); err != nil {
		return nil, 0, fmt.Errorf("applying updated_at range filter: %w", err)
	}

	if sourceID != "" {
		if err := applyResourceConfigFilter(&query, sourceID, configSourceType); err != nil {
			return nil, 0, err
		}
	}

	if conditionID != "" {
		if err := applyResourceConfigFilter(&query, conditionID, configConditionType); err != nil {
			return nil, 0, err
		}
	}

	if targetID != "" {
		if err := applyResourceConfigFilter(&query, targetID, configTargetType); err != nil {
			return nil, 0, err
		}
	}

	switch scmID {
	case "":
		// WITH filtered_reports AS (
		// 	SELECT id, data,created_at, updated_at
		// 	FROM pipelineReports
		// 	WHERE
		// 	  updated_at >  current_date - interval '%d day'
		// )
		// SELECT id, data, created_at, updated_at
		// FROM filtered_reports
		// ORDER BY updated_at DESC`

	case "none", "null", "nil":
		// WITH filtered_reports AS (
		// 	SELECT id, data, created_at, updated_at
		// 	FROM pipelineReports
		// 	WHERE
		// 	  	(( cardinality(target_db_scm_ids) = 0 ) OR ( target_db_scm_ids IS NULL )) AND
		//       	( updated_at >  current_date - interval '%d day' )
		// )
		// SELECT DISTINCT ON (data ->> 'Name')
		// 	id,
		// 	data,
		// 	created_at,
		// 	updated_at
		// FROM filtered_reports
		// ORDER BY (data ->> 'Name'), updated_at DESC;`

		query.Apply(
			sm.Where(
				psql.Or(
					psql.Quote("cardinality(target_db_scm_ids) = 0"),
					psql.Quote("target_db_scm_ids").IsNull(),
				),
			),
		)

	default:
		scm, _, err := GetSCM(ctx, scmID, "", "", 0, 1)
		if err != nil {
			logrus.Errorf("get scm data: %s", err)
			return nil, 0, err
		}

		switch len(scm) {
		case 0:
			logrus.Errorf("scm data not found")

		case 1:
			// WITH filtered_reports AS (
			// 	SELECT id, data, created_at, updated_at
			// 	FROM pipelineReports
			// 	WHERE
			// 		( target_db_scm_ids && '{ %q }' ) AND
			// 		( updated_at >  current_date - interval '%d day' )
			// )
			//
			// SELECT DISTINCT ON (data ->> 'Name')
			// 	id,
			// 	data,
			// 	created_at,
			// 	updated_at
			// FROM filtered_reports
			// ORDER BY (data ->> 'Name'), updated_at DESC;

			query.Apply(
				sm.Where(
					psql.Raw(`target_db_scm_ids && ?`, fmt.Sprintf("{%s}", scm[0].ID.String())),
				),
			)

		default:
			// Normally we should never have multiple scms with the same id
			// so we should never reach this point.
			logrus.Errorf("multiple scms found")
		}
	}

	// Total counter query must be built before applying pagination
	// because it needs to count all the reports matching the query.
	totalCountQuery := psql.Select(sm.From(query), sm.Columns("count(*)"))

	totalCountQueryString, totalCountArgs, err := totalCountQuery.Build(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("building total count query failed: %s\n\t%s",
			totalCountQueryString, err)
	}

	totalCount := 0
	if err = DB.QueryRow(ctx, totalCountQueryString, totalCountArgs...).Scan(
		&totalCount,
	); err != nil {
		logrus.Errorf("parsing total count result: %s", err)
	}

	// If limit and page are not set, we do not apply pagination.
	if limit < totalCount && limit > 0 {
		query.Apply(
			sm.Limit(limit),
			sm.Offset((page-1)*limit),
		)
	}

	queryString, args, err = query.Build(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	rows, err := DB.Query(ctx, queryString, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query failed: %q\n\t%s", queryString, err)
	}

	dataset := []SearchLatestReportData{}

	for rows.Next() {
		p := model.PipelineReport{}

		filteredResources := pgtype.Hstore{}

		if sourceID != "" || conditionID != "" || targetID != "" {
			err = rows.Scan(
				&p.ReportID,
				&p.ID,
				&p.PipelineID,
				&p.Result,
				&p.Pipeline,
				&p.Created_at,
				&p.Updated_at,
				&filteredResources,
			)
			if err != nil {
				return nil, 0, fmt.Errorf("parsing result: %s", err)
			}

		} else {
			err = rows.Scan(
				&p.ReportID,
				&p.ID,
				&p.PipelineID,
				&p.Result,
				&p.Pipeline,
				&p.Created_at,
				&p.Updated_at,
			)
			if err != nil {
				return nil, 0, fmt.Errorf("parsing result: %s", err)
			}
		}

		data := SearchLatestReportData{
			ID:        p.ID.String(),
			Name:      p.Pipeline.Name,
			Result:    p.Pipeline.Result,
			Report:    p.Pipeline,
			CreatedAt: p.Created_at.String(),
			UpdatedAt: p.Created_at.String(),
		}

		if sourceID != "" {
			if _, ok := filteredResources[sourceID]; !ok {
				return nil, 0, fmt.Errorf("sourceID %s not found in pipeline report", sourceID)
			}
			data.FilteredResourceID = *filteredResources[sourceID]
		}

		if conditionID != "" {
			if _, ok := filteredResources[conditionID]; !ok {
				return nil, 0, fmt.Errorf("conditionID %s not found in pipeline report", conditionID)
			}
			data.FilteredResourceID = *filteredResources[conditionID]
		}

		if targetID != "" {
			if _, ok := filteredResources[targetID]; !ok {
				return nil, 0, fmt.Errorf("targetID %s not found in pipeline report", targetID)
			}
			data.FilteredResourceID = *filteredResources[targetID]
		}

		dataset = append(dataset, data)
	}

	return dataset, totalCount, nil
}

// InsertReport inserts a new report into the database.
func InsertReport(ctx context.Context, report reports.Report) (string, error) {
	configTargetIDs := pgtype.Hstore{}
	configConditionIDs := pgtype.Hstore{}

	configSourceIDs := buildConfigSources(ctx, report)

	for conditionID, condition := range report.Conditions {
		if condition.Config == nil {
			continue
		}

		c, ok := condition.Config.(map[string]interface{})
		if !ok {
			logrus.Errorf("wrong config condition")
			continue
		}

		kind, ok := c["Kind"].(string)
		if !ok || kind == "" {
			continue
		}

		data, err := json.Marshal(c)
		if err != nil {
			logrus.Errorf("marshaling target config: %s", err)
			continue
		}

		results, _, err := GetTargetConfigs(ctx, kind, "", string(data), 0, 1, false)
		if err != nil {
			logrus.Errorf("failed: %s", err)
			continue
		}

		switch len(results) {
		case 0:
			id, err := InsertConfigResource(ctx, "condition", kind, string(data))
			if err != nil {
				logrus.Errorf("insert config condition data: %s", err)
				continue
			}

			parsedID, err := uuid.Parse(id)
			if err != nil {
				logrus.Errorf("parsing id: %s", err)
			}

			configConditionIDs[parsedID.String()] = stringPtr(conditionID)
		case 1:
			configConditionIDs[results[0].ID.String()] = stringPtr(conditionID)
		default:
			logrus.Warningf("multiple config condition found for %s", conditionID)
			for _, result := range results {
				logrus.Warningf("config condition %s", result.ID)
			}
		}
	}

	var targetDBScmIDs []uuid.UUID
	for targetID, target := range report.Targets {
		if target.Scm.URL != "" && target.Scm.Branch.Target != "" {
			url := target.Scm.URL
			branch := target.Scm.Branch.Target

			ids, _, err := GetSCM(ctx, "", url, branch, 0, 1)
			if err != nil {
				logrus.Errorf("query failed: %s", err)
				return "", err
			}

			switch len(ids) {
			// If no scm is found, we insert it
			case 0:
				id, err := InsertSCM(ctx, target.Scm.URL, target.Scm.Branch.Source)
				if err != nil {
					logrus.Errorf("insert scm data: %s", err)
					continue
				}

				parsedID, err := uuid.Parse(id)
				if err != nil {
					logrus.Errorf("parsing id: %s", err)
				}

				targetDBScmIDs = append(targetDBScmIDs, parsedID)
			default:
				for _, id := range ids {
					if !slices.Contains(targetDBScmIDs, id.ID) {
						targetDBScmIDs = append(targetDBScmIDs, id.ID)
					}
				}
			}
		}

		if target.Config != nil {
			t, ok := target.Config.(map[string]interface{})
			if !ok {
				logrus.Errorf("wrong config target:\n\t%s:\n%v", targetID, target.Config)
				continue
			}

			kind, ok := t["Kind"].(string)
			if !ok || kind == "" {
				logrus.Errorf("wrong config target kind:\n\t%s:\n%v", targetID, target.Config)
				continue
			}

			data, err := json.Marshal(t)
			if err != nil {
				logrus.Errorf("marshaling target config: %s", err)
				continue
			}

			results, _, err := GetTargetConfigs(ctx, kind, "", string(data), 0, 1, false)
			if err != nil {
				logrus.Errorf("failed: %s", err)
				continue
			}

			switch len(results) {
			case 0:
				id, err := InsertConfigResource(ctx, "target", kind, string(data))
				if err != nil {
					logrus.Errorf("insert config target data: %s", err)
					continue
				}

				parsedID, err := uuid.Parse(id)
				if err != nil {
					logrus.Errorf("parsing id: %s", err)
				}

				configTargetIDs[parsedID.String()] = stringPtr(targetID)
			case 1:
				configTargetIDs[results[0].ID.String()] = stringPtr(targetID)
			default:
				logrus.Warningf("multiple config target found for %s", targetID)
				for _, result := range results {
					logrus.Warningf("config target %s", result.ID)
				}
			}
		}
	}

	// INSERT INTO pipelineReports
	// (data, pipeline_id, pipeline_result, pipeline_name, target_db_scm_ids, config_source_ids, config_condition_ids, config_target_ids)
	// VALUES
	// ($1, $2, $3, $4, $5, $6, $7, $8)
	// RETURNING id
	query := psql.Insert(
		im.Into(
			"pipelineReports",
			"data",
			"pipeline_id",
			"pipeline_result",
			"pipeline_name",
			"target_db_scm_ids",
			"config_source_ids",
			"config_condition_ids",
			"config_target_ids",
		),
		im.Values(
			psql.Arg(report),
			psql.Arg(report.ID),
			psql.Arg(report.Result),
			psql.Arg(report.Name),
			psql.Arg(targetDBScmIDs),
			psql.Arg(configSourceIDs),
			psql.Arg(configConditionIDs),
			psql.Arg(configTargetIDs),
		),
		im.Returning("id"),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return "", err
	}

	var reportID uuid.UUID
	err = DB.QueryRow(ctx, queryString, args...).Scan(
		&reportID,
	)
	if err != nil {
		logrus.Errorf("query failed: %s\n\t=> %q", err, queryString)
		return "", err
	}

	return reportID.String(), nil
}

func buildConfigSources(ctx context.Context, report reports.Report) pgtype.Hstore {
	configSourceIDs := pgtype.Hstore{}
	for sourceID, source := range report.Sources {
		if source.Config == nil {
			continue
		}

		s, ok := source.Config.(map[string]interface{})
		if !ok {
			logrus.Errorf("wrong config source:\n\t%s:\n%v", sourceID, source.Config)
			continue
		}

		data, err := json.Marshal(s)
		if err != nil {
			logrus.Errorf("marshaling source config: %s", err)
			continue
		}

		kind, ok := s["Kind"].(string)
		if !ok || kind == "" {
			continue
		}

		results, _, err := GetSourceConfigs(ctx, kind, "", string(data), 0, 1, false)
		if err != nil {
			logrus.Errorf("failed: %s", err)
			continue
		}

		switch len(results) {
		case 0:
			id, err := InsertConfigResource(ctx, "source", kind, string(data))
			if err != nil {
				logrus.Errorf("insert config source data: %s", err)
				continue
			}

			parsedID, err := uuid.Parse(id)
			if err != nil {
				logrus.Errorf("parsing id: %s", err)
			}

			configSourceIDs[parsedID.String()] = stringPtr(sourceID)
		case 1:
			configSourceIDs[results[0].ID.String()] = stringPtr(sourceID)
		default:
			logrus.Warningf("multiple config source found for %s", sourceID)
			for _, result := range results {
				logrus.Warningf("config source %s", result.ID)
			}
		}
	}

	return configSourceIDs
}

// DeleteReport deletes a report from the database.
func DeleteReport(ctx context.Context, id string) error {
	//"DELETE FROM pipelineReports WHERE id=$1"
	query := psql.Delete(
		dm.From("pipelineReports"),
		dm.Where(psql.Quote("id").EQ(psql.Arg(id))),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		return fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	if _, err := DB.Exec(ctx, queryString, args...); err != nil {
		logrus.Errorf("query failed: %s", err)
		return err
	}
	return nil
}

// SearchNumberOfReportsByPipelineID searches the number of reports for a specific pipeline id.
func SearchNumberOfReportsByPipelineID(ctx context.Context, id string) (int, error) {
	// "SELECT COUNT(data) FROM pipelineReports WHERE pipeline_id = $1"

	query := psql.Select(
		sm.Columns("count(data)"),
		sm.From("pipelineReports"),
		sm.Where(psql.Quote("pipeline_id").EQ(psql.Arg(id))),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		return 0, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	var result int
	err = DB.QueryRow(ctx, queryString, args...).Scan(
		&result,
	)

	if err != nil {
		logrus.Errorf("parsing result: %s", err)
		return 0, err
	}

	return result, nil
}

// SearchLatestReportByPipelineID searches the latest report for a specific pipeline id.
func SearchLatestReportByPipelineID(ctx context.Context, id string) (*model.PipelineReport, error) {
	report := model.PipelineReport{}

	// SELECT id,data,created_at,updated_at
	// FROM pipelineReports
	// WHERE pipeline_id = $1
	// ORDER BY updated_at DESC FETCH FIRST 1 ROWS ONLY

	query := psql.Select(
		sm.Columns("id", "data", "created_at", "updated_at"),
		sm.From("pipelineReports"),
		sm.Where(psql.Quote("pipeline_id").EQ(psql.Arg(id))),
		sm.OrderBy(psql.Quote("updated_at")).Desc(),
		sm.Limit(1),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	err = DB.QueryRow(ctx, queryString, args...).Scan(
		&report.ID,
		&report.Pipeline,
		&report.Created_at,
		&report.Updated_at,
	)

	if err != nil {
		logrus.Errorf("parsing result: %s", err)
		return nil, err
	}

	return &report, nil
}

// applyResourceConfigFilters applies resource config filters to the given query.
func applyResourceConfigFilter(query *bob.BaseQuery[*dialect.SelectQuery], id, kind string) error {

	// Ensure resource id is a valid UUID
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("parsing %sID: %w", kind, err)
	}

	query.Apply(
		sm.Where(
			psql.Raw(fmt.Sprintf(`config_%s_ids \? ?`, kind), id),
		),
		sm.Columns(fmt.Sprintf("config_%s_ids", kind)),
	)
	return nil
}
