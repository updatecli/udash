package database

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
	"github.com/updatecli/udash/pkg/model"
)

const (
	// configSourceTableName defines the table name for config sources
	configSourceTableName = "config_sources"
	// configConditionTableName defines the table name for config conditions
	configConditionTableName = "config_conditions"
	// configTargetTableName defines the table name for config targets
	configTargetTableName = "config_targets"

	//configSourceType defines the kind of config source
	configSourceType = "source"
	//configConditionType defines the kind of config condition
	configConditionType = "condition"
	//configTargetType defines the kind of config target
	configTargetType = "target"
)

// InsertConfigResource inserts a new resource configuration into the database.
func InsertConfigResource(ctx context.Context, resourceType, resourceKind string, resourceConfig interface{}) (string, error) {
	table := ""
	switch resourceType {
	case configSourceType:
		table = configSourceTableName
	case configConditionType:
		table = configConditionTableName
	case configTargetType:
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

	queryString, args, err := query.Build(ctx)

	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return "", err
	}

	var configID uuid.UUID
	err = DB.QueryRow(context.Background(), queryString, args...).Scan(
		&configID,
	)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return "", err
	}

	return configID.String(), nil
}

// DeleteConfigResource deletes a resource configuration from the database.
func DeleteConfigResource(ctx context.Context, resourceType string, id string) error {
	table := ""
	switch resourceType {
	case configSourceType:
		table = configSourceTableName
	case configConditionType:
		table = configConditionTableName
	case configTargetType:
		table = configTargetTableName
	default:
		return fmt.Errorf("unknown resource type %q", resourceType)
	}

	// "DELETE FROM %s WHERE id = $1", table
	query := psql.Delete(
		dm.From(table),
		dm.Where(psql.Quote("id").EQ(psql.Arg(id))),
	)
	queryString, args, err := query.Build(ctx)

	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return err
	}

	_, err = DB.Exec(ctx, queryString, args...)
	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return err
	}

	return nil
}

// GetConfigKind returns a list of resource configurations from the database filtered by kind.
func GetConfigKind(ctx context.Context, resourceType string) ([]string, error) {
	table := ""
	switch resourceType {
	case configSourceType:
		table = configSourceTableName
	case configConditionType:
		table = configConditionTableName
	case configTargetType:
		table = configTargetTableName
	default:
		return nil, fmt.Errorf("unknown resource type %q", resourceType)
	}

	// SELECT kind FROM config_sources GROUP BY kind
	query := psql.Select(
		sm.Columns("kind"),
		sm.From(table),
		sm.GroupBy("kind"),
	)

	queryString, args, err := query.Build(ctx)

	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, err
	}

	rows, err := DB.Query(context.Background(), queryString, args...)
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

// GetSourceConfigs returns a list of resource configurations from the database.
func GetSourceConfigs(ctx context.Context, kind, id, config string, limit, page int) ([]model.ConfigSource, int, error) {
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

	rows, err := DB.Query(context.Background(), queryString, args...)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return nil, 0, err
	}

	results := []model.ConfigSource{}

	for rows.Next() {
		r := model.ConfigSource{}

		var config string

		err := rows.Scan(&r.ID, &r.Kind, &r.Created_at, &r.Updated_at, &config)
		if err != nil {
			logrus.Errorf("parsing Source result: %s", err)
			return nil, 0, err
		}

		err = json.Unmarshal([]byte(config), &r.Config)
		if err != nil {
			logrus.Errorf("parsing config source result: %s\n\t%s", r.ID, err)
			continue
		}

		results = append(results, r)
	}

	return results, totalCount, nil
}

// GetConditionConfigs returns a list of resource configurations from the database.
func GetConditionConfigs(ctx context.Context, kind, id, config string, limit, page int) ([]model.ConfigCondition, int, error) {
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

	// Apply pagination if limit and page are set
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

	rows, err := DB.Query(context.Background(), queryString, args...)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return nil, 0, err
	}

	results := []model.ConfigCondition{}

	for rows.Next() {

		r := model.ConfigCondition{}

		var config string

		err := rows.Scan(&r.ID, &r.Kind, &r.Created_at, &r.Updated_at, &config)
		if err != nil {

			logrus.Errorf("Query: %q\n\t%s", queryString, err)
			logrus.Errorf("parsing  condition result: %s", err)
			return nil, 0, err
		}

		err = json.Unmarshal([]byte(config), &r.Config)
		if err != nil {
			logrus.Errorf("parsing config condition result: %s\n\t%s", r.ID, err)
			continue
		}

		results = append(results, r)
	}

	return results, totalCount, nil
}

// GetTargetConfigs returns a list of resource configurations from the database.
func GetTargetConfigs(ctx context.Context, kind, id, config string, limit, page int) ([]model.ConfigTarget, int, error) {
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

	// Apply pagination if limit and page are set
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

	rows, err := DB.Query(context.Background(), queryString, args...)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return nil, 0, err
	}

	results := []model.ConfigTarget{}

	for rows.Next() {

		r := model.ConfigTarget{}
		var config string

		err := rows.Scan(&r.ID, &r.Kind, &r.Created_at, &r.Updated_at, &config)
		if err != nil {
			logrus.Errorf("Query: %q\n\t%s", queryString, err)
			logrus.Errorf("parsing target result: %s", err)
			return nil, 0, err
		}

		err = json.Unmarshal([]byte(config), &r.Config)
		if err != nil {
			logrus.Errorf("parsing config source result: %s\n\t%s", r.ID, err)
			continue
		}

		results = append(results, r)
	}

	return results, totalCount, nil
}
