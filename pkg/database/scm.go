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

// InsertSCM creates a new SCM and inserts it into the database.
//
// It returns the ID of the newly created SCM.
func InsertSCM(ctx context.Context, url, branch string) (string, error) {
	//"INSERT INTO scms (url, branch) VALUES ($1, $2) RETURNING id"
	query := psql.Insert(
		im.Into("scms", "url", "branch"),
		im.Values(psql.Arg(url), psql.Arg(branch)),
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

// GetSCM returns a list of scms from the scm database table.
func GetSCM(ctx context.Context, id, url, branch string, limit, page int) ([]model.SCM, int, error) {
	query := psql.Select(
		sm.Columns("id", "branch", "url", "created_at", "updated_at"),
		sm.From("scms"),
	)

	if id != "" {
		query.Apply(
			sm.Where(psql.Quote("id").EQ(psql.Arg(id))),
		)
	}

	if url != "" {
		query.Apply(
			sm.Where(psql.Quote("url").EQ(psql.Arg(url))),
		)
	}

	if branch != "" {
		query.Apply(
			sm.Where(psql.Quote("branch").EQ(psql.Arg(branch))),
		)
	}

	// Get total scm count
	// Get total count of results
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

	results := []model.SCM{}

	for rows.Next() {
		r := model.SCM{}

		err = rows.Scan(&r.ID, &r.Branch, &r.URL, &r.Created_at, &r.Updated_at)
		if err != nil {
			logrus.Errorf("scanning scm row failed: %s", err)
			continue
		}

		if r.URL == "" || r.Branch == "" {
			continue
		}

		results = append(results, r)
	}

	return results, totalCount, nil
}

// ScmSummaryData represents the summary data for a single SCM.
type ScmSummaryData struct {
	// ID is the unique identifier of the SCM.
	ID string `json:"id"`
	// TotalResultByType is a map of result types and their counts.
	TotalResultByType map[string]int `json:"total_result_by_type"`
	// TotalResult is the total number of results for this SCM.
	TotalResult int `json:"total_result"`
}

// SCMBranchDataset represents a map of branches and their summary data for a single SCM URL.
type SCMBranchDataset map[string]ScmSummaryData

// SCMDataset represents the response for the FindSCMSummary endpoint.
type SCMDataset struct {
	Data map[string]SCMBranchDataset `json:"data"`
}

// GetSCMSummary returns a list of scms summary from the scm database table.
func GetSCMSummary(ctx context.Context, scmRows []model.SCM, totalCount, monitoringDurationDays int) (*SCMDataset, error) {

	dataset := SCMDataset{}

	for _, row := range scmRows {

		scmID := row.ID
		scmURL := row.URL
		scmBranch := row.Branch

		if scmBranch == "" || scmURL == "" {
			logrus.Debugf("skipping scm %s, missing branch or url", row.ID)
			continue
		}

		filteredSCMsQuery := psql.Select(
			sm.From("pipelineReports"),
			sm.Where(
				psql.Raw("target_db_scm_ids && ?",
					psql.Arg(fmt.Sprintf("{%s}", scmID)),
				),
			),
			sm.Where(
				psql.Raw(fmt.Sprintf("updated_at > current_date - interval '%d day'", monitoringDurationDays)),
			),
			sm.Columns("id", "data", "updated_at"),
		)

		query := psql.Select(
			sm.Distinct(
				psql.Raw("data ->> 'ID'"),
			),
			sm.With("filtered_reports").As(filteredSCMsQuery),
			sm.Columns("id", "data ->> 'Result'"),
			sm.From("filtered_reports"),
			sm.OrderBy(psql.Raw("data ->> 'ID'")),
			sm.OrderBy(psql.Quote("updated_at")).Desc(),
		)

		queryString, queryArgs, err := query.Build(ctx)
		if err != nil {
			return nil, fmt.Errorf("building scm summary query: %w", err)
		}

		rows, err := DB.Query(context.Background(), queryString, queryArgs...)
		if err != nil {
			return nil, fmt.Errorf("querying scm summary: %w", err)
		}

		if dataset.Data == nil {
			dataset.Data = make(map[string]SCMBranchDataset)
		}

		if dataset.Data[scmURL] == nil {
			dataset.Data[scmURL] = make(map[string]ScmSummaryData)
		}

		d := ScmSummaryData{
			ID:                scmID.String(),
			TotalResultByType: make(map[string]int),
		}

		dataset.Data[scmURL][scmBranch] = d

		for rows.Next() {

			id := ""
			result := ""

			err = rows.Scan(&id, &result)
			if err != nil {
				return nil, fmt.Errorf("scanning scm summary row: %w", err)
			}

			resultFound := false
			for r := range dataset.Data[scmURL][scmBranch].TotalResultByType {
				if r == result {
					dataset.Data[scmURL][scmBranch].TotalResultByType[r]++
					resultFound = true
				}
			}

			if !resultFound {
				dataset.Data[scmURL][scmBranch].TotalResultByType[result] = 1
			}
		}

		scmData := dataset.Data[scmURL][scmBranch]
		for r := range scmData.TotalResultByType {
			scmData.TotalResult += scmData.TotalResultByType[r]
		}
		dataset.Data[scmURL][scmBranch] = scmData
	}
	return &dataset, nil
}
