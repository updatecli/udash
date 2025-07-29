package server

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sirupsen/logrus"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dm"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

func stringPtr(s string) *string {
	return &s
}

// dbInsertReport inserts a new report into the database.
func dbInsertReport(report reports.Report) (string, error) {
	var ID uuid.UUID
	var targetDBScmIDs []uuid.UUID

	configTargetIDs := pgtype.Hstore{}
	configConditionIDs := pgtype.Hstore{}

	configSourceIDs := buildConfigSources(report)

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

		results, err := dbGetConfigTarget(kind, "", string(data))
		if err != nil {
			logrus.Errorf("failed: %s", err)
			continue
		}

		switch len(results) {
		case 0:
			id, err := dbInsertConfigResource("condition", kind, string(data))
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

	for targetID, target := range report.Targets {
		if target.Scm.URL != "" && target.Scm.Branch.Target != "" {
			url := target.Scm.URL
			branch := target.Scm.Branch.Target

			ids, err := database.GetScm("", url, branch)
			if err != nil {
				logrus.Errorf("query failed: %s", err)
				return "", err
			}

			switch len(ids) {
			// If no scm is found, we insert it
			case 0:
				id, err := database.InsertSCM(target.Scm.URL, target.Scm.Branch.Source)
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

			results, err := dbGetConfigTarget(kind, "", string(data))
			if err != nil {
				logrus.Errorf("failed: %s", err)
				continue
			}

			switch len(results) {
			case 0:
				id, err := dbInsertConfigResource("target", kind, string(data))
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

	ctx := context.Background()
	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return "", err
	}

	err = database.DB.QueryRow(context.Background(), queryString, args...).Scan(
		&ID,
	)
	if err != nil {
		logrus.Errorf("query failed: %s\n\t=> %q", err, queryString)
		return "", err
	}

	return ID.String(), nil
}

func buildConfigSources(report reports.Report) pgtype.Hstore {
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

		results, err := dbGetConfigSource(kind, "", string(data))
		if err != nil {
			logrus.Errorf("failed: %s", err)
			continue
		}

		switch len(results) {
		case 0:
			id, err := dbInsertConfigResource("source", kind, string(data))
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

// dbDeleteReport deletes a report from the database.
func dbDeleteReport(id string) error {

	//"DELETE FROM pipelineReports WHERE id=$1"
	query := psql.Delete(
		dm.From("pipelineReports"),
		dm.Where(psql.Quote("id").EQ(psql.Arg(id))),
	)

	ctx := context.Background()
	queryString, args, err := query.Build(ctx)
	if err != nil {
		return fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	if _, err := database.DB.Exec(context.Background(), queryString, args...); err != nil {
		logrus.Errorf("query failed: %s", err)
		return err
	}
	return nil
}

// dbSearchReport searches a report by its database record id.
func dbSearchReport(id string) (*model.PipelineReport, error) {
	report := model.PipelineReport{}

	// "SELECT id,data,created_at,updated_at FROM pipelineReports WHERE id=$1"
	query := psql.Select(
		sm.Columns("id", "data", "created_at", "updated_at"),
		sm.From("pipelineReports"),
		sm.Where(psql.Quote("id").EQ(psql.Arg(id))),
	)

	queryString, args, err := query.Build(context.Background())
	if err != nil {
		return nil, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	err = database.DB.QueryRow(context.Background(), queryString, args...).Scan(
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

// dbSearchNumberOfReportsByPipelineID searches the number of reports for a specific pipeline id.
func dbSearchNumberOfReportsByPipelineID(id string) (int, error) {
	var result int

	// "SELECT COUNT(data) FROM pipelineReports WHERE pipeline_id = $1"

	query := psql.Select(
		sm.Columns("count(data)"),
		sm.From("pipelineReports"),
		sm.Where(psql.Quote("pipeline_id").EQ(psql.Arg(id))),
	)

	queryString, args, err := query.Build(context.Background())
	if err != nil {
		return 0, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	err = database.DB.QueryRow(context.Background(), queryString, args...).Scan(
		&result,
	)

	if err != nil {
		logrus.Errorf("parsing result: %s", err)
		return 0, err
	}

	return result, nil
}

// dbSearchLatestReportByPipelineID searches the latest report for a specific pipeline id.
func dbSearchLatestReportByPipelineID(id string) (*model.PipelineReport, error) {
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

	queryString, args, err := query.Build(context.Background())
	if err != nil {
		return nil, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	err = database.DB.QueryRow(context.Background(), queryString, args...).Scan(
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

type respSearchLatestReportData struct {
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

// dbSearchLatestReport searches the latest reports according some parameters.
func dbSearchLatestReport(scmID, sourceID, conditionID, targetID string) ([]respSearchLatestReportData, error) {

	queryString := ""
	var args []any
	var err error

	filteredReportsQuery := psql.Select(
		sm.From("pipelineReports"),
		sm.Columns("id", "data", "config_source_ids", "config_condition_ids", "config_target_ids", "target_db_scm_ids", "created_at", "updated_at"),
		sm.Where(
			psql.Raw(fmt.Sprintf("updated_at > current_date - interval '%d day'", monitoringDurationDays)),
		),
	)

	query := psql.Select(
		sm.Distinct(
			"data -> 'ID'",
			"target_db_scm_ids",
		),
		sm.With("filtered_reports").As(filteredReportsQuery),
		sm.Columns("id", "data", "created_at", "updated_at"),
		sm.From("filtered_reports"),
		sm.OrderBy("data -> 'ID'"),
		sm.OrderBy("target_db_scm_ids"),
		sm.OrderBy(psql.Quote("updated_at")).Desc(),
	)

	if sourceID != "" {

		// Ensure sourceID is a valid UUID
		_, err = uuid.Parse(sourceID)
		if err != nil {
			return nil, fmt.Errorf("parsing sourceID: %s", err)
		}

		filteredReportsQuery.Apply(
			sm.Where(
				psql.Raw(`config_source_ids \? ?`, sourceID),
			),
		)

		query.Apply(
			sm.Columns("config_source_ids"),
		)
	}

	if conditionID != "" {
		// Ensure conditionID is a valid UUID
		_, err = uuid.Parse(conditionID)
		if err != nil {
			return nil, fmt.Errorf("parsing conditionID: %s", err)
		}

		filteredReportsQuery.Apply(
			sm.Where(
				psql.Raw(`config_condition_ids \? ?`, conditionID),
			),
		)

		query.Apply(
			sm.Columns("config_condition_ids"),
		)
	}

	if targetID != "" {
		// Ensure targetID is a valid UUID
		_, err = uuid.Parse(targetID)
		if err != nil {
			return nil, fmt.Errorf("parsing targetID: %s", err)
		}

		filteredReportsQuery.Apply(
			sm.Where(
				psql.Raw(`config_target_ids \? ?`, targetID),
			),
		)
		query.Apply(
			sm.Columns("config_target_ids"),
		)
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

		filteredReportsQuery.Apply(
			sm.Where(
				psql.Or(
					psql.Quote("cardinality(target_db_scm_ids) = 0"),
					psql.Quote("target_db_scm_ids").IsNull(),
				),
			),
		)

	default:
		scm, err := database.GetScm(scmID, "", "")
		if err != nil {
			logrus.Errorf("get scm data: %s", err)
			return nil, err
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

			filteredReportsQuery.Apply(
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

	queryString, args, err = query.Build(context.Background())
	if err != nil {
		return nil, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	rows, err := database.DB.Query(context.Background(), queryString, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %q\n\t%s", queryString, err)
	}

	dataset := []respSearchLatestReportData{}

	for rows.Next() {
		p := model.PipelineReport{}

		filteredResources := pgtype.Hstore{}

		if sourceID != "" || conditionID != "" || targetID != "" {
			err = rows.Scan(&p.ID, &p.Pipeline, &p.Created_at, &p.Updated_at, &filteredResources)
			if err != nil {
				return nil, fmt.Errorf("parsing result: %s", err)
			}

		} else {
			err = rows.Scan(&p.ID, &p.Pipeline, &p.Created_at, &p.Updated_at)
			if err != nil {
				return nil, fmt.Errorf("parsing result: %s", err)
			}
		}

		data := respSearchLatestReportData{
			ID:        p.ID.String(),
			Name:      p.Pipeline.Name,
			Result:    p.Pipeline.Result,
			Report:    p.Pipeline,
			CreatedAt: p.Created_at.String(),
			UpdatedAt: p.Created_at.String(),
		}

		if sourceID != "" {
			if _, ok := filteredResources[sourceID]; !ok {
				return nil, fmt.Errorf("sourceID %s not found in pipeline report", sourceID)
			}
			data.FilteredResourceID = *filteredResources[sourceID]
		}

		if conditionID != "" {
			if _, ok := filteredResources[conditionID]; !ok {
				return nil, fmt.Errorf("conditionID %s not found in pipeline report", conditionID)
			}
			data.FilteredResourceID = *filteredResources[conditionID]
		}

		if targetID != "" {
			if _, ok := filteredResources[targetID]; !ok {
				return nil, fmt.Errorf("targetID %s not found in pipeline report", targetID)
			}
			data.FilteredResourceID = *filteredResources[targetID]
		}

		dataset = append(dataset, data)
	}

	return dataset, nil
}
