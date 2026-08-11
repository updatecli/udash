package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/test"
	"github.com/updatecli/updatecli/pkg/core/reports"
	"github.com/updatecli/updatecli/pkg/core/result"
)

func TestEndpoints(t *testing.T) {
	eng := newGinEngine(Options{})
	srv := httptest.NewServer(eng)
	defer srv.Close()

	ctx := context.Background()

	postgresContainer, err := test.SetupDatabase(t, ctx)
	require.NoError(t, err)

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Connect to the database and run migrations
	require.NoError(t, database.Connect(database.Options{URI: dbURL}))
	t.Log("Postgres Container connected")
	require.NoError(t, database.RunMigrationUp())
	t.Log("Postgres Container migrations run")

	t.Run("GET /api", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api")
		assertJSONResponse(t, resp, map[string]any{
			"message": "Welcome to the Udash API",
		}, assert.Equal)
	})

	t.Run("GET /api/ping", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api/ping")
		assertJSONResponse(t, resp, map[string]any{
			"message": "pong",
		}, assert.Equal)
	})

	t.Run("GET /api/about", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api/about")
		assertJSONResponse(t, resp, map[string]any{
			"version": map[string]any{},
		}, assert.Equal)
	})

	// TODO: Test query parameters:
	// scmid, url, branch, summary
	t.Run("GET /api/pipeline/scms", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api/pipeline/scms")
		assertJSONResponse(t, resp, map[string]any{
			"scms":        []any{},
			"total_count": float64(0),
		}, assert.Equal)

		id, err := database.InsertSCM(context.TODO(), "https://example.com/testing.git", "main")
		t.Cleanup(func() {
			deleteSCM(t, id)
		})
		require.NoError(t, err)
		resp = doGetRequest(t, srv, "/api/pipeline/scms")
		assertJSONResponse(t, resp, []map[string]any{
			{
				"Branch": "main",
				"ID":     id,
				"URL":    "https://example.com/testing.git",
			},
		}, removeFieldsAsserter("scms", "Created_at", "Updated_at"))
	})

	// Test pagination on scms
	t.Run("GET /api/pipeline/scms?limit=1", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api/pipeline/scms?limit=1")
		assertJSONResponse(t, resp, map[string]any{
			"scms":        []any{},
			"total_count": float64(0),
		}, assert.Equal)

		v1ID, v2ID := "", ""
		v1ID, err = database.InsertSCM(context.TODO(), "https://example.com/testing.git", "v1")
		t.Cleanup(func() {
			deleteSCM(t, v1ID)
		})
		v2ID, err = database.InsertSCM(context.TODO(), "https://example.com/testing.git", "v2")
		t.Cleanup(func() {
			deleteSCM(t, v2ID)
		})

		require.NoError(t, err)
		resp = doGetRequest(t, srv, "/api/pipeline/scms?limit=1")
		assertJSONResponse(t, resp, []map[string]any{
			{
				"Branch": "v1",
				"ID":     v1ID,
				"URL":    "https://example.com/testing.git",
			},
		}, removeFieldsAsserter("scms", "Created_at", "Updated_at"))
	})

	// TODO: Test query parameters:
	// scmid
	t.Run("GET /api/pipeline/reports", func(t *testing.T) {
		t.Run("with no reports", func(t *testing.T) {
			resp := doGetRequest(t, srv, "/api/pipeline/reports")
			assertJSONResponse(t, resp, map[string]any{
				"data":        []any{},
				"total_count": float64(0),
			}, assert.Equal)
		})

		t.Run("with a report", func(t *testing.T) {
			reportID, err := database.InsertReport(context.TODO(), reports.Report{
				Name:       "ci: bump Venom version",
				Result:     "✔",
				ID:         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
				PipelineID: "venom",
				Actions: map[string]*reports.Action{
					"default": {
						ID: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
					},
				},
			})
			require.NoError(t, err)

			resp := doGetRequest(t, srv, "/api/pipeline/reports")
			assertJSONResponse(t, resp, []map[string]any{
				{
					"ID":     reportID,
					"Name":   "ci: bump Venom version",
					"Result": "✔",
					"Report": map[string]any{
						"Name":       "ci: bump Venom version",
						"Err":        "",
						"Graph":      "",
						"Result":     "✔",
						"ID":         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
						"PipelineID": "venom",
						"Actions": map[string]any{
							"default": map[string]any{
								"id": "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
							},
						},
						"Sources":    nil,
						"CI":         nil,
						"Conditions": nil,
						"Targets":    nil,
						"ReportURL":  "",
					},
					"FilteredResourceID": "",
					"ConditionConfigIDs": map[string]any{},
					"SourceConfigIDs":    map[string]any{},
					"TargetConfigIDs":    map[string]any{},
				},
			}, removeFieldsAsserter("data", "CreatedAt", "UpdatedAt", "Labels"))
		})
	})

	t.Run("GET /api/pipeline/reports?limit=1", func(t *testing.T) {
		t.Run("with two reports", func(t *testing.T) {
			report2ID := ""
			_, err = database.InsertReport(context.TODO(), reports.Report{
				Name:       "ci: bump Venom version",
				Result:     "✔",
				ID:         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
				PipelineID: "venom",
				Actions: map[string]*reports.Action{
					"default": {
						ID: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
					},
				},
			})
			require.NoError(t, err)

			report2ID, err = database.InsertReport(context.TODO(), reports.Report{
				Name:       "ci: bump Venom version",
				Result:     "✔",
				ID:         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
				PipelineID: "venom",
				Actions: map[string]*reports.Action{
					"default": {
						ID: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
					},
				},
			})
			require.NoError(t, err)

			resp := doGetRequest(t, srv, "/api/pipeline/reports?limit=1")
			assertJSONResponse(t, resp, []map[string]any{
				{
					"ID":     report2ID,
					"Name":   "ci: bump Venom version",
					"Result": "✔",
					"Report": map[string]any{
						"Name":       "ci: bump Venom version",
						"Err":        "",
						"Graph":      "",
						"Result":     "✔",
						"ID":         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
						"PipelineID": "venom",
						"Actions": map[string]any{
							"default": map[string]any{
								"id": "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
							},
						},
						"Sources":    nil,
						"CI":         nil,
						"Conditions": nil,
						"Targets":    nil,
						"ReportURL":  "",
					},
					"FilteredResourceID": "",
					"ConditionConfigIDs": map[string]any{},
					"SourceConfigIDs":    map[string]any{},
					"TargetConfigIDs":    map[string]any{},
				},
			}, removeFieldsAsserter("data", "CreatedAt", "UpdatedAt", "Labels"))
		})
	})

	t.Run("GET /api/pipeline/reports/:id", func(t *testing.T) {
		t.Run("with an unknown report ID", func(t *testing.T) {
			resp := doGetRequest(t, srv, "/api/pipeline/reports/daa9b61e-42b9-4e35-b9d7-071461a36838")
			assertErrorResponse(t, resp, http.StatusNotFound, pgx.ErrNoRows.Error())
		})

		t.Run("with a known report ID", func(t *testing.T) {
			reportID, err := database.InsertReport(context.TODO(), reports.Report{
				Name:       "ci: bump Venom version",
				Result:     "✔",
				ID:         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
				PipelineID: "venom",
				Actions: map[string]*reports.Action{
					"default": {
						ID: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
					},
				},
			})
			require.NoError(t, err)

			resp := doGetRequest(t, srv, "/api/pipeline/reports/"+reportID)
			assertJSONResponse(t, resp, map[string]any{
				"ID": reportID,
				"Pipeline": map[string]any{
					"Name": "ci: bump Venom version", "Err": "", "Graph": "", "Result": "✔",
					"ID":         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
					"PipelineID": "venom",
					"Actions": map[string]any{
						"default": map[string]any{
							"id": "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
						},
					},
					"Sources":    nil,
					"CI":         nil,
					"Conditions": nil,
					"Targets":    nil,
					"ReportURL":  "",
				},
			}, removeFieldsAsserter("data", "Created_at", "Updated_at", "Labels"))
		})
	})

	t.Run("GET /api/pipeline/config/kinds", func(t *testing.T) {
		t.Run("with no type", func(t *testing.T) {
			resp := doGetRequest(t, srv, "/api/pipeline/config/kinds")

			assertErrorResponse(t, resp, http.StatusBadRequest, "no type provided")
		})

		t.Run("with unknown type", func(t *testing.T) {
			resp := doGetRequest(t, srv, "/api/pipeline/config/kinds?type=test")

			assertErrorResponse(t, resp, http.StatusBadRequest, `unknown resource type "test"`)
		})

		t.Run("with no entries for a type", func(t *testing.T) {
			resp := doGetRequest(t, srv, "/api/pipeline/config/kinds?type=source")

			assertJSONResponse(t, resp, map[string]any{
				"data": []any{},
			}, assert.Equal)
		})

		t.Run("with config entries for a type", func(t *testing.T) {
			ctx := context.TODO()
			id1, err := database.InsertConfigResource(ctx, "source", "testing-1", map[string]any{"testing": "value"})
			require.NoError(t, err)
			t.Cleanup(func() {
				assert.NoError(t, database.DeleteConfigResource(ctx, "source", id1))
			})
			id2, err := database.InsertConfigResource(ctx, "source", "testing-2", map[string]any{"testing": "value"})
			t.Cleanup(func() {
				assert.NoError(t, database.DeleteConfigResource(ctx, "source", id2))
			})
			require.NoError(t, err)

			resp := doGetRequest(t, srv, "/api/pipeline/config/kinds?type=source")

			assertJSONResponse(t, resp, map[string]any{
				"data": []any{
					"testing-2",
					"testing-1",
				},
			}, assert.Equal)
		})
	})

	t.Run("GET /api/pipeline/config/sources", func(t *testing.T) {
		t.Run("with no data", func(t *testing.T) {
			resp := doGetRequest(t, srv, "/api/pipeline/config/sources")

			assertJSONResponse(t, resp, map[string]any{
				"configs":     []any{},
				"total_count": float64(0),
			}, assert.Equal)
		})

		t.Run("with config sources", func(t *testing.T) {
			config1, err := database.InsertConfigResource(ctx, "source", "testing-1", map[string]any{"testing": "value"})
			require.NoError(t, err)
			t.Cleanup(func() {
				assert.NoError(t, database.DeleteConfigResource(ctx, "source", config1))
			})

			config2, err := database.InsertConfigResource(ctx, "source", "testing-2", map[string]any{"testing": "value"})
			require.NoError(t, err)
			t.Cleanup(func() {
				assert.NoError(t, database.DeleteConfigResource(ctx, "source", config2))
			})

			t.Run("with no parameters", func(t *testing.T) {
				resp := doGetRequest(t, srv, "/api/pipeline/config/sources")

				assertJSONResponse(t, resp, []map[string]any{
					{
						"Config": map[string]any{
							"DependsOn":           nil,
							"DeprecatedDependsOn": nil,
							"DeprecatedSCMID":     "",
							"Kind":                "",
							"Name":                "",
							"SCMID":               "",
							"Spec":                nil,
							"Transformers":        nil,
						},
						"ID":   config2,
						"Kind": "testing-2",
					},
					{
						"Config": map[string]any{
							"DependsOn":           nil,
							"DeprecatedDependsOn": nil,
							"DeprecatedSCMID":     "",
							"Kind":                "",
							"Name":                "",
							"SCMID":               "",
							"Spec":                nil,
							"Transformers":        nil,
						},
						"ID":   config1,
						"Kind": "testing-1",
					},
				}, removeFieldsAsserter("configs", "Created_at", "Updated_at"))
			})

			t.Run("with no sources matching kind", func(t *testing.T) {
				resp := doGetRequest(t, srv, "/api/pipeline/config/sources?kind=test")

				assertJSONResponse(t, resp, map[string]any{
					"configs":     []any{},
					"total_count": float64(0),
				}, assert.Equal)
			})
			t.Run("with sources matching kind", func(t *testing.T) {
				resp := doGetRequest(t, srv, "/api/pipeline/config/sources?kind=testing-1")

				assertJSONResponse(t, resp, []map[string]any{
					{
						"Config": map[string]any{
							"DependsOn":           nil,
							"DeprecatedDependsOn": nil,
							"DeprecatedSCMID":     "",
							"Kind":                "",
							"Name":                "",
							"SCMID":               "",
							"Spec":                nil,
							"Transformers":        nil,
						},
						"ID":   config1,
						"Kind": "testing-1",
					},
				}, removeFieldsAsserter("configs", "Created_at", "Updated_at"))
			})
		})
	})

	t.Run("GET /api/pipeline/labels", func(t *testing.T) {
		t.Run("with no labels", func(t *testing.T) {
			resp := doGetRequest(t, srv, "/api/pipeline/labels")
			assertJSONResponse(t, resp, map[string]any{
				"labels":      []any{},
				"total_count": float64(0),
			}, assert.Equal)
		})

		t.Run("with labels", func(t *testing.T) {
			labelIDs, err := database.InitLabels(ctx, map[string]string{
				"env": "production",
			})
			require.NoError(t, err)
			require.Len(t, labelIDs, 1)

			labelID := labelIDs[0]

			t.Cleanup(func() {
				deleteLabel(t, labelID)
			})

			resp := doGetRequest(t, srv, "/api/pipeline/labels")
			assertJSONResponse(t, resp, []map[string]any{
				{
					"id":    labelID.String(),
					"key":   "env",
					"value": "production",
				},
			}, removeFieldsAsserter("labels", "created_at", "updated_at", "last_pipeline_report_at"))

			resp = doGetRequest(t, srv, "/api/pipeline/labels?keyonly=true")
			assertJSONResponse(t, resp, map[string]any{
				"labels": []any{
					"env",
				},
				"total_count": float64(1),
			}, assert.Equal)
		})
	})

	t.Run("POST /api/pipeline/labels/search", func(t *testing.T) {
		labelIDs, err := database.InitLabels(ctx, map[string]string{
			"env":  "production",
			"tier": "backend",
		})
		require.NoError(t, err)
		require.Len(t, labelIDs, 2)

		envLabelRecords, totalCount, err := database.GetLabelRecords(ctx, "", "env", "production", "", "", 0, 1)
		require.NoError(t, err)
		require.Equal(t, 1, totalCount)
		envLabelID := envLabelRecords[0].ID.String()

		for i := range labelIDs {
			id := labelIDs[i]
			t.Cleanup(func() {
				deleteLabel(t, id)
			})
		}

		resp := doPostRequest(t, srv, "/api/pipeline/labels/search", map[string]any{
			"key":   "env",
			"value": "production",
		})

		assertJSONResponse(t, resp, []map[string]any{
			{
				"id":    envLabelID,
				"key":   "env",
				"value": "production",
			},
		}, removeFieldsAsserter("labels", "created_at", "updated_at", "last_pipeline_report_at"))
	})

	// This subtest must run last as it removes every pipeline report.
	t.Run("POST /api/pipeline/reports/summary", func(t *testing.T) {
		const summaryPath = "/api/pipeline/reports/summary"
		// Every bucket identifies itself by its start, formatted as RFC3339, whatever
		// the granularity is.
		const bucketLayout = time.RFC3339

		// The previous subtests leave reports behind which would all be
		// counted in today's bucket.
		truncateReports(t)
		t.Cleanup(func() {
			truncateReports(t)
		})

		// The expected days are derived from the same clock as the request, so
		// a request crossing midnight would make this subtest flaky. Seeding at
		// the current time of day keeps that window as small as possible.
		now := time.Now().UTC()

		seedReport := func(pipelineResult string, dayOffset int) string {
			t.Helper()

			id, err := database.InsertReport(ctx, reports.Report{
				Name:       "ci: bump Venom version",
				Result:     pipelineResult,
				ID:         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
				PipelineID: "venom",
			})
			require.NoError(t, err)

			setReportTimestamp(t, id, now.AddDate(0, 0, dayOffset))
			return id
		}

		day := func(dayOffset int) string {
			return dayStart(now.AddDate(0, 0, dayOffset)).Format(bucketLayout)
		}

		// bucketEntry builds the expected response entry for a bucket, starting from
		// a zeroed set of results. None of the reports seeded here carries an action, so
		// the open action breakdown is always zeroed; it is covered on its own below.
		bucketEntry := func(date string, results map[string]any) map[string]any {
			zeroedResults := func() map[string]any {
				return map[string]any{
					"✔":       float64(0),
					"✗":       float64(0),
					"⚠":       float64(0),
					"-":       float64(0),
					"unknown": float64(0),
				}
			}

			allResults := zeroedResults()
			total := float64(0)
			for k, v := range results {
				allResults[k] = v
				total += v.(float64)
			}

			return map[string]any{
				"date":         date,
				"results":      allResults,
				"open_actions": zeroedResults(),
				"total":        total,
			}
		}

		entry := func(dayOffset int, results map[string]any) map[string]any {
			return bucketEntry(day(dayOffset), results)
		}

		// want builds the expected response body, the metric and the granularity being
		// echoed back by the endpoint.
		want := func(granularity string, totalCount float64, entries ...any) map[string]any {
			return map[string]any{
				"metric":      "result",
				"granularity": granularity,
				"data":        entries,
				"total_count": totalCount,
			}
		}

		seeds := []struct {
			result string
			offset int
		}{
			{"✔", 0},
			{"✔", 0},
			{"✗", 0},
			{"✔", -3},
			// Outside of the default seven days window.
			{"⚠", -9},
		}

		seededIDs := make([]string, 0, len(seeds))
		for _, seed := range seeds {
			seededIDs = append(seededIDs, seedReport(seed.result, seed.offset))
		}
		scmReportID := seededIDs[0]

		// bucketedEntries builds the expected entries of a window of days, bucketing the
		// seeded reports the same way the endpoint does. Expressing the expectation this
		// way keeps the week and month cases independent from the day this test runs on.
		bucketedEntries := func(days int, truncate, next func(time.Time) time.Time) []any {
			counts := map[string]map[string]any{}
			for _, seed := range seeds {
				date := truncate(now.AddDate(0, 0, seed.offset)).Format(bucketLayout)
				if counts[date] == nil {
					counts[date] = map[string]any{}
				}

				previous, _ := counts[date][seed.result].(float64)
				counts[date][seed.result] = previous + 1
			}

			entries := []any{}
			last := truncate(now)
			for bucket := truncate(now.AddDate(0, 0, -(days - 1))); !bucket.After(last); bucket = next(bucket) {
				date := bucket.Format(bucketLayout)
				entries = append(entries, bucketEntry(date, counts[date]))
			}

			return entries
		}

		t.Run("with the default window", func(t *testing.T) {
			resp := doPostRequest(t, srv, summaryPath, map[string]any{})

			assertJSONResponse(t, resp, want("day", 4,
				entry(-6, nil),
				entry(-5, nil),
				entry(-4, nil),
				entry(-3, map[string]any{"✔": float64(1)}),
				entry(-2, nil),
				entry(-1, nil),
				entry(0, map[string]any{"✔": float64(2), "✗": float64(1)}),
			), assert.Equal)
		})

		t.Run("with an explicit number of days", func(t *testing.T) {
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"days": 3,
			})

			assertJSONResponse(t, resp, want("day", 3,
				entry(-2, nil),
				entry(-1, nil),
				entry(0, map[string]any{"✔": float64(2), "✗": float64(1)}),
			), assert.Equal)
		})

		t.Run("with a window wide enough to catch every report", func(t *testing.T) {
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"days": 10,
			})

			blob := map[string]any{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&blob))
			defer resp.Body.Close()

			assert.Len(t, blob["data"], 10)
			assert.Equal(t, float64(5), blob["total_count"])
			assert.Equal(t, entry(-9, map[string]any{"⚠": float64(1)}), blob["data"].([]any)[0])
		})

		t.Run("with a week granularity", func(t *testing.T) {
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"days":        10,
				"granularity": "week",
			})

			assertJSONResponse(t, resp, want("week", 5,
				bucketedEntries(10, weekStart, func(t time.Time) time.Time {
					return t.AddDate(0, 0, 7)
				})...,
			), assert.Equal)
		})

		t.Run("with a month granularity", func(t *testing.T) {
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"days":        10,
				"granularity": "month",
			})

			assertJSONResponse(t, resp, want("month", 5,
				bucketedEntries(10, monthStart, func(t time.Time) time.Time {
					return t.AddDate(0, 1, 0)
				})...,
			), assert.Equal)
		})

		t.Run("filtered by scm", func(t *testing.T) {
			scmID, err := database.InsertSCM(ctx, "https://example.com/summary.git", "main")
			require.NoError(t, err)
			t.Cleanup(func() {
				deleteSCM(t, scmID)
			})

			attachReportToSCM(t, scmReportID, scmID)

			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"days":  1,
				"scmid": scmID,
			})

			assertJSONResponse(t, resp, want("day", 1,
				entry(0, map[string]any{"✔": float64(1)}),
			), assert.Equal)

			t.Run("without any scm", func(t *testing.T) {
				resp := doPostRequest(t, srv, summaryPath, map[string]any{
					"days":  1,
					"scmid": "none",
				})

				assertJSONResponse(t, resp, want("day", 2,
					entry(0, map[string]any{"✔": float64(1), "✗": float64(1)}),
				), assert.Equal)
			})
		})

		t.Run("with an invalid number of days", func(t *testing.T) {
			for _, days := range []int{-1, maxMonitoringDurationDays + 1} {
				resp := doPostRequest(t, srv, summaryPath, map[string]any{
					"days": days,
				})

				assertErrorResponse(t, resp, http.StatusBadRequest, ErrInvalidDaysParam)
			}
		})

		t.Run("with an incomplete time range", func(t *testing.T) {
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"start_time": now.Format(timeRangeLayout),
			})

			assertErrorResponse(t, resp, http.StatusBadRequest, ErrInvalidTimeRangeParams)
		})

		t.Run("with an unsupported metric", func(t *testing.T) {
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"metric": "duration",
			})

			assertErrorResponse(t, resp, http.StatusBadRequest, ErrInvalidMetricParam)
		})

		t.Run("with an unsupported granularity", func(t *testing.T) {
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"granularity": "minute",
			})

			assertErrorResponse(t, resp, http.StatusBadRequest, ErrInvalidGranularityParam)
		})

		t.Run("with both days and hours", func(t *testing.T) {
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"days":  1,
				"hours": 1,
			})

			assertErrorResponse(t, resp, http.StatusBadRequest, ErrConflictingWindowParams)
		})

		t.Run("with an invalid number of hours", func(t *testing.T) {
			for _, hours := range []int{-1, maxMonitoringDurationDays*24 + 1} {
				resp := doPostRequest(t, srv, summaryPath, map[string]any{
					"hours": hours,
				})

				assertErrorResponse(t, resp, http.StatusBadRequest, ErrInvalidHoursParam)
			}
		})

		t.Run("with a granularity producing too many buckets", func(t *testing.T) {
			// The days limit bounds how much of the table is scanned, not how large the
			// response gets: a year of hourly buckets is a cheap scan but thousands of
			// entries, so it has to be rejected by the bucket limit instead.
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"granularity": "hour",
				"days":        maxMonitoringDurationDays,
			})

			assertErrorResponse(t, resp, http.StatusBadRequest, ErrTooManyBuckets)
		})

		t.Run("with a time range wider than the limit", func(t *testing.T) {
			// The days validation does not cover an explicit time range, so this is
			// the only guard against summarizing the whole table.
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"start_time": now.AddDate(0, 0, -(maxMonitoringDurationDays + 1)).Format(timeRangeLayout),
				"end_time":   now.Format(timeRangeLayout),
			})

			assertErrorResponse(t, resp, http.StatusBadRequest, ErrTimeRangeTooWide)
		})

		// The remaining subtests seed reports of their own, so they must run after the
		// ones asserting on the counts above.
		t.Run("with a report without any result", func(t *testing.T) {
			id := seedReport("", 0)
			t.Cleanup(func() {
				deleteReport(t, id)
			})

			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"days": 1,
			})

			assertJSONResponse(t, resp, want("day", 4,
				entry(0, map[string]any{"✔": float64(2), "✗": float64(1), "unknown": float64(1)}),
			), assert.Equal)
		})

		// This subtest replaces the seeded dataset, so it must run after every other one.
		t.Run("with an hour granularity", func(t *testing.T) {
			truncateReports(t)

			currentHour := hourStart(now)

			seedReportAt := func(pipelineResult string, at time.Time) {
				t.Helper()

				id, err := database.InsertReport(ctx, reports.Report{
					Name:       "ci: bump Venom version",
					Result:     pipelineResult,
					ID:         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
					PipelineID: "venom",
				})
				require.NoError(t, err)

				setReportTimestamp(t, id, at)
			}

			// Halfway into each hour, so that a report cannot land in a neighboring
			// bucket.
			halfPast := 30 * time.Minute
			seedReportAt("✔", currentHour.Add(halfPast))
			seedReportAt("✔", currentHour.Add(-1*time.Hour+halfPast))
			seedReportAt("✗", currentHour.Add(-1*time.Hour+halfPast))
			seedReportAt("⚠", currentHour.Add(-3*time.Hour+halfPast))

			hour := func(hourOffset int) string {
				return currentHour.Add(time.Duration(hourOffset) * time.Hour).Format(bucketLayout)
			}

			// Driving this with an explicit time range rather than the hours window keeps
			// the expected buckets independent from the clock, so a request crossing an
			// hour boundary cannot shift them.
			resp := doPostRequest(t, srv, summaryPath, map[string]any{
				"granularity": "hour",
				"start_time":  currentHour.Add(-3 * time.Hour).Format(timeRangeLayout),
				"end_time":    currentHour.Format(timeRangeLayout),
			})

			assertJSONResponse(t, resp, want("hour", 4,
				bucketEntry(hour(-3), map[string]any{"⚠": float64(1)}),
				bucketEntry(hour(-2), nil),
				bucketEntry(hour(-1), map[string]any{"✔": float64(1), "✗": float64(1)}),
				bucketEntry(hour(0), map[string]any{"✔": float64(1)}),
			), assert.Equal)

			t.Run("with a relative hours window", func(t *testing.T) {
				// hours is resolved against the server clock, so a request crossing an
				// hour boundary shifts the whole window by one bucket. The window is wide
				// enough for every seeded report to stay inside it either way, and only
				// the shape of the response is asserted.
				resp := doPostRequest(t, srv, summaryPath, map[string]any{
					"granularity": "hour",
					"hours":       6,
				})

				blob := map[string]any{}
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&blob))
				defer resp.Body.Close()

				assert.Equal(t, "hour", blob["granularity"])
				assert.Len(t, blob["data"], 6)
				assert.Equal(t, float64(4), blob["total_count"])
			})
		})
	})

	t.Run("filtering on an action left open", func(t *testing.T) {
		// Updatecli reports a pipeline which had nothing to change as a success even when
		// the change it would have made is already waiting in an open pull request. That
		// is the state which needs a human, yet the result alone cannot express it: the
		// only thing telling it apart from a genuinely up to date pipeline is the action
		// link the report carries.
		truncateReports(t)
		t.Cleanup(func() {
			truncateReports(t)
		})

		seed := func(name, pipelineResult, actionURL string) string {
			t.Helper()

			id, err := database.InsertReport(ctx, reports.Report{
				Name:       name,
				Result:     pipelineResult,
				ID:         name,
				PipelineID: "venom",
				Actions: map[string]*reports.Action{
					"default": {ID: "default", Link: actionURL},
				},
			})
			require.NoError(t, err)

			return id
		}

		successWithOpenPR := seed("succeeded, pull request still open", "✔",
			"https://example.com/testing/pull/42")
		seed("succeeded, nothing to follow up", "✔", "")
		attentionWithOpenPR := seed("changed something and opened a pull request", "⚠",
			"https://example.com/testing/pull/43")

		// reportNames returns the name of every report of a search response, which
		// identifies the seeded reports more readably than their database id.
		reportNames := func(resp *http.Response) []string {
			t.Helper()

			blob := struct {
				Data []struct {
					Name string
				}
				TotalCount int `json:"total_count"`
			}{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&blob))
			defer resp.Body.Close()

			names := make([]string, 0, len(blob.Data))
			for _, report := range blob.Data {
				names = append(names, report.Name)
			}

			// A filter dropping reports from the page while still counting them in the
			// total breaks pagination, so the two are checked against each other.
			assert.Equal(t, len(names), blob.TotalCount)
			sort.Strings(names)

			return names
		}

		t.Run("POST /api/pipeline/reports/search", func(t *testing.T) {
			t.Run("without the filter", func(t *testing.T) {
				resp := doPostRequest(t, srv, "/api/pipeline/reports/search", map[string]any{})

				assert.Equal(t, []string{
					"changed something and opened a pull request",
					"succeeded, nothing to follow up",
					"succeeded, pull request still open",
				}, reportNames(resp))
			})

			t.Run("with an open action", func(t *testing.T) {
				resp := doPostRequest(t, srv, "/api/pipeline/reports/search", map[string]any{
					"open_action": true,
				})

				assert.Equal(t, []string{
					"changed something and opened a pull request",
					"succeeded, pull request still open",
				}, reportNames(resp))
			})

			t.Run("without any open action", func(t *testing.T) {
				resp := doPostRequest(t, srv, "/api/pipeline/reports/search", map[string]any{
					"open_action": false,
				})

				assert.Equal(t, []string{"succeeded, nothing to follow up"}, reportNames(resp))
			})

			t.Run("combined with a result", func(t *testing.T) {
				// This is the combination the whole dimension exists for: the pipelines
				// which succeeded only because their change is already waiting in a pull
				// request nobody merged.
				resp := doPostRequest(t, srv, "/api/pipeline/reports/search", map[string]any{
					"results":     []string{"✔"},
					"open_action": true,
				})

				assert.Equal(t, []string{"succeeded, pull request still open"}, reportNames(resp))
			})
		})

		t.Run("POST /api/pipeline/reports/summary", func(t *testing.T) {
			summaryOf := func(body map[string]any) (results, openActions map[string]any, totalCount float64) {
				t.Helper()

				blob := map[string]any{}
				resp := doPostRequest(t, srv, "/api/pipeline/reports/summary", body)
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&blob))
				defer resp.Body.Close()

				data := blob["data"].([]any)
				today := data[len(data)-1].(map[string]any)

				return today["results"].(map[string]any),
					today["open_actions"].(map[string]any),
					blob["total_count"].(float64)
			}

			t.Run("reports the open actions as a breakdown of the results", func(t *testing.T) {
				// Nothing is filtered out here: the breakdown is what lets a dashboard
				// split the success bucket without having to run a second query.
				results, openActions, totalCount := summaryOf(map[string]any{"days": 1})

				assert.Equal(t, float64(3), totalCount)
				assert.Equal(t, float64(2), results["✔"])
				assert.Equal(t, float64(1), results["⚠"])
				assert.Equal(t, float64(1), openActions["✔"])
				assert.Equal(t, float64(1), openActions["⚠"])
				assert.Equal(t, float64(0), openActions["✗"])
			})

			t.Run("filtered on an open action", func(t *testing.T) {
				results, openActions, totalCount := summaryOf(map[string]any{
					"days":        1,
					"open_action": true,
				})

				assert.Equal(t, float64(2), totalCount)
				assert.Equal(t, float64(1), results["✔"])
				assert.Equal(t, float64(1), openActions["✔"])
			})

			t.Run("filtered on the absence of an open action", func(t *testing.T) {
				results, openActions, totalCount := summaryOf(map[string]any{
					"days":        1,
					"open_action": false,
				})

				assert.Equal(t, float64(1), totalCount)
				assert.Equal(t, float64(1), results["✔"])
				assert.Equal(t, float64(0), openActions["✔"])
			})
		})

		t.Run("POST /api/pipeline/scms/search", func(t *testing.T) {
			scmID, err := database.InsertSCM(ctx, "https://example.com/openaction.git", "main")
			require.NoError(t, err)
			t.Cleanup(func() {
				deleteSCM(t, scmID)
			})

			attachReportToSCM(t, successWithOpenPR, scmID)
			attachReportToSCM(t, attentionWithOpenPR, scmID)

			branchOf := func(body map[string]any) map[string]any {
				t.Helper()

				blob := map[string]any{}
				resp := doPostRequest(t, srv, "/api/pipeline/scms/search", body)
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&blob))
				defer resp.Body.Close()

				data := blob["data"].(map[string]any)
				repository := data["https://example.com/openaction.git"].(map[string]any)

				return repository["main"].(map[string]any)
			}

			t.Run("breaks the open actions down per result", func(t *testing.T) {
				branch := branchOf(map[string]any{"summary": true, "scmid": scmID})

				assert.Equal(t, map[string]any{"✔": float64(1), "⚠": float64(1)},
					branch["total_result_by_type"])
				assert.Equal(t, map[string]any{"✔": float64(1), "⚠": float64(1)},
					branch["total_open_action_by_result"])
				// Two pipelines, but each on a pull request of its own.
				assert.Equal(t, float64(2), branch["total_action_urls"])
			})

			t.Run("filtered on a result and an open action", func(t *testing.T) {
				branch := branchOf(map[string]any{
					"summary":     true,
					"scmid":       scmID,
					"results":     []string{"✔"},
					"open_action": true,
				})

				assert.Equal(t, map[string]any{"✔": float64(1)}, branch["total_result_by_type"])
				assert.Equal(t, map[string]any{"✔": float64(1)}, branch["total_open_action_by_result"])
			})

			t.Run("filtered on the absence of an open action", func(t *testing.T) {
				// Both reports attached to this scm carry one, so the summary keeps the
				// branch but empties its counts.
				branch := branchOf(map[string]any{
					"summary":     true,
					"scmid":       scmID,
					"open_action": false,
				})

				assert.Equal(t, map[string]any{}, branch["total_result_by_type"])
				assert.Equal(t, map[string]any{}, branch["total_open_action_by_result"])
			})
		})
	})

	t.Run("pagination", func(t *testing.T) {
		truncateReports(t)

		for range 3 {
			id, err := database.InsertReport(ctx, reports.Report{
				Name: "paginated", Result: "✔", ID: "paginated", PipelineID: "paginated",
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				deleteReport(t, id)
			})
		}

		reportsOf := func(t *testing.T, body map[string]any) []any {
			t.Helper()

			resp := doPostRequest(t, srv, "/api/pipeline/reports/search", body)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			blob := map[string]any{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&blob))
			require.Equal(t, float64(3), blob["total_count"])

			return blob["data"].([]any)
		}

		t.Run("a limit without a page returns the first page", func(t *testing.T) {
			// Pages are one based, so an unset page used to build "OFFSET -1". Postgres
			// rejects it, and because that error only surfaces when the rows are read it
			// was reported as an empty, successful result.
			assert.Len(t, reportsOf(t, map[string]any{"limit": 1}), 1)
		})

		t.Run("an explicit page is honored", func(t *testing.T) {
			assert.Len(t, reportsOf(t, map[string]any{"limit": 2, "page": 1}), 2)
			assert.Len(t, reportsOf(t, map[string]any{"limit": 2, "page": 2}), 1)
		})

		t.Run("a page past the end returns nothing", func(t *testing.T) {
			// The limit used to be ignored whenever it reached the total count, which
			// answered any page with the whole dataset.
			assert.Empty(t, reportsOf(t, map[string]any{"limit": 3, "page": 2}))
		})

		t.Run("an invalid limit is reported once", func(t *testing.T) {
			// The helper reporting the mistake used to answer the request itself and
			// still hand the caller a nil error, appending a second body to the 400.
			for _, query := range []string{"limit=abc", "limit=2000", "page=xyz"} {
				resp := doGetRequest(t, srv, "/api/pipeline/reports?"+query)
				defer resp.Body.Close()

				require.Equal(t, http.StatusBadRequest, resp.StatusCode, query)

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				trailing := json.NewDecoder(bytes.NewReader(body))
				blob := map[string]any{}
				require.NoError(t, trailing.Decode(&blob), query)
				assert.Contains(t, blob[errMessageType], ErrInvalidPaginationParams)
				assert.False(t, trailing.More(), "%s: more than one body was written: %s", query, body)
			}
		})
	})

	t.Run("POST /api/pipeline/reports/search combining resource filters", func(t *testing.T) {
		truncateReports(t)

		reportID, err := database.InsertReport(ctx, reports.Report{
			Name: "combined", Result: "✔", ID: "combined", PipelineID: "combined",
			Sources: map[string]*result.Source{
				"src": {Config: map[string]any{"Kind": "shell", "Spec": map[string]any{"command": "echo"}}},
			},
			Targets: map[string]*result.Target{
				"tgt": {Config: map[string]any{"Kind": "file", "Spec": map[string]any{"file": "combined.txt"}}},
			},
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			deleteReport(t, reportID)
		})

		sourceIDs, targetIDs := pgtype.Hstore{}, pgtype.Hstore{}
		require.NoError(t, database.DB.QueryRow(ctx,
			"SELECT config_source_ids, config_target_ids FROM pipelineReports WHERE id = $1", reportID,
		).Scan(&sourceIDs, &targetIDs))

		firstKeyOf := func(h pgtype.Hstore) string {
			for key := range h {
				return key
			}
			return ""
		}

		sourceID, targetID := firstKeyOf(sourceIDs), firstKeyOf(targetIDs)
		require.NotEmpty(t, sourceID)
		require.NotEmpty(t, targetID)

		// Each filter adds a column to the select, so combining two of them used to
		// build a query returning more columns than the scan expected.
		resp := doPostRequest(t, srv, "/api/pipeline/reports/search", map[string]any{
			"sourceid": sourceID,
			"targetid": targetID,
		})
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		blob := map[string]any{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&blob))

		data := blob["data"].([]any)
		require.Len(t, data, 1)
		// The last applied filter names the matched resource, as it did when a single
		// one could be applied.
		assert.Equal(t, "tgt", data[0].(map[string]any)["FilteredResourceID"])
	})

	t.Run("GET /api/pipeline/reports with an invalid latest", func(t *testing.T) {
		// An unparsable value used to be warned about and then read as false, which is
		// the opposite of the documented default.
		resp := doGetRequest(t, srv, "/api/pipeline/reports?latest=notabool")
		assertErrorResponse(t, resp, http.StatusBadRequest, ErrInvalidLatestParam)
	})

	t.Run("GET /api/pipeline/scms with a time range", func(t *testing.T) {
		truncateReports(t)

		scmID, err := database.InsertSCM(ctx, "https://example.com/timerange.git", "main")
		require.NoError(t, err)
		t.Cleanup(func() {
			deleteSCM(t, scmID)
		})

		reportID, err := database.InsertReport(ctx, reports.Report{
			Name: "timerange", Result: "✔", ID: "timerange", PipelineID: "timerange",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			deleteReport(t, reportID)
		})

		// The scms are dated by the reports published for them, which the trigger of
		// migration 000009 records on insert.
		_, err = database.DB.Exec(ctx,
			"UPDATE scms SET last_pipeline_report_at = $1 WHERE id = $2",
			time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC), scmID)
		require.NoError(t, err)

		scmsIn := func(t *testing.T, start, end time.Time) []any {
			t.Helper()

			resp := doGetRequest(t, srv, fmt.Sprintf("/api/pipeline/scms?start_time=%s&end_time=%s",
				url.QueryEscape(start.Format(timeRangeLayout)),
				url.QueryEscape(end.Format(timeRangeLayout))))
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			blob := map[string]any{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&blob))

			return blob["scms"].([]any)
		}

		t.Run("keeps the scms reported within the range", func(t *testing.T) {
			assert.Len(t, scmsIn(t,
				time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)), 1)
		})

		t.Run("drops the scms reported outside of it", func(t *testing.T) {
			// The range used to be accepted and then ignored, returning every scm.
			assert.Empty(t, scmsIn(t,
				time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))
		})

		t.Run("rejects a half open range", func(t *testing.T) {
			resp := doGetRequest(t, srv, "/api/pipeline/scms?start_time=2026-03-01+00:00:00Z")
			assertErrorResponse(t, resp, http.StatusBadRequest, ErrInvalidTimeRangeParams)
		})
	})
}

// hourStart returns the beginning of the UTC hour of the provided time.
func hourStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
}

// dayStart returns the midnight of the UTC day of the provided time.
func dayStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// weekStart returns the monday of the UTC week of the provided time, matching how
// Postgres truncates a timestamp to a week.
func weekStart(t time.Time) time.Time {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

// monthStart returns the first day of the UTC month of the provided time.
func monthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// timeRangeLayout is the layout the start_time and end_time filters are expected in.
const timeRangeLayout = "2006-01-02 15:04:05Z07:00"

// truncateReports removes every pipeline report from the database.
func truncateReports(t *testing.T) {
	t.Helper()

	_, err := database.DB.Exec(context.TODO(), "DELETE FROM pipelineReports")
	require.NoError(t, err)
}

// deleteReport removes a single pipeline report from the database.
func deleteReport(t *testing.T, id string) {
	t.Helper()

	_, err := database.DB.Exec(context.TODO(), "DELETE FROM pipelineReports WHERE id = $1", id)
	require.NoError(t, err)
}

// setReportTimestamp forces the creation and update date of an existing report.
// InsertReport always relies on the database defaults, so backdating a report
// requires updating it afterwards.
func setReportTimestamp(t *testing.T, id string, at time.Time) {
	t.Helper()

	// The value must be normalized to UTC: the driver sends the wall clock of
	// its own location, which is what the timestamp column stores.
	_, err := database.DB.Exec(context.TODO(),
		"UPDATE pipelineReports SET created_at = $1, updated_at = $1 WHERE id = $2",
		at.UTC(), id)
	require.NoError(t, err)
}

// attachReportToSCM associates an existing report to an scm.
func attachReportToSCM(t *testing.T, reportID, scmID string) {
	t.Helper()

	_, err := database.DB.Exec(context.TODO(),
		"UPDATE pipelineReports SET target_db_scm_ids = ARRAY[$1]::uuid[] WHERE id = $2",
		scmID, reportID)
	require.NoError(t, err)
}

func doGetRequest(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()

	r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", ts.URL, path), nil)
	require.NoError(t, err)

	resp, err := ts.Client().Do(r)
	require.NoError(t, err)

	return resp
}

func doPostRequest(t *testing.T, ts *httptest.Server, path string, body any) *http.Response {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)

	r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s%s", ts.URL, path), bytes.NewReader(payload))
	require.NoError(t, err)
	r.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(r)
	require.NoError(t, err)

	return resp
}

type assertionFunc func(t assert.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool

func assertJSONResponse(t *testing.T, res *http.Response, want any, asserter assertionFunc) {
	t.Helper()

	assertJSONResponseWithCode(t, res, http.StatusOK, want, asserter)
}

func assertErrorResponse(t *testing.T, res *http.Response, code int, wantMsg string) {
	t.Helper()

	assertJSONResponseWithCode(t, res, code, map[string]any{errMessageType: wantMsg}, assert.Equal)
}

func assertJSONResponseWithCode(t *testing.T, res *http.Response, code int, want any, asserter assertionFunc) {
	t.Helper()
	require.Equal(t, code, res.StatusCode)
	assert.Equal(t, "application/json; charset=utf-8", res.Header.Get("Content-Type"))

	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	got := map[string]any{}
	require.NoError(t, json.Unmarshal(b, &got))

	asserter(t, want, got)
}

func deleteKeys(source map[string]any, keys ...string) map[string]any {
	fields := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		fields[key] = struct{}{}
	}

	cleaned, ok := deleteKeysDeep(source, fields).(map[string]any)
	if !ok {
		return maps.Clone(source)
	}

	return cleaned
}

func deleteKeysDeep(value any, fields map[string]struct{}) any {
	switch v := value.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(v))
		for key, item := range v {
			if _, found := fields[key]; found {
				continue
			}

			cleaned[key] = deleteKeysDeep(item, fields)
		}
		return cleaned
	case []any:
		cleaned := make([]any, 0, len(v))
		for _, item := range v {
			cleaned = append(cleaned, deleteKeysDeep(item, fields))
		}
		return cleaned
	default:
		return value
	}
}

func removeFieldsAsserter(key string, fields ...string) assertionFunc {
	return func(t assert.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
		blob := actual.(map[string]any)
		var toCompare any

		switch v := blob[key].(type) {
		case []any:
			var cleaned []map[string]any
			for _, data := range blob[key].([]any) {
				cleaned = append(cleaned, deleteKeys(data.(map[string]any), fields...))
			}
			toCompare = cleaned
		case map[string]any:
			toCompare = deleteKeys(v, fields...)
		}

		return assert.Equal(t, expected, toCompare)
	}
}

func deleteSCM(t *testing.T, id string) {
	query := psql.Delete(
		dm.From("scms"),
		dm.Where(psql.Quote("id").EQ(psql.Arg(id))),
	)

	ctx := context.TODO()
	queryString, args, err := query.Build(ctx)
	require.NoError(t, err)

	_, err = database.DB.Exec(ctx, queryString, args...)
	assert.NoError(t, err)
}

func deleteLabel(t *testing.T, id uuid.UUID) {
	query := psql.Delete(
		dm.From("labels"),
		dm.Where(psql.Quote("id").EQ(psql.Arg(id))),
	)

	ctx := context.TODO()
	queryString, args, err := query.Build(ctx)
	require.NoError(t, err)

	_, err = database.DB.Exec(ctx, queryString, args...)
	assert.NoError(t, err)
}
