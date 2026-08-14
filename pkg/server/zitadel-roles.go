package server

import (
	"context"
	"fmt"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/zitadel-go/v3/pkg/client"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user"
	"github.com/zitadel/zitadel-go/v3/pkg/zitadel"
)

// newZitadelUserRoles builds a lookup of the roles currently granted to a subject.
//
// A request authenticating with an Udash API token carries no Zitadel token, so
// there are no claims to read the roles from and they have to be asked for. The
// service user behind the key file needs permission to read user grants in the
// organisation, otherwise every lookup fails and the resolver falls back on the
// permission recorded when the token was created.
func newZitadelUserRoles(ctx context.Context, opts ZitadelOptions) (zitadelUserRoles, error) {
	api, err := client.New(ctx, zitadel.New(opts.Domain),
		client.WithAuth(client.DefaultServiceUserAuthentication(
			opts.KeyFile,
			oidc.ScopeOpenID,
			client.ScopeZitadelAPI(),
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to Zitadel: %w", err)
	}

	return func(ctx context.Context, subject string) ([]string, error) {
		resp, err := api.ManagementService().ListUserGrants(ctx, &management.ListUserGrantRequest{
			Queries: []*user.UserGrantQuery{
				{
					Query: &user.UserGrantQuery_UserIdQuery{
						UserIdQuery: &user.UserGrantUserIDQuery{UserId: subject},
					},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("listing the grants of %q: %w", subject, err)
		}

		roles := []string{}
		for _, grant := range resp.GetResult() {
			roles = append(roles, grant.GetRoleKeys()...)
		}

		return roles, nil
	}, nil
}
