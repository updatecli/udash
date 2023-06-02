package server

import (
	"context"

	"github.com/olblak/udash/pkg/database"
	"github.com/sirupsen/logrus"
)

func dbInsertReport(p PipelineReport) error {
	query := "INSERT INTO pipelineReports (data) VALUES ($1)"

	_, err := database.DB.Exec(context.Background(), query, p)
	if err != nil {
		logrus.Errorf("query failed: %s", err)
		return err
	}

	return nil
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

func dbSearchNumberOfReportsByName(occurrence string) (int, error) {
	var result int

	err := database.DB.QueryRow(context.Background(), "SELECT COUNT(data) FROM pipelineReports WHERE data ->> 'Name' = $1", occurrence).Scan(
		&result,
	)

	if err != nil {
		logrus.Errorf("parsing result: %s", err)
		return 0, err
	}

	return result, nil
}

func dbSearchLatestReportByName(reportName string) (*PipelineRow, error) {
	report := PipelineRow{}

	err := database.DB.QueryRow(context.Background(), "select * from pipelineReports where data ->> 'Name'=$1 ORDER BY updated_at DESC FETCH FIRST 1 ROWS ONLY", reportName).Scan(
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
