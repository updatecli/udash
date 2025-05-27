package server

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sirupsen/logrus"
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

			ids, err := dbGetScm("", url, branch)
			if err != nil {
				logrus.Errorf("query failed: %s", err)
				return "", err
			}

			switch len(ids) {
			// If no scm is found, we insert it
			case 0:
				id, err := dbInsertSCM(target.Scm.URL, target.Scm.Branch.Source)
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

	query := `INSERT INTO pipelineReports
	(data, pipeline_id, pipeline_result, pipeline_name, target_db_scm_ids, config_source_ids, config_condition_ids, config_target_ids)
	VALUES
	($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id
`
	err := database.DB.QueryRow(context.Background(), query,
		report,
		report.ID,
		report.Result,
		report.Name,
		targetDBScmIDs,
		configSourceIDs,
		configConditionIDs,
		configTargetIDs).Scan(
		&ID,
	)
	if err != nil {
		logrus.Errorf("query failed: %s\n\t=> %q", err, query)
		return "", err
	}

	return ID.String(), nil
}

// dbDeleteReport deletes a report from the database.
func dbDeleteReport(id string) error {
	query := "DELETE FROM pipelineReports WHERE id=$1"

	if _, err := database.DB.Exec(context.Background(), query, id); err != nil {
		logrus.Errorf("query failed: %s", err)
		return err
	}
	return nil
}

// dbSearchReport searches a report by its database record id.
func dbSearchReport(id string) (*model.PipelineReport, error) {
	report := model.PipelineReport{}

	query := "SELECT id,data,created_at,updated_at FROM pipelineReports WHERE id=$1"

	err := database.DB.QueryRow(context.Background(), query, id).Scan(
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

	query := "SELECT COUNT(data) FROM pipelineReports WHERE pipeline_id = $1"

	err := database.DB.QueryRow(context.Background(), query, id).Scan(
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

	query := `SELECT id,data,created_at,updated_at
	FROM pipelineReports
	WHERE pipeline_id = $1
	ORDER BY updated_at DESC FETCH FIRST 1 ROWS ONLY
`

	err := database.DB.QueryRow(context.Background(), query, id).Scan(
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
	ID        string
	Name      string
	Result    string
	CreatedAt string
	UpdatedAt string
}

// dbSearchLatestReport searches the latest reports according some parameters.
func dbSearchLatestReport(scmID, sourceID, conditionID, targetID string) ([]respSearchLatestReportData, error) {

	query := ""

	switch scmID {
	case "":
		query = `
WITH filtered_reports AS (
	SELECT id, data,created_at, updated_at
	FROM pipelineReports
	WHERE
	  updated_at >  current_date - interval '%d day'
)
SELECT id, data, created_at, updated_at
FROM filtered_reports
ORDER BY updated_at DESC`
		query = fmt.Sprintf(query, monitoringDurationDays)

	case "none", "null", "nil":
		query = `
WITH filtered_reports AS (
	SELECT id, data, created_at, updated_at
	FROM pipelineReports
	WHERE
	  	(( cardinality(target_db_scm_ids) = 0 ) OR ( target_db_scm_ids IS NULL )) AND
      	( updated_at >  current_date - interval '%d day' )
)
SELECT DISTINCT ON (data ->> 'Name')
	id,
	data,
	created_at,
	updated_at
FROM filtered_reports
ORDER BY (data ->> 'Name'), updated_at DESC;`

		query = fmt.Sprintf(query, monitoringDurationDays)

	default:
		scm, err := dbGetScm(scmID, "", "")
		if err != nil {
			logrus.Errorf("get scm data: %s", err)
			return nil, err
		}

		switch len(scm) {
		case 0:
			logrus.Errorf("scm data not found")

		case 1:
			query = `
WITH filtered_reports AS (
	SELECT id, data, created_at, updated_at
	FROM pipelineReports
	WHERE
		( target_db_scm_ids && '{ %q }' ) AND
		( updated_at >  current_date - interval '%d day' )
)

SELECT DISTINCT ON (data ->> 'Name')
	id,
	data,
	created_at,
	updated_at
FROM filtered_reports
ORDER BY (data ->> 'Name'), updated_at DESC;
`

			query = fmt.Sprintf(query, scm[0].ID, monitoringDurationDays)

		default:
			// Normally we should never have multiple scms with the same id
			// so we should never reach this point.
			logrus.Errorf("multiple scms found")
		}
	}

	rows, err := database.DB.Query(context.Background(), query)
	if err != nil {
		logrus.Errorf("query failed: %s", err)
		return nil, err
	}

	dataset := []respSearchLatestReportData{}

	for rows.Next() {
		p := model.PipelineReport{}

		err = rows.Scan(&p.ID, &p.Pipeline, &p.Created_at, &p.Updated_at)
		if err != nil {
			logrus.Errorf("parsing result: %s", err)
			return nil, err
		}

		data := respSearchLatestReportData{
			ID:        p.ID.String(),
			Name:      p.Pipeline.Name,
			Result:    p.Pipeline.Result,
			CreatedAt: p.Created_at.String(),
			UpdatedAt: p.Created_at.String(),
		}

		dataset = append(dataset, data)
	}

	return dataset, nil
}
