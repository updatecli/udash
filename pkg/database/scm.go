package database

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/model"

	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/sm"
)

// InsertSCM creates a new SCM and inserts it into the database.
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

// GetSCMParams contains the filters used to look up scms.
type GetSCMParams struct {
	// ID restricts the lookup to a specific scm.
	ID string
	// URL restricts the lookup to the scms of a repository.
	URL string
	// Branch restricts the lookup to the scms of a branch.
	Branch string
	// StartTime and EndTime restrict the lookup to the scms a report was published for
	// within that range. Both must be provided, an empty range does not filter anything
	// out.
	StartTime string
	EndTime   string
	// Limit is the maximum number of scms to return, a value lower than one returns
	// them all. Page is one based.
	Limit int
	Page  int
}

// GetSCM returns a list of scms from the scm database table.
func GetSCM(ctx context.Context, params GetSCMParams) ([]model.SCM, int, error) {
	query := psql.Select(
		sm.Columns("id", "branch", "url", "created_at", "updated_at"),
		sm.From("scms"),
	)

	if params.ID != "" {
		query.Apply(
			sm.Where(psql.Quote("id").EQ(psql.Arg(params.ID))),
		)
	}

	if params.URL != "" {
		query.Apply(
			sm.Where(psql.Quote("url").EQ(psql.Arg(params.URL))),
		)
	}

	if params.Branch != "" {
		query.Apply(
			sm.Where(psql.Quote("branch").EQ(psql.Arg(params.Branch))),
		)
	}

	// An scm is only interesting for a time range if a report was published for it during
	// that range, which is what the trigger of migration 000009 records. Filtering on
	// created_at would instead answer when the repository was first seen.
	if err := applyRangeFilter(
		"last_pipeline_report_at",
		dateRangeFilterParams{
			Query:         &query,
			DateRangeDays: 0,
			StartTime:     params.StartTime,
			EndTime:       params.EndTime,
		}); err != nil {
		return nil, 0, fmt.Errorf("applying last_pipeline_report_at range filter: %w", err)
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

	applyPagination(&query, params.Limit, params.Page)

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

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("reading scms: %w", err)
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
	// TotalActionURLs is the total number of unique action URLs for this SCM.
	TotalActionURLs int `json:"total_action_urls"`
	// TotalOpenActionByResult is a map of result types to the number of pipelines in that
	// result which also carry an open action, such as a pull request still waiting to be
	// merged. It is a breakdown of TotalResultByType, so its counts are always lower than
	// or equal to the matching ones there.
	//
	// Unlike TotalActionURLs, which counts distinct action URLs, this counts pipelines: a
	// single pull request grouping the changes of several pipelines is counted once there
	// and once per pipeline here.
	TotalOpenActionByResult map[string]int `json:"total_open_action_by_result"`
}

// SCMBranchDataset represents a map of branches and their summary data for a single SCM URL.
type SCMBranchDataset map[string]ScmSummaryData

// SCMDataset represents the response for the FindSCMSummary endpoint.
type SCMDataset struct {
	Data map[string]SCMBranchDataset `json:"data"`
}

type GetSCMSummaryParams struct {
	MonitoringDurationDays int
	StartTime              string
	EndTime                string
	Labels                 map[string]string
	// Results restricts the summary to the reports whose pipeline result is one of
	// them. An empty list does not filter anything out.
	Results []string
	// OpenAction restricts the summary to the pipelines which carry an open action, such as
	// a pull request still waiting to be merged, or to the ones which do not. A nil value
	// does not filter anything out.
	OpenAction   *bool
	TotalCount   int
	TotalActions int
	Ctx          context.Context
	ScmRows      []model.SCM
}

// GetSCMSummary returns a list of scms summary from the scm database table.
//
// Every scm is summarized by a single query. A summary covers every scm reported within
// its range, so running the query, and the label lookup, once per scm made it slower with
// every repository.
func GetSCMSummary(params GetSCMSummaryParams) (*SCMDataset, error) {

	dataset := SCMDataset{}

	summaries := map[uuid.UUID]*scmSummary{}
	scmIDs := []uuid.UUID{}

	for _, row := range params.ScmRows {
		if row.Branch == "" || row.URL == "" {
			logrus.Debugf("skipping scm %s, missing branch or url", row.ID)
			continue
		}

		if _, ok := summaries[row.ID]; !ok {
			summaries[row.ID] = newSCMSummary(row.ID)
			scmIDs = append(scmIDs, row.ID)
		}
	}

	if len(scmIDs) > 0 {
		if err := summarizeSCMs(params, scmIDs, summaries); err != nil {
			return nil, err
		}
	}

	for _, row := range params.ScmRows {
		summary, ok := summaries[row.ID]
		if !ok {
			continue
		}

		if dataset.Data == nil {
			dataset.Data = make(map[string]SCMBranchDataset)
		}

		if dataset.Data[row.URL] == nil {
			dataset.Data[row.URL] = make(map[string]ScmSummaryData)
		}

		dataset.Data[row.URL][row.Branch] = summary.data()
	}

	return &dataset, nil
}

// scmSummary accumulates the summary of a single scm.
type scmSummary struct {
	ScmSummaryData
	// actionURLs contains every distinct action URL found for the scm.
	actionURLs map[string]bool
}

func newSCMSummary(id uuid.UUID) *scmSummary {
	return &scmSummary{
		ScmSummaryData: ScmSummaryData{
			ID:                      id.String(),
			TotalResultByType:       make(map[string]int),
			TotalOpenActionByResult: make(map[string]int),
		},
		actionURLs: make(map[string]bool),
	}
}

// data returns the summary with its totals computed.
func (s *scmSummary) data() ScmSummaryData {
	data := s.ScmSummaryData
	for r := range data.TotalResultByType {
		data.TotalResult += data.TotalResultByType[r]
	}
	data.TotalActionURLs = len(s.actionURLs)

	return data
}

// summarizeSCMs adds to the summary of each scm where every pipeline reporting to it
// stands, according to its latest report within the range.
func summarizeSCMs(params GetSCMSummaryParams, scmIDs []uuid.UUID, summaries map[uuid.UUID]*scmSummary) error {

	filteredReports := psql.Select(
		sm.From("pipelineReports"),
		sm.Columns("id", "pipeline_id", "pipeline_result", "updated_at", "target_db_scm_ids"),
		sm.Where(psql.Raw("target_db_scm_ids && ?", psql.Arg(scmIDs))),
	)

	if err := applyRangeFilter(
		"updated_at",
		dateRangeFilterParams{
			Query:         &filteredReports,
			DateRangeDays: params.MonitoringDurationDays,
			StartTime:     params.StartTime,
			EndTime:       params.EndTime,
		}); err != nil {
		return fmt.Errorf("applying updated_at range filter: %w", err)
	}

	if len(params.Labels) > 0 {
		if err := applyLabelFilter(labelFilterParams{
			Ctx:       params.Ctx,
			Query:     &filteredReports,
			Labels:    params.Labels,
			StartTime: params.StartTime,
			EndTime:   params.EndTime,
		}); err != nil {
			return fmt.Errorf("applying label filter: %w", err)
		}
	}

	// A report may belong to several scms, so it is repeated once per scm before the latest
	// report of every pipeline is picked, per scm. That choice only reads columns: the
	// payload is read afterwards, for the reports picked only.
	latestReports := psql.Select(
		sm.Distinct("s.scm_id", "r.pipeline_id"),
		sm.Columns("s.scm_id", "r.id", "r.pipeline_result"),
		sm.From(psql.Raw("filtered_reports AS r CROSS JOIN LATERAL unnest(r.target_db_scm_ids) AS s(scm_id)")),
		sm.Where(psql.Raw("s.scm_id = ANY(?)", psql.Arg(scmIDs))),
		sm.OrderBy("s.scm_id"),
		sm.OrderBy("r.pipeline_id"),
		sm.OrderBy("r.updated_at").Desc(),
	)

	query := psql.Select(
		sm.With("filtered_reports").As(filteredReports),
		sm.With("latest_reports").As(latestReports),
		// The action URLs are read with the same jsonpath as openActionSQLExpr, so that
		// a pipeline counted as carrying an open action here is the one the reports
		// search and the reports summary would return too.
		sm.Columns("l.scm_id", "l.pipeline_result", "jsonb_path_query_array(p.data, '$.Actions.*.actionUrl')"),
		sm.From("latest_reports").As("l"),
		sm.InnerJoin("pipelineReports").As("p").On(psql.Raw("p.id = l.id")),
	)

	queryString, queryArgs, err := query.Build(params.Ctx)
	if err != nil {
		return fmt.Errorf("building scm summary query: %w", err)
	}

	rows, err := DB.Query(params.Ctx, queryString, queryArgs...)
	if err != nil {
		return fmt.Errorf("querying scm summary: %w", err)
	}
	defer rows.Close()

	for rows.Next() {

		scmID := uuid.UUID{}
		result := ""
		actionUrls := []string{}

		if err := rows.Scan(&scmID, &result, &actionUrls); err != nil {
			return fmt.Errorf("scanning scm summary row: %w", err)
		}

		summary, ok := summaries[scmID]
		if !ok {
			continue
		}

		hasOpenAction := len(actionUrls) > 0

		// The results and the open actions are dropped here rather than in the query
		// above on purpose. That query keeps the latest report of every pipeline, so
		// this summary reports where each pipeline stands now; filtering the reports
		// before that would instead keep the latest report which happened to carry one
		// of those results, reporting a pipeline as failing long after it
		// recovered.
		if len(params.Results) > 0 && !slices.Contains(params.Results, result) {
			continue
		}

		if params.OpenAction != nil && *params.OpenAction != hasOpenAction {
			continue
		}

		summary.TotalResultByType[result]++

		if hasOpenAction {
			summary.TotalOpenActionByResult[result]++
		}

		for i := range actionUrls {
			summary.actionURLs[actionUrls[i]] = true
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading scm summary: %w", err)
	}

	return nil
}
