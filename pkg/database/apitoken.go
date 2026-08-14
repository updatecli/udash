package database

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/model"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/dm"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	"github.com/stephenafamo/bob/dialect/psql/um"
)

// ErrAPITokenNotFound is returned when no token matches the request.
var ErrAPITokenNotFound = errors.New("api token not found")

// apiTokenColumns is the column list every read shares, in scan order.
var apiTokenColumns = []any{
	"id", "name", "subject", "permission", "scopes",
	"created_at", "last_used_at", "expires_at",
}

// InsertAPIToken stores a new token and returns it.
//
// Only the hash is passed in: the token itself is shown once, to whoever created
// it, and is never written down.
func InsertAPIToken(ctx context.Context, name, subject, permission string, scopes []string, tokenHash []byte, expiresAt *time.Time) (*model.APIToken, error) {
	query := psql.Insert(
		im.Into("api_tokens", "name", "subject", "permission", "scopes", "token_hash", "expires_at"),
		im.Values(
			psql.Arg(name),
			psql.Arg(subject),
			psql.Arg(permission),
			psql.Arg(scopes),
			psql.Arg(tokenHash),
			psql.Arg(expiresAt),
		),
		im.Returning(apiTokenColumns...),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, err
	}

	token := model.APIToken{}
	if err := scanAPIToken(DB.QueryRow(ctx, queryString, args...), &token); err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return nil, err
	}

	return &token, nil
}

// GetAPITokenByHash returns the token matching the given hash.
//
// Looking a token up by its hash is what makes the stored value useless to anybody
// who reads the database: it cannot be turned back into a usable credential.
func GetAPITokenByHash(ctx context.Context, tokenHash []byte) (*model.APIToken, error) {
	query := psql.Select(
		sm.Columns(apiTokenColumns...),
		sm.From("api_tokens"),
		sm.Where(psql.Quote("token_hash").EQ(psql.Arg(tokenHash))),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, err
	}

	token := model.APIToken{}
	if err := scanAPIToken(DB.QueryRow(ctx, queryString, args...), &token); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPITokenNotFound
		}
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return nil, err
	}

	return &token, nil
}

// ListAPITokens returns the tokens of a subject, or every token when subject is
// empty, which only an administrator may ask for.
func ListAPITokens(ctx context.Context, subject string) ([]model.APIToken, error) {
	mods := []bob.Mod[*dialect.SelectQuery]{
		sm.Columns(apiTokenColumns...),
		sm.From("api_tokens"),
		sm.OrderBy("created_at").Desc(),
	}
	if subject != "" {
		mods = append(mods, sm.Where(psql.Quote("subject").EQ(psql.Arg(subject))))
	}

	query := psql.Select(mods...)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return nil, err
	}

	rows, err := DB.Query(ctx, queryString, args...)
	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return nil, err
	}
	defer rows.Close()

	tokens := []model.APIToken{}
	for rows.Next() {
		token := model.APIToken{}
		if err := scanAPIToken(rows, &token); err != nil {
			logrus.Errorf("parsing result: %s", err)
			return nil, err
		}
		tokens = append(tokens, token)
	}

	return tokens, rows.Err()
}

// DeleteAPIToken removes a token. A non empty subject restricts the deletion to
// that subject's own tokens, so one identity cannot revoke another's.
func DeleteAPIToken(ctx context.Context, id uuid.UUID, subject string) error {
	mods := []bob.Mod[*dialect.DeleteQuery]{
		dm.From("api_tokens"),
		dm.Where(psql.Quote("id").EQ(psql.Arg(id))),
	}
	if subject != "" {
		mods = append(mods, dm.Where(psql.Quote("subject").EQ(psql.Arg(subject))))
	}

	query := psql.Delete(mods...)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return err
	}

	result, err := DB.Exec(ctx, queryString, args...)
	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrAPITokenNotFound
	}

	return nil
}

// DeleteAPITokensBySubject removes every token of a subject, which is what
// offboarding an identity needs.
func DeleteAPITokensBySubject(ctx context.Context, subject string) (int64, error) {
	query := psql.Delete(
		dm.From("api_tokens"),
		dm.Where(psql.Quote("subject").EQ(psql.Arg(subject))),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		logrus.Errorf("building query failed: %s\n\t%s", queryString, err)
		return 0, err
	}

	result, err := DB.Exec(ctx, queryString, args...)
	if err != nil {
		logrus.Errorf("query failed: %q\n\t%s", queryString, err)
		return 0, err
	}

	return result.RowsAffected(), nil
}

// TouchAPIToken records that a token was just used.
//
// A failure here is not worth failing the request it belongs to: the timestamp is
// there to help somebody spot an unused or a leaked token, not to authorize.
func TouchAPIToken(ctx context.Context, id uuid.UUID) error {
	query := psql.Update(
		um.Table("api_tokens"),
		um.SetCol("last_used_at").ToArg(time.Now()),
		um.Where(psql.Quote("id").EQ(psql.Arg(id))),
	)

	queryString, args, err := query.Build(ctx)
	if err != nil {
		return err
	}

	_, err = DB.Exec(ctx, queryString, args...)
	return err
}

// scanner is what pgx rows and single rows have in common.
type scanner interface {
	Scan(dest ...any) error
}

func scanAPIToken(row scanner, token *model.APIToken) error {
	return row.Scan(
		&token.ID,
		&token.Name,
		&token.Subject,
		&token.Permission,
		&token.Scopes,
		&token.CreatedAt,
		&token.LastUsedAt,
		&token.ExpiresAt,
	)
}
