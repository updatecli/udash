package server

import (
	"context"
	"fmt"

	"encoding/json"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
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

	query := fmt.Sprintf("INSERT INTO %s (kind, config) VALUES ($1, $2) RETURNING id", table)

	err := database.DB.QueryRow(context.Background(), query, resourceKind, resourceConfig).Scan(
		&ID,
	)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", query, err)
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

	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", table)

	_, err := database.DB.Exec(context.Background(), query, id)
	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", query, err)
		return err
	}

	return nil
}

// dbGetConfigSource returns a list of resource configurations from the database.
func dbGetConfigSource(kind, id, config string) ([]model.ConfigSource, error) {

	table := configSourceTableName

	query := "SELECT id, kind, created_at, updated_at, config FROM " + table
	if id != "" || kind != "" || config != "" {
		query = query + " WHERE ("
		argCounter := 0
		if id != "" {
			switch argCounter {
			case 0:
				query = query + " id='" + id + "'"
				argCounter++
			default:
				query = query + " AND id='" + id + "'"
				argCounter++
			}
		}
		if kind != "" {
			switch argCounter {
			case 0:
				query = query + " kind='" + kind + "'"
				argCounter++
			default:
				query = query + " AND kind='" + kind + "'"
				argCounter++
			}
		}
		if config != "" {
			switch argCounter {
			case 0:
				query = query + " config @> '" + config + "'"
			default:
				query = query + " AND config @> '" + config + "'"
			}
		}
		query = query + ")"
	}

	rows, err := database.DB.Query(context.Background(), query)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", query, err)
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

	query := "SELECT id, kind, created_at, updated_at, config FROM " + table
	if id != "" || kind != "" || config != "" {
		query = query + " WHERE ("
		argCounter := 0
		if id != "" {
			switch argCounter {
			case 0:
				query = query + " id='" + id + "'"
				argCounter++
			default:
				query = query + " AND id='" + id + "'"
				argCounter++
			}
		}
		if kind != "" {
			switch argCounter {
			case 0:
				query = query + " kind='" + kind + "'"
				argCounter++
			default:
				query = query + " AND kind='" + kind + "'"
				argCounter++
			}
		}

		if config != "" {
			switch argCounter {
			case 0:
				query = query + " config @> '" + config + "'"
			default:
				query = query + " AND config @> '" + config + "'"
			}
		}
		query = query + ")"
	}

	rows, err := database.DB.Query(context.Background(), query)

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

			logrus.Errorf("Query: %q\n\t%s", query, err)
			logrus.Errorf("parsing  condition result: %s", err)
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

// dbGetConfigTarget returns a list of resource configurations from the database.
func dbGetConfigTarget(kind, id, config string) ([]model.ConfigTarget, error) {

	table := configTargetTableName

	query := "SELECT id, kind, created_at, updated_at, config FROM " + table

	if id != "" || kind != "" || config != "" {
		query = query + " WHERE ("
		argCounter := 0
		if id != "" {
			switch argCounter {
			case 0:
				query = query + " id='" + id + "'"
				argCounter++
			default:
				query = query + " AND id='" + id + "'"
				argCounter++
			}
		}
		if kind != "" {
			switch argCounter {
			case 0:
				query = query + " kind='" + kind + "'"
				argCounter++
			default:
				query = query + " AND kind='" + kind + "'"
				argCounter++
			}
		}

		if config != "" {
			switch argCounter {
			case 0:
				query = query + " config @> '" + config + "'"
			default:
				query = query + " AND config @> '" + config + "'"
			}
		}
		query = query + ")"
	}

	rows, err := database.DB.Query(context.Background(), query)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", query, err)
		return nil, err
	}

	results := []model.ConfigTarget{}

	for rows.Next() {

		r := model.ConfigTarget{}
		var config string

		err := rows.Scan(&r.ID, &r.Kind, &r.Created_at, &r.Updated_at, &config)
		if err != nil {
			logrus.Errorf("Query: %q\n\t%s", query, err)
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
