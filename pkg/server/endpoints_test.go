package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

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
		}, removeFieldsAsserter("scms", "total_count", "Created_at", "Updated_at"))
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
		}, removeFieldsAsserter("scms", "total_count", "Created_at", "Updated_at"))
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
						"Conditions": nil,
						"Targets":    nil,
						"ReportURL":  "",
					},
					"FilteredResourceID": "",
				},
			}, removeFieldsAsserter("data", "CreatedAt", "UpdatedAt"))
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
						"Conditions": nil,
						"Targets":    nil,
						"ReportURL":  "",
					},
					"FilteredResourceID": "",
				},
			}, removeFieldsAsserter("data", "CreatedAt", "UpdatedAt"))
		})
	})

	t.Run("GET /api/pipeline/reports/:id", func(t *testing.T) {
		t.Run("with an unknown report ID", func(t *testing.T) {
			resp := doGetRequest(t, srv, "/api/pipeline/reports/daa9b61e-42b9-4e35-b9d7-071461a36838")
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
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
					"Conditions": nil,
					"Targets":    nil,
					"ReportURL":  "",
				},
			}, removeFieldsAsserter("data", "Created_at", "Updated_at"))
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
						"ID":   config1,
						"Kind": "testing-1",
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
						"ID":   config2,
						"Kind": "testing-2",
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
}

func doGetRequest(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", ts.URL, path), nil)
	require.NoError(t, err)

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

	assertJSONResponseWithCode(t, res, code, map[string]any{"error": wantMsg}, assert.Equal)
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
	updated := maps.Clone(source)
	for _, key := range keys {
		delete(updated, key)
	}

	return updated
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
