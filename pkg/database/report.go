package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

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
	"github.com/updatecli/updatecli/pkg/core/result"
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
	// TargetConfigIDs contains the config target IDs associated with the report.
	TargetConfigIDs pgtype.Hstore
	// ConditionConfigIDs contains the config condition IDs associated with the report.
	ConditionConfigIDs pgtype.Hstore
	// SourceConfigIDs contains the config source IDs associated with the report.
	SourceConfigIDs pgtype.Hstore
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
		sm.Columns("id", "data", "created_at", "updated_at", "config_target_ids", "config_condition_ids", "config_source_ids"),
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
		&report.TargetConfigIDs,
		&report.ConditionConfigIDs,
		&report.SourceConfigIDs,
	)
	if err != nil {
		logrus.Errorf("querying for report: %s", err)
		return nil, err
	}

	return &report, nil
}

type SearchLatestReportsParams struct {
	Ctx         context.Context
	ScmID       string
	SourceID    string
	ConditionID string
	TargetID    string
	Options     ReportSearchOptions
	StartTime   string
	EndTime     string
	Limit       int
	Page        int
	Latest      bool
	Labels      map[string]string
	// Results restricts the search to the reports whose pipeline result is one of
	// them. An empty list does not filter anything out.
	Results []string
	// OpenAction restricts the search to the reports which carry an open action, such as
	// a pull request still waiting to be merged, or to the ones which do not. A nil value
	// does not filter anything out.
	OpenAction *bool
}

// SearchLatestReports searches the latest reports according some parameters.
func SearchLatestReports(params SearchLatestReportsParams) ([]SearchLatestReportData, int, error) {
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
			"updated_at",
			"config_target_ids", "config_condition_ids", "config_source_ids",
		),
	)

	if params.Latest {
		query.Apply(sm.Distinct("data -> 'ID'"), sm.OrderBy("data -> 'ID'"))
	}

	if len(params.Labels) > 0 {
		err := applyLabelFilter(labelFilterParams{
			Query:     &query,
			Labels:    params.Labels,
			StartTime: params.StartTime,
			EndTime:   params.EndTime,
			Ctx:       params.Ctx,
		})
		if err != nil {
			return nil, 0, err
		}
	}

	query.Apply(sm.OrderBy(psql.Quote("updated_at")).Desc())

	if err := applyRangeFilter(
		"updated_at",
		dateRangeFilterParams{
			Query:         &query,
			DateRangeDays: params.Options.Days,
			StartTime:     params.StartTime,
			EndTime:       params.EndTime,
		}); err != nil {
		return nil, 0, fmt.Errorf("applying updated_at range filter: %w", err)
	}

	// Every applied filter adds a column to the select, so the filters are collected
	// here and the rows are scanned against that same list further down. Reading the
	// three of them independently would build a query returning more columns than the
	// scan expects as soon as two are combined.
	resourceFilters := []resourceConfigFilter{}
	if params.SourceID != "" {
		resourceFilters = append(resourceFilters, resourceConfigFilter{ID: params.SourceID, Kind: configSourceType})
	}

	if params.ConditionID != "" {
		resourceFilters = append(resourceFilters, resourceConfigFilter{ID: params.ConditionID, Kind: configConditionType})
	}

	if params.TargetID != "" {
		resourceFilters = append(resourceFilters, resourceConfigFilter{ID: params.TargetID, Kind: configTargetType})
	}

	for _, filter := range resourceFilters {
		if err := applyResourceConfigFilter(&query, filter.ID, filter.Kind); err != nil {
			return nil, 0, err
		}
	}

	if err := applyScmFilter(params.Ctx, &query, params.ScmID); err != nil {
		return nil, 0, err
	}

	applyResultFilter(&query, params.Results)
	applyOpenActionFilter(&query, params.OpenAction)

	// Total counter query must be built before applying pagination
	// because it needs to count all the reports matching the query.
	totalCountQuery := psql.Select(sm.From(query), sm.Columns("count(*)"))

	totalCountQueryString, totalCountArgs, err := totalCountQuery.Build(params.Ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("building total count query failed: %s\n\t%s",
			totalCountQueryString, err)
	}

	totalCount := 0
	if err = DB.QueryRow(params.Ctx, totalCountQueryString, totalCountArgs...).Scan(
		&totalCount,
	); err != nil {
		logrus.Errorf("get reports: %s", err)
	}

	applyPagination(&query, params.Limit, params.Page)

	queryString, args, err = query.Build(params.Ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	rows, err := DB.Query(params.Ctx, queryString, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query failed: %q\n\t%s", queryString, err)
	}
	defer rows.Close()

	dataset := []SearchLatestReportData{}
	for rows.Next() {
		p := model.PipelineReport{}

		// One extra column per applied resource config filter, in the order they were
		// applied to the query.
		filteredResources := make([]pgtype.Hstore, len(resourceFilters))

		scanTargets := []any{
			&p.ReportID,
			&p.ID,
			&p.PipelineID,
			&p.Result,
			&p.Pipeline,
			&p.Created_at,
			&p.Updated_at,
			&p.TargetConfigIDs,
			&p.ConditionConfigIDs,
			&p.SourceConfigIDs,
		}

		for i := range filteredResources {
			scanTargets = append(scanTargets, &filteredResources[i])
		}

		if err := rows.Scan(scanTargets...); err != nil {
			return nil, 0, fmt.Errorf("parsing result: %s", err)
		}

		data := SearchLatestReportData{
			ID:                 p.ID.String(),
			Name:               p.Pipeline.Name,
			Result:             p.Pipeline.Result,
			Report:             p.Pipeline,
			CreatedAt:          p.Created_at.String(),
			UpdatedAt:          p.Updated_at.String(),
			TargetConfigIDs:    p.TargetConfigIDs,
			ConditionConfigIDs: p.ConditionConfigIDs,
			SourceConfigIDs:    p.SourceConfigIDs,
		}

		// When several filters are combined the last one wins, as it did when they were
		// read one after the other.
		for i, filter := range resourceFilters {
			resourceID, ok := filteredResources[i][filter.ID]
			if !ok || resourceID == nil {
				return nil, 0, fmt.Errorf("%sID %s not found in pipeline report", filter.Kind, filter.ID)
			}

			data.FilteredResourceID = *resourceID
		}

		dataset = append(dataset, data)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("reading results: %s", err)
	}

	return dataset, totalCount, nil
}

// SummaryGranularity is the size of the time buckets a reports summary is grouped by.
type SummaryGranularity string

const (
	// SummaryGranularityHour groups the reports per UTC hour.
	SummaryGranularityHour SummaryGranularity = "hour"
	// SummaryGranularityDay groups the reports per UTC day.
	SummaryGranularityDay SummaryGranularity = "day"
	// SummaryGranularityWeek groups the reports per ISO week, starting on monday.
	SummaryGranularityWeek SummaryGranularity = "week"
	// SummaryGranularityMonth groups the reports per calendar month.
	SummaryGranularityMonth SummaryGranularity = "month"
)

// IsValid reports whether the granularity is one this package knows how to bucket.
func (g SummaryGranularity) IsValid() bool {
	switch g {
	case SummaryGranularityHour, SummaryGranularityDay, SummaryGranularityWeek, SummaryGranularityMonth:
		return true
	default:
		return false
	}
}

// ErrSummaryRangeTooWide is returned when the requested time range spans more days than
// the caller allows. Callers are expected to turn it into a client error.
var ErrSummaryRangeTooWide = errors.New("requested time range is too wide")

// ErrSummaryTooManyBuckets is returned when the requested time range and granularity would
// produce more buckets than the caller allows. Callers are expected to turn it into a
// client error.
var ErrSummaryTooManyBuckets = errors.New("requested time range produces too many buckets")

// summaryDateFormat is the layout used to identify the bucket of a summary entry. It has to
// carry the time of the day, otherwise every bucket of an hourly summary would share the
// same identifier and their counts would be merged together.
const summaryDateFormat = time.RFC3339

// summaryUnknownResult is the key reporting the reports whose result is empty or is not
// an Updatecli result.
const summaryUnknownResult = "unknown"

// summaryResultKeys contains the keys always reported for a bucket, even when no report
// matched, so that consumers always retrieve the same set of keys.
var summaryResultKeys = []string{
	result.SUCCESS,
	result.FAILURE,
	result.ATTENTION,
	result.SKIPPED,
	summaryUnknownResult,
}

// ReportSummaryParams contains the parameters used to summarize reports per time bucket.
type ReportSummaryParams struct {
	Ctx context.Context
	// Days is how far back to look for reports, in days.
	// It is ignored when Hours, or StartTime and EndTime, are provided.
	Days int
	// Hours is how far back to look for reports, in hours. It takes precedence over
	// Days and is ignored when StartTime and EndTime are provided.
	Hours int
	// Granularity is the size of the time buckets, it defaults to a day.
	Granularity SummaryGranularity
	// MaxDays is the widest time range accepted, in days. A value lower than one
	// does not enforce any limit.
	MaxDays int
	// MaxBuckets is the largest number of buckets a summary may return. A value lower
	// than one does not enforce any limit.
	MaxBuckets int
	// StartTime and EndTime define an explicit time range, both must be provided.
	StartTime string
	EndTime   string
	// ScmID restricts the summary to the reports of a specific scm.
	ScmID string
	// Labels restricts the summary to the reports matching those labels.
	Labels map[string]string
	// Results restricts the summary to the reports whose pipeline result is one of
	// them. An empty list does not filter anything out.
	Results []string
	// OpenAction restricts the summary to the reports which carry an open action, such as
	// a pull request still waiting to be merged, or to the ones which do not. A nil value
	// does not filter anything out.
	OpenAction *bool
}

// ReportResultSummaryEntry contains the number of reports per result for a single time bucket.
type ReportResultSummaryEntry struct {
	// Date is the start of the bucket, in UTC, formatted as RFC3339.
	Date string `json:"date"`
	// Results contains the number of reports per Updatecli result for that bucket.
	Results map[string]int `json:"results"`
	// OpenActions contains, for each Updatecli result, how many of the reports counted in
	// Results also carry an open action, such as a pull request still waiting to be merged.
	// It is a breakdown of Results, not an addition to it, so its counts are always lower
	// than or equal to the matching ones in Results.
	//
	// The interesting one is the count reported under the success result: those pipelines
	// ran fine and had nothing to change only because the change is already waiting in a
	// pull request.
	OpenActions map[string]int `json:"open_actions"`
	// Total is the number of reports for that bucket, all results combined.
	Total int `json:"total"`
}

// SearchReportsSummary returns the number of reports per result for each time bucket of
// the requested time range. Buckets without any report are reported with a zeroed entry
// so that the returned dataset always covers the whole time range.
//
// The summary always covers whole buckets: an explicit time range is widened to the
// buckets it overlaps, otherwise a partial bucket would be reported as a drop of activity.
func SearchReportsSummary(params ReportSummaryParams) ([]ReportResultSummaryEntry, int, error) {

	granularity := params.Granularity
	if granularity == "" {
		granularity = SummaryGranularityDay
	}

	if !granularity.IsValid() {
		return nil, 0, fmt.Errorf("unsupported granularity %q", params.Granularity)
	}

	firstBucket, lastBucket, err := summaryRange(params, granularity)
	if err != nil {
		return nil, 0, fmt.Errorf("resolving summary range: %w", err)
	}

	// granularity is one of the constants above, never the raw value received from a
	// caller, so it cannot inject anything into the query.
	dateTrunc := fmt.Sprintf("date_trunc('%s', updated_at)", granularity)

	query := psql.Select(
		sm.From("pipelineReports"),
		sm.Columns(
			dateTrunc,
			// pipeline_result is denormalized from data ->> 'Result' when the report is
			// inserted, grouping on it avoids parsing the jsonb document of every report.
			"pipeline_result",
			openActionSQLExpr,
			"count(*)",
		),
		sm.Where(
			psql.Raw("updated_at >= ? AND updated_at < ?", firstBucket, nextBucket(lastBucket, granularity)),
		),
		sm.GroupBy(dateTrunc),
		sm.GroupBy("pipeline_result"),
		sm.GroupBy(openActionSQLExpr),
		sm.OrderBy(dateTrunc),
	)

	if err := applyScmFilter(params.Ctx, &query, params.ScmID); err != nil {
		return nil, 0, err
	}

	applyResultFilter(&query, params.Results)
	applyOpenActionFilter(&query, params.OpenAction)

	if len(params.Labels) > 0 {
		// The report window is widened to whole buckets so the label lookup must cover
		// the same range, otherwise labels timestamped within the widened part would be
		// missed and their reports silently dropped. An empty range keeps the lookup
		// unbounded, as SearchLatestReports does.
		labelStartTime, labelEndTime := "", ""
		if params.StartTime != "" || params.EndTime != "" {
			labelStartTime = firstBucket.Format(timeRangeLayout)
			labelEndTime = nextBucket(lastBucket, granularity).Format(timeRangeLayout)
		}

		if err := applyLabelFilter(labelFilterParams{
			Ctx:       params.Ctx,
			Query:     &query,
			Labels:    params.Labels,
			StartTime: labelStartTime,
			EndTime:   labelEndTime,
		}); err != nil {
			return nil, 0, fmt.Errorf("applying label filter: %w", err)
		}
	}

	queryString, args, err := query.Build(params.Ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	rows, err := DB.Query(params.Ctx, queryString, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query failed: %q\n\t%s", queryString, err)
	}
	defer rows.Close()

	countByDate := map[string]map[string]int{}
	openActionCountByDate := map[string]map[string]int{}
	totalCount := 0

	for rows.Next() {
		bucket := time.Time{}
		reportResult := ""
		hasOpenAction := false
		count := 0

		if err := rows.Scan(&bucket, &reportResult, &hasOpenAction, &count); err != nil {
			return nil, 0, fmt.Errorf("parsing result: %s", err)
		}

		date := bucket.UTC().Format(summaryDateFormat)
		if countByDate[date] == nil {
			countByDate[date] = map[string]int{}
			openActionCountByDate[date] = map[string]int{}
		}

		resultKey := summaryResultKey(reportResult)

		countByDate[date][resultKey] += count
		if hasOpenAction {
			openActionCountByDate[date][resultKey] += count
		}
		totalCount += count
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("reading results: %s", err)
	}

	dataset := []ReportResultSummaryEntry{}
	for bucket := firstBucket; !bucket.After(lastBucket); bucket = nextBucket(bucket, granularity) {
		entry := ReportResultSummaryEntry{
			Date:        bucket.Format(summaryDateFormat),
			Results:     map[string]int{},
			OpenActions: map[string]int{},
		}

		for _, r := range summaryResultKeys {
			entry.Results[r] = 0
			entry.OpenActions[r] = 0
		}

		for r, count := range countByDate[entry.Date] {
			entry.Results[r] += count
			entry.Total += count
		}

		for r, count := range openActionCountByDate[entry.Date] {
			entry.OpenActions[r] += count
		}

		dataset = append(dataset, entry)
	}

	return dataset, totalCount, nil
}

// summaryResultKey maps a stored pipeline result to the key it is reported under.
// Anything unexpected, including the empty result of a report inserted before the
// pipeline_result column was backfilled, is folded into a single bucket so that the
// reported keys stay stable.
func summaryResultKey(pipelineResult string) string {
	switch pipelineResult {
	case result.SUCCESS, result.FAILURE, result.ATTENTION, result.SKIPPED:
		return pipelineResult
	default:
		return summaryUnknownResult
	}
}

// summaryRange returns the first and the last bucket, both included, covered by a
// summary. Both are the start of a bucket, in UTC.
func summaryRange(params ReportSummaryParams, granularity SummaryGranularity) (time.Time, time.Time, error) {

	var firstTime time.Time
	var lastTime time.Time

	switch {
	case params.StartTime != "" || params.EndTime != "":
		var err error
		firstTime, lastTime, err = resolveTimeRange(0, params.StartTime, params.EndTime)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}

	case params.Hours > 0:
		// The window includes the bucket of the current hour, as the Days one includes
		// the bucket of the current day.
		lastTime = time.Now().UTC()
		firstTime = lastTime.Add(-time.Duration(params.Hours-1) * time.Hour)

	default:
		days := params.Days
		if days < 1 {
			days = 1
		}

		lastTime = time.Now().UTC()
		firstTime = lastTime.AddDate(0, 0, -(days - 1))
	}

	// The limit is checked against the requested range rather than the widened one:
	// widening adds up to a bucket on each side, which a month granularity would
	// otherwise turn into a rejection of a request that is within the limit.
	if params.MaxDays > 0 && lastTime.Sub(firstTime) > time.Duration(params.MaxDays)*24*time.Hour {
		return time.Time{}, time.Time{}, ErrSummaryRangeTooWide
	}

	firstBucket := truncateToBucket(firstTime, granularity)
	lastBucket := truncateToBucket(lastTime, granularity)

	// MaxDays bounds how much of the table the query scans, this bounds how large the
	// response gets: an hourly summary of a year is a cheap scan but ~8800 entries.
	if params.MaxBuckets > 0 {
		count := 0
		for bucket := firstBucket; !bucket.After(lastBucket); bucket = nextBucket(bucket, granularity) {
			count++
			if count > params.MaxBuckets {
				return time.Time{}, time.Time{}, ErrSummaryTooManyBuckets
			}
		}
	}

	return firstBucket, lastBucket, nil
}

// truncateToBucket returns the start, in UTC, of the bucket containing the provided time.
// It must return the same instant as the matching date_trunc call, otherwise the zeroed
// buckets would not line up with the counted rows.
func truncateToBucket(t time.Time, granularity SummaryGranularity) time.Time {
	t = t.UTC()

	switch granularity {
	case SummaryGranularityHour:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	case SummaryGranularityWeek:
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		// date_trunc truncates a week to its ISO monday.
		return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	case SummaryGranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

// nextBucket returns the start of the bucket following the provided bucket start.
func nextBucket(t time.Time, granularity SummaryGranularity) time.Time {
	switch granularity {
	case SummaryGranularityHour:
		return t.Add(time.Hour)
	case SummaryGranularityWeek:
		return t.AddDate(0, 0, 7)
	case SummaryGranularityMonth:
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 1)
	}
}

// InsertReport inserts a new report into the database.
// Publisher identifies who published a report.
//
// Both fields are optional: an instance running without authentication has nobody
// to attribute a report to, and a report published from the browser has no token.
type Publisher struct {
	// Subject is the identity provider subject which published the report.
	Subject *string
	// TokenID is the API token used, when one was.
	TokenID *uuid.UUID
}

func InsertReport(ctx context.Context, report reports.Report, publisher Publisher) (string, error) {
	var err error
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

		results, _, err := GetTargetConfigs(ctx, kind, "", string(data), 0, 1)
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

			ids, _, err := GetSCM(ctx, GetSCMParams{URL: url, Branch: branch})
			if err != nil {
				logrus.Errorf("query failed: %s", err)
				return "", err
			}

			switch len(ids) {
			// If no scm is found, we insert it
			case 0:
				// The branch inserted must be the one looked up above. Storing
				// Branch.Source instead made the lookup of the next report miss the row
				// every time the two differ, which is the normal case when Updatecli
				// pushes its changes to a dedicated branch, and appended a duplicate scm
				// on every published report.
				id, err := InsertSCM(ctx, url, branch)
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

			results, _, err := GetTargetConfigs(ctx, kind, "", string(data), 0, 1)
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

	labelIDs := []uuid.UUID{}
	if len(report.Labels) > 0 {
		labelIDs, err = InitLabels(ctx, report.Labels)
		if err != nil {
			return "", fmt.Errorf("initializing labels: %w", err)
		}
	}

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
			"label_ids",
			"created_by_subject",
			"created_by_token_id",
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
			psql.Arg(labelIDs),
			psql.Arg(publisher.Subject),
			psql.Arg(publisher.TokenID),
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

		results, _, err := GetSourceConfigs(ctx, kind, "", string(data), 0, 1)
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
		sm.Columns("id", "data", "created_at", "updated_at", "config_target_ids", "config_condition_ids", "config_source_ids"),
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
		&report.TargetConfigIDs,
		&report.ConditionConfigIDs,
		&report.SourceConfigIDs,
	)

	if err != nil {
		logrus.Errorf("parsing result: %s", err)
		return nil, err
	}

	return &report, nil
}

// resourceConfigFilter identifies a resource config a reports search is restricted to.
type resourceConfigFilter struct {
	// ID is the config resource id the reports must reference.
	ID string
	// Kind is one of configSourceType, configConditionType or configTargetType.
	Kind string
}

// applyResourceConfigFilters applies resource config filters to the given query.
//
// It also selects the matching hstore column so that the caller can report which resource
// of the pipeline matched, which means every call adds one column to the query.
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

// applyResultFilter restricts the given query to the reports whose pipeline result is
// one of those given. An empty list does not filter anything out.
//
// pipeline_result is denormalized from data ->> 'Result' when the report is inserted,
// and indexed alongside updated_at, so this does not have to reach into the jsonb
// payload. A result which is not an Updatecli one simply matches no report, rather
// than being silently dropped from the filter.
func applyResultFilter(query *bob.BaseQuery[*dialect.SelectQuery], results []string) {
	if len(results) == 0 {
		return
	}

	args := make([]bob.Expression, len(results))
	for i := range results {
		args[i] = psql.Arg(results[i])
	}

	query.Apply(sm.Where(psql.Quote("pipeline_result").In(args...)))
}

// openActionSQLExpr is true of the reports carrying at least one action left open, which is
// how Updatecli reports a pull request still waiting to be merged.
//
// reports.Action.Link is serialized as "actionUrl" and omitted when empty, and Updatecli
// only ever fills it from an open pull request: CheckActionExist queries the forge for open
// pull requests only, and the pull request handler resets the link when it closes one. So
// the presence of that key is a self clearing marker, and it is already true of every report
// stored so far rather than only of the ones produced from now on.
//
// The jsonpath is inlined rather than bound as a parameter on purpose: an expression index
// only matches a literal expression, so binding it would cost
// idx_pipelinereports_updated_at_result_open_action. It contains no user input.
//
// It must also stay free of the jsonpath filter operator: bob reads "?" as a placeholder,
// so a path such as '$.Actions.*.actionUrl ? (@ != "")' silently consumes an argument and
// builds a query which matches nothing. Guarding against an empty link is unnecessary
// anyway, "actionUrl" is omitempty so it is absent rather than empty.
const openActionSQLExpr = `jsonb_path_exists(data, '$.Actions.*.actionUrl')`

// applyOpenActionFilter restricts the given query to the reports which do, or which do not,
// carry an open action. A nil openAction does not filter anything out.
//
// This is deliberately a dimension of its own rather than a fifth pipeline result: an open
// action is orthogonal to the result. A pipeline may have succeeded because its change is
// already in an open pull request, but it may also have changed something and just opened
// one, or be failing while a pull request from a previous run is still around.
func applyOpenActionFilter(query *bob.BaseQuery[*dialect.SelectQuery], openAction *bool) {
	if openAction == nil {
		return
	}

	query.Apply(sm.Where(psql.Raw(openActionSQLExpr+" = ?", psql.Arg(*openAction))))
}

// applyScmFilter restricts the given query to the reports associated to a specific scm.
// An empty scmID does not filter anything while "none", "null", or "nil" only keeps
// the reports which are not associated to any scm.
func applyScmFilter(ctx context.Context, query *bob.BaseQuery[*dialect.SelectQuery], scmID string) error {

	switch scmID {
	case "":
	case "none", "null", "nil":
		// psql.Quote would quote the whole expression as a column identifier,
		// so the cardinality call must be passed as a raw expression.
		query.Apply(
			sm.Where(
				psql.Or(
					psql.Raw("cardinality(target_db_scm_ids) = 0"),
					psql.Quote("target_db_scm_ids").IsNull(),
				),
			),
		)

	default:
		scm, _, err := GetSCM(ctx, GetSCMParams{ID: scmID})
		if err != nil {
			logrus.Errorf("get scm data: %s", err)
			return err
		}

		switch len(scm) {
		case 0:
			logrus.Errorf("scm data not found")
		case 1:
			query.Apply(
				sm.Where(
					psql.Raw(`target_db_scm_ids && ?`, fmt.Sprintf("{%s}", scm[0].ID.String())),
				),
			)
		default:
			// Normally we should never have multiple scms with the same id
			// so we should never reach this point.
			logrus.Errorf("unexpected behavior: multiple scms found")
		}
	}

	return nil
}
