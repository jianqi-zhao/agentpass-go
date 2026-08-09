package agentpass

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// IdentityService resolves the AgentPass account attached to an identity.basic grant.
type IdentityService struct {
	client *Client
}

// UserInfo is the verified AgentPass identity associated with an access token.
type UserInfo struct {
	Subject       string `json:"sub"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// UserInfo returns the stable AgentPass subject and verified profile. Use Subject
// as the external identity key in the application's own account/session store.
func (service *IdentityService) UserInfo(ctx context.Context) (*UserInfo, error) {
	request, err := service.client.newRequest(ctx, http.MethodGet, "/oauth/userinfo", nil)
	if err != nil {
		return nil, err
	}
	if err := service.client.authorizeRequest(ctx, request); err != nil {
		return nil, err
	}
	profile := &UserInfo{}
	if err := service.client.do(request, profile); err != nil {
		return nil, err
	}
	if strings.TrimSpace(profile.Subject) == "" ||
		strings.TrimSpace(profile.Email) == "" ||
		!profile.EmailVerified {
		return nil, fmt.Errorf("%w: AgentPass returned an incomplete or unverified identity", ErrInvalidResponse)
	}
	return profile, nil
}
