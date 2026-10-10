package database

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/dm"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	"github.com/updatecli/updatecli/pkg/core/result"
)

// OpenActionPipeline is a pipeline whose latest report carries an open action.
type OpenActionPipeline struct {
	// ID is the database record id of the latest report of the pipeline.
	ID string `json:"id"`
	// Name is the name of the pipeline.
	Name string `json:"name"`
	// Result is the result of the latest report of the pipeline.
	Result string `json:"result"`
	// UpdatedAt is the last update date of the latest report of the pipeline.
	UpdatedAt time.Time `json:"updated_at"`
}

// OpenActionData is an action left open, such as a pull request still waiting to be
// merged, along with every pipeline feeding it.
type OpenActionData struct {
	// URL is the link of the action, which identifies it.
	URL string `json:"url"`
	// Title is the title of the action, as reported by its most recent pipeline.
	Title string `json:"title"`
	// Repository is the url of the git repository of the first target of the action carrying
	// an scm, as reported by its most recent pipeline. It falls back to the first target of
	// that pipeline carrying one, and is empty when none does.
	Repository string `json:"repository"`
	// Branch is the target branch of that same scm, which the pull request is merged into.
	Branch string `json:"branch"`
	// UpdatedAt is the last update date of its most recent pipeline.
	UpdatedAt time.Time `json:"updated_at"`
	// Pipelines contains every pipeline feeding the action, most recent first.
	Pipelines []OpenActionPipeline `json:"pipelines"`
	// Acknowledgement is only set on acknowledged actions, which the search returns
	// when SearchOpenActionsParams.Acknowledged is true.
	Acknowledgement *OpenActionAcknowledgement `json:"acknowledgement,omitempty"`
}

// OpenActionAcknowledgement records who acknowledged an open action, and until when.
type OpenActionAcknowledgement struct {
	// Until is when the acknowledgement expires. It lifts earlier if the worst result changes.
	Until time.Time `json:"until"`
	// By is who acknowledged the action. It is empty when authentication is off.
	By string `json:"by"`
}

// SearchOpenActionsParams contains the filters used to search the actions left open.
type SearchOpenActionsParams struct {
	Ctx       context.Context
	Options   ReportSearchOptions
	StartTime string
	EndTime   string
	Labels    map[string]string
	// ScmID restricts the search to the pipelines targeting that scm. An empty value does
	// not filter anything out, "none" only keeps the pipelines targeting none.
	ScmID string
	// Results restricts the search to the actions fed by at least one pipeline whose
	// latest result is one of them. An empty list does not filter anything out.
	Results []string
	// Acknowledged returns only the acknowledged actions instead of only the others.
	Acknowledged bool
	Limit        int
	Page         int
}

// SearchOpenActions returns the actions left open by the latest report of every pipeline,
// one entry per action URL.
//
// Several pipelines may feed the same action: manifests sharing a pipelineid push to the
// same branch, so Updatecli groups their changes into a single pull request. A pipeline
// whose report ID changed between two runs also keeps its previous report as the latest
// one of a pipeline of its own. Grouping by URL lists each pull request once either way.
//
// That previous report is not superseded by the reports of the new ID though, nothing links
// the two. So once the pull request is merged, it is still listed from the previous report
// until that report falls out of the searched time range. The reports search reads the
// latest reports the same way and shares this limit.
//
// It returns one page of actions, the total number of actions matching the request,
// and the number of acknowledged actions.
func SearchOpenActions(params SearchOpenActionsParams) ([]OpenActionData, int, int, error) {
	filteredReports := psql.Select(
		sm.From("pipelineReports"),
		sm.Columns("id", "pipeline_id", "pipeline_result", "updated_at"),
	)

	if err := applyRangeFilter(
		"updated_at",
		dateRangeFilterParams{
			Query:         &filteredReports,
			DateRangeDays: params.Options.Days,
			StartTime:     params.StartTime,
			EndTime:       params.EndTime,
		}); err != nil {
		return nil, 0, 0, fmt.Errorf("applying updated_at range filter: %w", err)
	}

	if len(params.Labels) > 0 {
		if err := applyLabelFilter(labelFilterParams{
			Ctx:    params.Ctx,
			Query:  &filteredReports,
			Labels: params.Labels,
		}); err != nil {
			return nil, 0, 0, fmt.Errorf("applying label filter: %w", err)
		}
	}

	// Like the scm summary, the scm narrows the reports before the latest one of every
	// pipeline is picked.
	if err := applyScmFilter(params.Ctx, &filteredReports, params.ScmID); err != nil {
		return nil, 0, 0, fmt.Errorf("applying scm filter: %w", err)
	}

	// The latest report of every pipeline is picked from the columns alone, the payload is
	// only read afterwards for the reports picked.
	latestReports := psql.Select(
		sm.Distinct("pipeline_id"),
		sm.Columns("id", "pipeline_id", "pipeline_result", "updated_at"),
		sm.From("filtered_reports"),
		sm.OrderBy("pipeline_id"),
		sm.OrderBy("updated_at").Desc(),
		sm.OrderBy("id"),
	)

	// Every action carrying a link is read with the same jsonpath as openActionSQLExpr.
	// jsonb_path_query is used rather than jsonb_each, which fails on a report whose
	// Actions is not an object, and rather than the "?" operator, which bob reads as a
	// placeholder. A report listing the same link twice still counts once per action.
	//
	// The repository and branch are read from a target of the action carrying an scm.
	// Updatecli lists the targets of an action by the sha256 of their key in the report
	// Targets, so those keys are hashed the same way to find them. The first target of the
	// report carrying an scm is only used when none of the action targets carries one, such
	// as with reports listing no target in their actions. The branch is the target one,
	// which the scms are stored by.
	//
	// jsonb_each is only given an object, it fails on anything else.
	openActions := psql.Select(
		sm.Distinct("a.action ->> 'actionUrl'", "l.id"),
		sm.Columns(
			psql.Raw("a.action ->> 'actionUrl'").As("url"),
			psql.Raw("COALESCE(a.action ->> 'title', '')").As("title"),
			psql.Raw("COALESCE(s.scm ->> 'URL', '')").As("repository"),
			psql.Raw("COALESCE(s.scm -> 'Branch' ->> 'Target', '')").As("branch"),
			psql.Raw("COALESCE(p.data ->> 'Name', '')").As("name"),
			"l.id",
			"l.pipeline_result",
			"l.updated_at",
		),
		sm.From(psql.Raw(`latest_reports AS l
			INNER JOIN pipelineReports AS p ON p.id = l.id
			CROSS JOIN LATERAL jsonb_path_query(p.data, '$.Actions.*') AS a(action)
			LEFT JOIN LATERAL (
				SELECT t.target -> 'Scm' AS scm
				FROM jsonb_each(
					CASE WHEN jsonb_typeof(p.data -> 'Targets') = 'object'
						THEN p.data -> 'Targets'
						ELSE '{}'::jsonb
					END
				) WITH ORDINALITY AS t(key, target, position)
				WHERE t.target -> 'Scm' ->> 'URL' <> ''
				ORDER BY
					EXISTS (
						SELECT 1
						FROM jsonb_path_query(a.action, '$.targets[*].ID') AS at(id)
						WHERE at.id = to_jsonb(encode(sha256(convert_to(t.key, 'UTF8')), 'hex'))
					) DESC,
					t.position
				LIMIT 1
			) AS s ON true`)),
		sm.Where(psql.Raw("a.action ->> 'actionUrl' IS NOT NULL")),
		sm.OrderBy(psql.Raw("a.action ->> 'actionUrl'")),
		sm.OrderBy("l.id"),
	)

	// The most recent pipeline of an action gives it its title and repository.
	//
	// updated_at is a timestamp without time zone holding UTC, it is marked as such before
	// being written to json so that it reads back as RFC3339.
	groupedActions := psql.Select(
		sm.Columns(
			"url",
			psql.Raw("(array_agg(title ORDER BY updated_at DESC, id))[1]").As("title"),
			psql.Raw("(array_agg(repository ORDER BY updated_at DESC, id))[1]").As("repository"),
			psql.Raw("(array_agg(branch ORDER BY updated_at DESC, id))[1]").As("branch"),
			psql.Raw("max(updated_at)").As("updated_at"),
			psql.Raw("jsonb_agg(jsonb_build_object('id', id, 'name', name, 'result', pipeline_result, 'updated_at', updated_at AT TIME ZONE 'UTC') ORDER BY updated_at DESC, id)").As("pipelines"),
			worstResultExpr().As("worst_result"),
		),
		sm.From("open_actions"),
		sm.GroupBy("url"),
	)

	// The result filter keeps or drops an action as a whole, from the latest report of its
	// pipelines: "failing" means failing now, like the reports search. Filtering the
	// pipelines before grouping them would instead drop the others from the action, and
	// take its title from whichever pipeline is left.
	if len(params.Results) > 0 {
		groupedActions.Apply(sm.Having(psql.F("bool_or", resultInExpr(params.Results))))
	}

	// An acknowledgement holds while it has not expired and the action's worst result
	// still matches the one stored with it. The worst result comes from the reports this
	// search reads, so a time range covering older reports can bring back an action that
	// was acknowledged later.
	//
	// until is a UTC wall-clock timestamp, which is what the driver sends for a UTC time.
	actions := psql.Raw(`grouped_actions AS g
		LEFT JOIN action_acks AS k
			ON k.url = g.url AND k.until > ? AND k.worst_result = g.worst_result`,
		time.Now().UTC(),
	)

	with := []bob.Mod[*dialect.SelectQuery]{
		sm.With("filtered_reports").As(filteredReports),
		sm.With("latest_reports").As(latestReports),
		sm.With("open_actions").As(openActions),
		sm.With("grouped_actions").As(groupedActions),
	}

	// Count before pagination. The acknowledged count lets the list say how many
	// actions it hides.
	countQuery := psql.Select(append(with,
		sm.Columns(
			"count(*) FILTER (WHERE k.url IS NULL)",
			"count(*) FILTER (WHERE k.url IS NOT NULL)",
		),
		sm.From(actions),
	)...)

	countQueryString, countArgs, err := countQuery.Build(params.Ctx)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("building count query failed: %s\n\t%s", countQueryString, err)
	}

	unacknowledgedCount, acknowledgedCount := 0, 0
	if err := DB.QueryRow(params.Ctx, countQueryString, countArgs...).Scan(&unacknowledgedCount, &acknowledgedCount); err != nil {
		return nil, 0, 0, fmt.Errorf("counting open actions: %w", err)
	}

	totalCount := unacknowledgedCount
	acknowledged := "k.url IS NULL"
	if params.Acknowledged {
		totalCount = acknowledgedCount
		acknowledged = "k.url IS NOT NULL"
	}

	query := psql.Select(append(with,
		sm.Columns(
			"g.url",
			"g.title",
			"g.repository",
			"g.branch",
			"g.updated_at",
			"g.pipelines",
			"k.until",
			"k.acknowledged_by",
		),
		sm.From(actions),
		sm.Where(psql.Raw(acknowledged)),
		sm.OrderBy("g.updated_at").Desc(),
		sm.OrderBy("g.url"),
	)...)
	applyPagination(&query, params.Limit, params.Page)

	queryString, args, err := query.Build(params.Ctx)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	rows, err := DB.Query(params.Ctx, queryString, args...)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("query failed: %q\n\t%s", queryString, err)
	}
	defer rows.Close()

	dataset := []OpenActionData{}
	for rows.Next() {
		action := OpenActionData{}
		pipelines := []byte{}
		var until *time.Time
		var by *string

		if err := rows.Scan(
			&action.URL,
			&action.Title,
			&action.Repository,
			&action.Branch,
			&action.UpdatedAt,
			&pipelines,
			&until,
			&by,
		); err != nil {
			return nil, 0, 0, fmt.Errorf("parsing result: %s", err)
		}

		if err := json.Unmarshal(pipelines, &action.Pipelines); err != nil {
			return nil, 0, 0, fmt.Errorf("parsing pipelines of %q: %s", action.URL, err)
		}

		if until != nil {
			action.Acknowledgement = &OpenActionAcknowledgement{Until: until.UTC(), By: *by}
		}

		dataset = append(dataset, action)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("reading open actions: %w", err)
	}

	return dataset, totalCount, acknowledgedCount, nil
}

// worstResultExpr selects the worst latest result among the pipelines feeding an action,
// ranked by ActionResults. An unrecognized result is ranked and returned as ResultUnknown.
//
// The results are passed as arguments because bob treats "?" in raw SQL as a placeholder.
func worstResultExpr() psql.Expression {
	known := make([]any, 0, len(ActionResults))
	whens := ""
	for rank, result := range ActionResults {
		if result == ResultUnknown {
			continue
		}
		known = append(known, result)
		whens += fmt.Sprintf(" WHEN ? THEN %d", rank)
	}

	args := []any{}
	args = append(args, known...)
	args = append(args, ResultUnknown)
	args = append(args, known...)
	args = append(args, slices.Index(ActionResults, ResultUnknown))

	return psql.Raw(fmt.Sprintf(
		"(array_agg(CASE WHEN pipeline_result IN (%s) THEN pipeline_result ELSE ? END ORDER BY CASE pipeline_result%s ELSE ? END, updated_at DESC, id))[1]",
		strings.TrimSuffix(strings.Repeat("?, ", len(known)), ", "), whens,
	), args...)
}

// ResultUnknown replaces any result Udash does not recognize.
const ResultUnknown = "?"

// ActionResults lists the results an action can be acknowledged with, worst first:
// failure, attention (changed), unknown, skipped, success.
var ActionResults = []string{result.FAILURE, result.ATTENTION, ResultUnknown, result.SKIPPED, result.SUCCESS}

// AcknowledgeAction hides an open action until the given time, or until its worst result
// differs from worstResult. Acknowledging an action again replaces the previous
// acknowledgement.
func AcknowledgeAction(ctx context.Context, url string, until time.Time, worstResult, by string) error {
	query := psql.Insert(
		im.Into("action_acks", "url", "until", "worst_result", "acknowledged_by", "created_at"),
		im.Values(
			psql.Arg(url),
			psql.Arg(until.UTC()),
			psql.Arg(worstResult),
			psql.Arg(by),
			psql.Arg(time.Now().UTC()),
		),
		im.OnConflict("url").DoUpdate(
			im.SetExcluded("until", "worst_result", "acknowledged_by", "created_at"),
		),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		return fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	if _, err := DB.Exec(ctx, queryString, args...); err != nil {
		return fmt.Errorf("acknowledging %q: %w", url, err)
	}

	return nil
}

// UnacknowledgeAction removes the acknowledgement of an action, if there is one.
func UnacknowledgeAction(ctx context.Context, url string) error {
	query := psql.Delete(
		dm.From("action_acks"),
		dm.Where(psql.Quote("url").EQ(psql.Arg(url))),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		return fmt.Errorf("building query failed: %s\n\t%s", queryString, err)
	}

	if _, err := DB.Exec(ctx, queryString, args...); err != nil {
		return fmt.Errorf("unacknowledging %q: %w", url, err)
	}

	return nil
}
