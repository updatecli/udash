package server

import (
	"context"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

func dbInsertReport(p reports.Report) (string, error) {
	var ID uuid.UUID

	query := "INSERT INTO pipelineReports (data) VALUES ($1) RETURNING id"

	err := database.DB.QueryRow(context.Background(), query, p).Scan(
		&ID,
	)

	if err != nil {
		logrus.Errorf("query failed: %s", err)
		return "", err
	}

	return ID.String(), nil
}

func dbDeleteReport(id string) error {
	query := "DELETE FROM pipelineReports WHERE id=$1"

	if _, err := database.DB.Exec(context.Background(), query, id); err != nil {
		logrus.Errorf("query failed: %s", err)
		return err
	}
	return nil
}

func dbSearchReport(id string) (*PipelineRow, error) {
	report := PipelineRow{}

	err := database.DB.QueryRow(context.Background(), "select * from pipelineReports where id=$1", id).Scan(
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

func dbSearchNumberOfReportsByID(id string) (int, error) {
	var result int

	err := database.DB.QueryRow(context.Background(), "SELECT COUNT(data) FROM pipelineReports WHERE data ->> 'ID' = $1", id).Scan(
		&result,
	)

	if err != nil {
		logrus.Errorf("parsing result: %s", err)
		return 0, err
	}

	return result, nil
}

func dbSearchLatestReportByID(id string) (*PipelineRow, error) {
	report := PipelineRow{}

	err := database.DB.QueryRow(context.Background(), "select * from pipelineReports where data ->> 'ID'=$1 ORDER BY updated_at DESC FETCH FIRST 1 ROWS ONLY", id).Scan(
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
