package server

import (
	"context"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"

	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/sm"
)

func dbInsertSCM(url, branch string) (string, error) {

	var ID uuid.UUID

	//"INSERT INTO scms (url, branch) VALUES ($1, $2) RETURNING id"
	query := psql.Insert(
		im.Into("scms", "url", "branch"),
		im.Values(psql.Arg(url), psql.Arg(branch)),
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
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return "", err
	}

	return ID.String(), nil
}

// dbGetSCM returns a list of scms from the scm database table.
func dbGetScm(id, url, branch string) ([]model.SCM, error) {

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

	ctx := context.Background()
	queryString, args, err := query.Build(ctx)

	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, err
	}

	rows, err := database.DB.Query(ctx, queryString, args...)
	if err != nil {
		logrus.Errorf("query failed: %s\n\t%s", queryString, err)
		return nil, err
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

	return results, nil
}
