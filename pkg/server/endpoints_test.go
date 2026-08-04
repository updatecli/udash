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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/test"
	"github.com/updatecli/updatecli/pkg/core/reports"
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
		// a zeroed set of results.
		bucketEntry := func(date string, results map[string]any) map[string]any {
			allResults := map[string]any{
				"✔":       float64(0),
				"✗":       float64(0),
				"⚠":       float64(0),
				"-":       float64(0),
				"unknown": float64(0),
			}
			total := float64(0)
			for k, v := range results {
				allResults[k] = v
				total += v.(float64)
			}

			return map[string]any{
				"date":    date,
				"results": allResults,
				"total":   total,
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

			// Halfway into each hour, so that a report cannot land in a neighbouring
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
