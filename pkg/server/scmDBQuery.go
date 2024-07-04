package server

import (
	"context"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
)

// dbGetSCMFromTarget represents a specific scm configuration from the database.
func dbGetSCMFromTarget() ([]DatabaseSCMRow, error) {

	query := `
		SELECT j.targets -> 'Scm' ->> 'URL', j.targets -> 'Scm' -> 'Branch' ->> 'Target'
		FROM (
			SELECT jsonb_path_query(data::jsonb, '$.Targets[*].*')  as targets
			FROM pipelineReports
		) j group by 1,2;`

	rows, err := database.DB.Query(context.Background(), query)
	if err != nil {
		logrus.Errorf("get scm from pipeline target(s): %q\n\t%s", query, err)
		return nil, err
	}

	results := []DatabaseSCMRow{}

	for rows.Next() {
		r := DatabaseSCMRow{}

		err = rows.Scan(&r.URL, &r.Branch)
		if err != nil {
			logrus.Errorf("scanning scm row failed: %s", err)
			continue
		}

		// No need to return empty git url
		if r.URL == "" {
			continue
		}

		results = append(results, r)
	}

	return results, nil
}

func dbInsertSCM(url, branch string) (string, error) {

	var ID uuid.UUID

	query := "INSERT INTO scms (url, branch) VALUES ($1, $2) RETURNING id"

	err := database.DB.QueryRow(context.Background(), query, url, branch).Scan(
		&ID,
	)

	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", query, err)
		return "", err
	}

	return ID.String(), nil
}

// dbGetSCM returns a list of scms from the scm database table.
func dbGetScm(id, url, branch string) ([]DatabaseSCMRow, error) {

	query := "SELECT * FROM scms"
	if id != "" || url != "" || branch != "" {
		query = "SELECT * FROM scms WHERE ("

		argCounter := 0

		if id != "" {
			switch argCounter {
			case 0:
				query = query + " id='" + id + "'"
				argCounter++
			default:
				query = query + "AND id='" + id + "'"
				argCounter++
			}
		}

		if url != "" {
			switch argCounter {
			case 0:
				query = query + " url='" + url + "'"
				argCounter++
			default:
				query = query + "AND url='" + url + "'"
				argCounter++
			}
		}

		if branch != "" {
			switch argCounter {
			case 0:
				query = query + " branch='" + branch + "'"
			default:
				query = query + "AND branch='" + branch + "'"
			}
		}

		query = query + ")"
	}

	rows, err := database.DB.Query(context.Background(), query)
	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", query, err)
		return nil, err
	}

	results := []DatabaseSCMRow{}

	for rows.Next() {
		r := DatabaseSCMRow{}

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
