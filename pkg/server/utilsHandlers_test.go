package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
)

func TestRespondWithError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		err          error
		expectedCode int
		expectedErr  string
	}{
		{
			name:         "invalid parameter is returned to the caller",
			err:          fmt.Errorf("%w: parsing startTime: bad", database.ErrInvalidParameter),
			expectedCode: http.StatusBadRequest,
			expectedErr:  "invalid parameter: parsing startTime: bad",
		},
		{
			name:         "anything else is kept to the logs",
			err:          errors.New(`ERROR: relation "pipelinereports" does not exist (SQLSTATE 42P01)`),
			expectedCode: http.StatusInternalServerError,
			expectedErr:  ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			respondWithError(c, tt.err)

			require.Equal(t, tt.expectedCode, w.Code)

			got := DefaultResponseModel{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Equal(t, tt.expectedErr, got.Err)
		})
	}
}
