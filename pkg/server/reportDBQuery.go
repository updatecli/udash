package server

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

// dbInsertReport inserts a new report into the database.
func dbInsertReport(report reports.Report) (string, error) {
	var ID uuid.UUID
	var targetDBScmIDs []uuid.UUID

	for _, target := range report.Targets {
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
	}

	query := `INSERT INTO pipelineReports
	(data, pipeline_id, pipeline_result, pipeline_name, target_db_scm_ids)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id
`
	err := database.DB.QueryRow(context.Background(), query, report, report.ID, report.Result, report.Name, targetDBScmIDs).Scan(
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
