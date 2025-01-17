// Copyright 2020-2022 the Pinniped contributors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package oidc

import (
	"context"
	"net/url"
	"strings"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/openid"
	"github.com/pkg/errors"

	oidcapi "go.pinniped.dev/generated/latest/apis/supervisor/oidc"
	"go.pinniped.dev/internal/psession"
)

const (
	tokenTypeAccessToken = "urn:ietf:params:oauth:token-type:access_token" //nolint:gosec
	tokenTypeJWT         = "urn:ietf:params:oauth:token-type:jwt"          //nolint:gosec
)

type stsParams struct {
	subjectAccessToken string
	requestedAudience  string
}

func TokenExchangeFactory(config *compose.Config, storage interface{}, strategy interface{}) interface{} {
	return &TokenExchangeHandler{
		idTokenStrategy:     strategy.(openid.OpenIDConnectTokenStrategy),
		accessTokenStrategy: strategy.(oauth2.AccessTokenStrategy),
		accessTokenStorage:  storage.(oauth2.AccessTokenStorage),
	}
}

type TokenExchangeHandler struct {
	idTokenStrategy     openid.OpenIDConnectTokenStrategy
	accessTokenStrategy oauth2.AccessTokenStrategy
	accessTokenStorage  oauth2.AccessTokenStorage
}

var _ fosite.TokenEndpointHandler = (*TokenExchangeHandler)(nil)

func (t *TokenExchangeHandler) HandleTokenEndpointRequest(ctx context.Context, requester fosite.AccessRequester) error {
	if !t.CanHandleTokenEndpointRequest(requester) {
		return errors.WithStack(fosite.ErrUnknownRequest)
	}
	return nil
}

func (t *TokenExchangeHandler) PopulateTokenEndpointResponse(ctx context.Context, requester fosite.AccessRequester, responder fosite.AccessResponder) error {
	// Skip this request if it's for a different grant type.
	if err := t.HandleTokenEndpointRequest(ctx, requester); err != nil {
		return errors.WithStack(err)
	}

	// Validate the basic RFC8693 parameters we support.
	params, err := t.validateParams(requester.GetRequestForm())
	if err != nil {
		return errors.WithStack(err)
	}

	// Validate the incoming access token and lookup the information about the original authorize request.
	originalRequester, err := t.validateAccessToken(ctx, requester, params.subjectAccessToken)
	if err != nil {
		return errors.WithStack(err)
	}

	// Check that the currently authenticated client and the client which was originally used to get the access token are the same.
	if originalRequester.GetClient().GetID() != requester.GetClient().GetID() {
		// This error message is copied from the similar check in fosite's flow_authorize_code_token.go.
		return errors.WithStack(fosite.ErrInvalidGrant.WithHint("The OAuth 2.0 Client ID from this request does not match the one from the authorize request."))
	}

	// Check that the client is allowed to perform this grant type.
	if !requester.GetClient().GetGrantTypes().Has(oidcapi.GrantTypeTokenExchange) {
		// This error message is trying to be similar to the analogous one in fosite's flow_authorize_code_token.go.
		return errors.WithStack(fosite.ErrUnauthorizedClient.WithHintf(`The OAuth 2.0 Client is not allowed to use token exchange grant "%s".`, oidcapi.GrantTypeTokenExchange))
	}

	// Require that the incoming access token has the pinniped:request-audience and OpenID scopes.
	if !originalRequester.GetGrantedScopes().Has(oidcapi.ScopeRequestAudience) {
		return errors.WithStack(fosite.ErrAccessDenied.WithHintf("Missing the %q scope.", oidcapi.ScopeRequestAudience))
	}
	if !originalRequester.GetGrantedScopes().Has(oidcapi.ScopeOpenID) {
		return errors.WithStack(fosite.ErrAccessDenied.WithHintf("Missing the %q scope.", oidcapi.ScopeOpenID))
	}

	// Check that the stored session meets the minimum requirements for token exchange.
	if err := t.validateSession(originalRequester); err != nil {
		return errors.WithStack(err)
	}

	// Use the original authorize request information, along with the requested audience, to mint a new JWT.
	responseToken, err := t.mintJWT(ctx, originalRequester, params.requestedAudience)
	if err != nil {
		return errors.WithStack(err)
	}

	// Format the response parameters according to RFC8693.
	responder.SetAccessToken(responseToken)
	responder.SetTokenType("N_A")
	responder.SetExtra("issued_token_type", tokenTypeJWT)
	return nil
}

func (t *TokenExchangeHandler) mintJWT(ctx context.Context, requester fosite.Requester, audience string) (string, error) {
	downscoped := fosite.NewAccessRequest(requester.GetSession())
	downscoped.Client.(*fosite.DefaultClient).ID = audience
	return t.idTokenStrategy.GenerateIDToken(ctx, downscoped)
}

func (t *TokenExchangeHandler) validateSession(requester fosite.Requester) error {
	pSession, ok := requester.GetSession().(*psession.PinnipedSession)
	if !ok {
		// This shouldn't really happen.
		return fosite.ErrServerError.WithHint("Invalid session storage.")
	}
	username, ok := pSession.IDTokenClaims().Extra[oidcapi.IDTokenClaimUsername].(string)
	if !ok || username == "" {
		// No username was stored in the session's ID token claims (or the stored username was not a string, which
		// shouldn't really happen). Usernames will not be stored in the session's ID token claims when the username
		// scope was not requested/granted, but otherwise they should be stored.
		return fosite.ErrAccessDenied.WithHintf("No username found in session. Ensure that the %q scope was requested and granted at the authorization endpoint.", oidcapi.ScopeUsername)
	}
	return nil
}

func (t *TokenExchangeHandler) validateParams(params url.Values) (*stsParams, error) {
	var result stsParams

	// Validate some required parameters.
	result.requestedAudience = params.Get("audience")
	if result.requestedAudience == "" {
		return nil, fosite.ErrInvalidRequest.WithHint("Missing 'audience' parameter.")
	}
	result.subjectAccessToken = params.Get("subject_token")
	if result.subjectAccessToken == "" {
		return nil, fosite.ErrInvalidRequest.WithHint("Missing 'subject_token' parameter.")
	}

	// Validate some parameters with hardcoded values we support.
	if params.Get("subject_token_type") != tokenTypeAccessToken {
		return nil, fosite.ErrInvalidRequest.WithHintf("Unsupported 'subject_token_type' parameter value, must be %q.", tokenTypeAccessToken)
	}
	if params.Get("requested_token_type") != tokenTypeJWT {
		return nil, fosite.ErrInvalidRequest.WithHintf("Unsupported 'requested_token_type' parameter value, must be %q.", tokenTypeJWT)
	}

	// Validate that none of these unsupported parameters were sent. These are optional and we do not currently support them.
	for _, param := range []string{
		"resource",
		"scope",
		"actor_token",
		"actor_token_type",
	} {
		if params.Get(param) != "" {
			return nil, fosite.ErrInvalidRequest.WithHintf("Unsupported parameter %q.", param)
		}
	}

	// Validate that the requested audience is not one of the reserved strings. All possible requested audience strings
	// are subdivided into these classifications:
	// 1. pinniped-cli is reserved for the statically defined OAuth client, which is disallowed for this token exchange.
	// 2. client.oauth.pinniped.dev-* is reserved to be the names of user-defined dynamic OAuth clients, which is also
	//    disallowed for this token exchange.
	// 3. Anything else matching *.pinniped.dev* is reserved for future use, in case we want to create more
	//    buckets of names some day, e.g. something.pinniped.dev/*. These names are also disallowed for this
	//    token exchange.
	// 4. Any other string is reserved to conceptually mean the name of a workload cluster (technically, it's the
	//    configured audience of its Concierge JWTAuthenticator or other OIDC JWT validator). These are the only
	//    allowed values for this token exchange.
	if strings.Contains(result.requestedAudience, ".pinniped.dev") {
		return nil, fosite.ErrInvalidRequest.WithHintf("requested audience cannot contain '.pinniped.dev'")
	}
	if result.requestedAudience == oidcapi.ClientIDPinnipedCLI {
		return nil, fosite.ErrInvalidRequest.WithHintf("requested audience cannot equal '%s'", oidcapi.ClientIDPinnipedCLI)
	}

	return &result, nil
}

func (t *TokenExchangeHandler) validateAccessToken(ctx context.Context, requester fosite.AccessRequester, accessToken string) (fosite.Requester, error) {
	// Look up the access token's stored session data.
	signature := t.accessTokenStrategy.AccessTokenSignature(accessToken)
	originalRequester, err := t.accessTokenStorage.GetAccessTokenSession(ctx, signature, requester.GetSession())
	if err != nil {
		// The access token was not found, or there was some other error while reading it.
		return nil, fosite.ErrRequestUnauthorized.WithWrap(err).WithHint("Invalid 'subject_token' parameter value.")
	}
	// Validate the access token using its stored session data, which includes its expiration time.
	if err := t.accessTokenStrategy.ValidateAccessToken(ctx, originalRequester, accessToken); err != nil {
		return nil, errors.WithStack(err)
	}
	return originalRequester, nil
}

func (t *TokenExchangeHandler) CanSkipClientAuth(_ fosite.AccessRequester) bool {
	return false
}

func (t *TokenExchangeHandler) CanHandleTokenEndpointRequest(requester fosite.AccessRequester) bool {
	return requester.GetGrantTypes().ExactOne(oidcapi.GrantTypeTokenExchange)
}
