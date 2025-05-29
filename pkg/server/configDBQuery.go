package server

import (
	"context"
	"fmt"

	"encoding/json"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dm"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
)

const (
	// configSourceTableName defines the table name for config sources
	configSourceTableName = "config_sources"
	// configConditionTableName defines the table name for config conditions
	configConditionTableName = "config_conditions"
	// configTargetTableName defines the table name for config targets
	configTargetTableName = "config_targets"
)

// dbInsertConfigResource inserts a new resource configuration into the database.
func dbInsertConfigResource(resourceType string, resourceKind string, resourceConfig interface{}) (string, error) {
	var ID uuid.UUID

	table := ""
	switch resourceType {
	case "source":
		table = configSourceTableName
	case "condition":
		table = configConditionTableName
	case "target":
		table = configTargetTableName
	default:
		return "", fmt.Errorf("unknown resource type %q", resourceType)
	}

	// INSERT INTO %s (kind, config) VALUES ($1, $2) RETURNING id", table)
	query := psql.Insert(
		im.Into(table, "kind", "config"),
		im.Values(psql.Arg(resourceKind), psql.Arg(resourceConfig)),
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

// dbDeleteConfigResource deletes a resource configuration from the database.
func dbDeleteConfigResource(resourceType string, id string) error {

	table := ""
	switch resourceType {
	case "source":
		table = configSourceTableName
	case "condition":
		table = configConditionTableName
	case "target":
		table = configTargetTableName
	default:
		return fmt.Errorf("unknown resource type %q", resourceType)
	}

	// "DELETE FROM %s WHERE id = $1", table
	query := psql.Delete(
		dm.From(table),
		dm.Where(psql.Quote("id").EQ(psql.Arg(id))),
	)
	ctx := context.Background()
	queryString, args, err := query.Build(ctx)

	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return err
	}

	_, err = database.DB.Exec(context.Background(), queryString, args...)
	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return err
	}

	return nil
}

// dbGetConfigKind returns a list of resource configurations from the database filtered by kind.
func dbGetConfigKind(resourceType string) ([]string, error) {
	// SELECT kind FROM config_sources GROUP BY kind

	table := ""
	switch resourceType {
	case "source":
		table = configSourceTableName
	case "condition":
		table = configConditionTableName
	case "target":
		table = configTargetTableName
	default:
		return nil, fmt.Errorf("unknown resource type %q", resourceType)
	}

	query := psql.Select(
		sm.Columns("kind"),
		sm.From(table),
		sm.GroupBy("kind"),
	)

	ctx := context.Background()
	queryString, args, err := query.Build(ctx)

	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, err
	}

	rows, err := database.DB.Query(context.Background(), queryString, args...)
	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return nil, err
	}

	results := []string{}
	for rows.Next() {
		var kind string
		err := rows.Scan(&kind)
		if err != nil {
			logrus.Errorf("parsing config source kind result: %s", err)
			return nil, err
		}
		results = append(results, kind)
	}

	return results, nil
}

// dbGetConfigSource returns a list of resource configurations from the database.
func dbGetConfigSource(kind, id, config string) ([]model.ConfigSource, error) {

	table := configSourceTableName

	// SELECT id, kind, created_at, updated_at, config FROM " + table
	query := psql.Select(
		sm.Columns("id", "kind", "created_at", "updated_at", "config"),
		sm.From(table),
	)

	if id != "" {
		query.Apply(
			sm.Where(psql.Quote("id").EQ(psql.Arg(id))),
		)
	}

	if kind != "" {
		query.Apply(
			sm.Where(psql.Quote("kind").EQ(psql.Arg(kind))),
		)
	}

	if config != "" {
		query.Apply(
			sm.Where(psql.Raw("config @> ?", config)),
		)
	}

	ctx := context.Background()
	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, err
	}

	rows, err := database.DB.Query(context.Background(), queryString, args...)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return nil, err
	}

	results := []model.ConfigSource{}

	for rows.Next() {

		r := model.ConfigSource{}

		var config string

		err := rows.Scan(&r.ID, &r.Kind, &r.Created_at, &r.Updated_at, &config)
		if err != nil {
			logrus.Errorf("parsing Source result: %s", err)
			return nil, err
		}

		err = json.Unmarshal([]byte(config), &r.Config)
		if err != nil {
			logrus.Errorf("parsing config source result: %s\n\t%s", r.ID, err)
			continue
		}

		results = append(results, r)
	}

	return results, nil
}

// dbGetConfigCondition returns a list of resource configurations from the database.
func dbGetConfigCondition(kind, id, config string) ([]model.ConfigCondition, error) {

	table := configConditionTableName

	// SELECT id, kind, created_at, updated_at, config FROM " + table
	query := psql.Select(
		sm.Columns("id", "kind", "created_at", "updated_at", "config"),
		sm.From(table),
	)

	if id != "" {
		query.Apply(
			sm.Where(psql.Quote("id").EQ(psql.Arg(id))),
		)
	}

	if kind != "" {
		query.Apply(
			sm.Where(psql.Quote("kind").EQ(psql.Arg(kind))),
		)
	}

	if config != "" {
		query.Apply(
			sm.Where(psql.Raw("config @> ?", config)),
		)
	}

	ctx := context.Background()
	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, err
	}

	rows, err := database.DB.Query(context.Background(), queryString, args...)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", query, err)
		return nil, err
	}

	results := []model.ConfigCondition{}

	for rows.Next() {

		r := model.ConfigCondition{}

		var config string

		err := rows.Scan(&r.ID, &r.Kind, &r.Created_at, &r.Updated_at, &config)
		if err != nil {

			logrus.Errorf("Query: %q\n\t%s", queryString, err)
			logrus.Errorf("parsing  condition result: %s", err)
			return nil, err
		}

		err = json.Unmarshal([]byte(config), &r.Config)
		if err != nil {
			logrus.Errorf("parsing config condition result: %s\n\t%s", r.ID, err)
			continue
		}

		results = append(results, r)
	}

	return results, nil
}

// dbGetConfigTarget returns a list of resource configurations from the database.
func dbGetConfigTarget(kind, id, config string) ([]model.ConfigTarget, error) {

	table := configTargetTableName

	// SELECT id, kind, created_at, updated_at, config FROM " + table
	query := psql.Select(
		sm.Columns("id", "kind", "created_at", "updated_at", "config"),
		sm.From(table),
	)

	if id != "" {
		query.Apply(
			sm.Where(psql.Quote("id").EQ(psql.Arg(id))),
		)
	}

	if kind != "" {
		query.Apply(
			sm.Where(psql.Quote("kind").EQ(psql.Arg(kind))),
		)
	}

	if config != "" {
		query.Apply(
			sm.Where(psql.Raw("config @> ?", config)),
		)
	}

	ctx := context.Background()
	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, err
	}

	rows, err := database.DB.Query(context.Background(), queryString, args...)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return nil, err
	}

	results := []model.ConfigTarget{}

	for rows.Next() {

		r := model.ConfigTarget{}
		var config string

		err := rows.Scan(&r.ID, &r.Kind, &r.Created_at, &r.Updated_at, &config)
		if err != nil {
			logrus.Errorf("Query: %q\n\t%s", queryString, err)
			logrus.Errorf("parsing target result: %s", err)
			return nil, err
		}

		err = json.Unmarshal([]byte(config), &r.Config)
		if err != nil {
			logrus.Errorf("parsing config source result: %s\n\t%s", r.ID, err)
			continue
		}

		results = append(results, r)
	}

	return results, nil
}
