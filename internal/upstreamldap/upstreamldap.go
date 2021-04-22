// Copyright 2021 the Pinniped contributors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package upstreamldap implements an abstraction of upstream LDAP IDP interactions.
package upstreamldap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"

	"go.pinniped.dev/internal/plog"
)

const (
	ldapsScheme                                 = "ldaps"
	distinguishedNameAttributeName              = "dn"
	userSearchFilterInterpolationLocationMarker = "{}"
	invalidCredentialsErrorPrefix               = `LDAP Result Code 49 "Invalid Credentials":`
)

// Conn abstracts the upstream LDAP communication protocol (mostly for testing).
type Conn interface {
	Bind(username, password string) error

	Search(searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error)

	Close()
}

// Our Conn type is subset of the ldap.Client interface, which is implemented by ldap.Conn.
var _ Conn = &ldap.Conn{}

// LDAPDialer is a factory of Conn, and the resulting Conn can then be used to interact with an upstream LDAP IDP.
type LDAPDialer interface {
	Dial(ctx context.Context, hostAndPort string) (Conn, error)
}

// LDAPDialerFunc makes it easy to use a func as an LDAPDialer.
type LDAPDialerFunc func(ctx context.Context, hostAndPort string) (Conn, error)

func (f LDAPDialerFunc) Dial(ctx context.Context, hostAndPort string) (Conn, error) {
	return f(ctx, hostAndPort)
}

// ProviderConfig includes all of the settings for connection and searching for users and groups in
// the upstream LDAP IDP. It also provides methods for testing the connection and performing logins.
// The nested structs are not pointer fields to enable deep copy on function params and return values.
type ProviderConfig struct {
	// Name is the unique name of this upstream LDAP IDP.
	Name string

	// Host is the hostname or "hostname:port" of the LDAP server. When the port is not specified,
	// the default LDAP port will be used.
	Host string

	// PEM-encoded CA cert bundle to trust when connecting to the LDAP server. Can be nil.
	CABundle []byte

	// BindUsername is the username to use when performing a bind with the upstream LDAP IDP.
	BindUsername string

	// BindPassword is the password to use when performing a bind with the upstream LDAP IDP.
	BindPassword string

	// UserSearch contains information about how to search for users in the upstream LDAP IDP.
	UserSearch UserSearchConfig

	// Dialer exists to enable testing. When nil, will use a default appropriate for production use.
	Dialer LDAPDialer
}

// UserSearchConfig contains information about how to search for users in the upstream LDAP IDP.
type UserSearchConfig struct {
	// Base is the base DN to use for the user search in the upstream LDAP IDP.
	Base string

	// Filter is the filter to use for the user search in the upstream LDAP IDP.
	Filter string

	// UsernameAttribute is the attribute in the LDAP entry from which the username should be
	// retrieved.
	UsernameAttribute string

	// UIDAttribute is the attribute in the LDAP entry from which the user's unique ID should be
	// retrieved.
	UIDAttribute string
}

type Provider struct {
	c ProviderConfig
}

// Create a Provider. The config is not a pointer to ensure that a copy of the config is created,
// making the resulting Provider use an effectively read-only configuration.
func New(config ProviderConfig) *Provider {
	return &Provider{c: config}
}

// A reader for the config. Returns a copy of the config to keep the underlying config read-only.
func (p *Provider) GetConfig() ProviderConfig {
	return p.c
}

func (p *Provider) dial(ctx context.Context) (Conn, error) {
	hostAndPort, err := hostAndPortWithDefaultPort(p.c.Host, ldap.DefaultLdapsPort)
	if err != nil {
		return nil, ldap.NewError(ldap.ErrorNetwork, err)
	}
	if p.c.Dialer != nil {
		return p.c.Dialer.Dial(ctx, hostAndPort)
	}
	return p.dialTLS(ctx, hostAndPort)
}

// dialTLS is the default implementation of the Dialer, used when Dialer is nil.
// Unfortunately, the go-ldap library does not seem to support dialing with a context.Context,
// so we implement it ourselves, heavily inspired by ldap.DialURL.
func (p *Provider) dialTLS(ctx context.Context, hostAndPort string) (Conn, error) {
	var rootCAs *x509.CertPool
	if p.c.CABundle != nil {
		rootCAs = x509.NewCertPool()
		if !rootCAs.AppendCertsFromPEM(p.c.CABundle) {
			return nil, ldap.NewError(ldap.ErrorNetwork, fmt.Errorf("could not parse CA bundle"))
		}
	}

	dialer := &tls.Dialer{Config: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    rootCAs,
	}}

	c, err := dialer.DialContext(ctx, "tcp", hostAndPort)
	if err != nil {
		return nil, ldap.NewError(ldap.ErrorNetwork, err)
	}

	conn := ldap.NewConn(c, true)
	conn.Start()
	return conn, nil
}

// Adds the default port if hostAndPort did not already include a port.
func hostAndPortWithDefaultPort(hostAndPort string, defaultPort string) (string, error) {
	host, port, err := net.SplitHostPort(hostAndPort)
	if err != nil {
		if strings.HasSuffix(err.Error(), ": missing port in address") { // sad to need to do this string compare
			host = hostAndPort
			port = defaultPort
		} else {
			return "", err // hostAndPort argument was not parsable
		}
	}
	switch {
	case port != "" && strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]"):
		// don't add extra square brackets to an IPv6 address that already has them
		return host + ":" + port, nil
	case port != "":
		return net.JoinHostPort(host, port), nil
	default:
		return host, nil
	}
}

// A name for this upstream provider.
func (p *Provider) GetName() string {
	return p.c.Name
}

// Return a URL which uniquely identifies this LDAP provider, e.g. "ldaps://host.example.com:1234".
// This URL is not used for connecting to the provider, but rather is used for creating a globally unique user
// identifier by being combined with the user's UID, since user UIDs are only unique within one provider.
func (p *Provider) GetURL() string {
	return fmt.Sprintf("%s://%s", ldapsScheme, p.c.Host)
}

// TestConnection provides a method for testing the connection and bind settings. It performs a dial and bind
// and returns any errors that we encountered.
func (p *Provider) TestConnection(ctx context.Context) error {
	err := p.validateConfig()
	if err != nil {
		return err
	}

	conn, err := p.dial(ctx)
	if err != nil {
		return fmt.Errorf(`error dialing host "%s": %w`, p.c.Host, err)
	}
	defer conn.Close()

	err = conn.Bind(p.c.BindUsername, p.c.BindPassword)
	if err != nil {
		return fmt.Errorf(`error binding as "%s": %w`, p.c.BindUsername, err)
	}

	return nil
}

// DryRunAuthenticateUser provides a method for testing all of the Provider settings in a kind of dry run of
// authentication for a given end user's username. It runs the same logic as AuthenticateUser except it does
// not bind as that user, so it does not test their password. It returns the same values that a real call to
// AuthenticateUser with the correct password would return.
func (p *Provider) DryRunAuthenticateUser(ctx context.Context, username string) (*authenticator.Response, bool, error) {
	endUserBindFunc := func(conn Conn, foundUserDN string) error {
		// Act as if the end user bind always succeeds.
		return nil
	}
	return p.authenticateUserImpl(ctx, username, endUserBindFunc)
}

// Authenticate an end user and return their mapped username, groups, and UID. Implements authenticators.UserAuthenticator.
func (p *Provider) AuthenticateUser(ctx context.Context, username, password string) (*authenticator.Response, bool, error) {
	endUserBindFunc := func(conn Conn, foundUserDN string) error {
		return conn.Bind(foundUserDN, password)
	}
	return p.authenticateUserImpl(ctx, username, endUserBindFunc)
}

func (p *Provider) authenticateUserImpl(ctx context.Context, username string, bindFunc func(conn Conn, foundUserDN string) error) (*authenticator.Response, bool, error) {
	err := p.validateConfig()
	if err != nil {
		return nil, false, err
	}

	if len(username) == 0 {
		// Empty passwords are already handled by go-ldap.
		return nil, false, nil
	}

	conn, err := p.dial(ctx)
	if err != nil {
		return nil, false, fmt.Errorf(`error dialing host "%s": %w`, p.c.Host, err)
	}
	defer conn.Close()

	err = conn.Bind(p.c.BindUsername, p.c.BindPassword)
	if err != nil {
		return nil, false, fmt.Errorf(`error binding as "%s" before user search: %w`, p.c.BindUsername, err)
	}

	mappedUsername, mappedUID, err := p.searchAndBindUser(conn, username, bindFunc)
	if err != nil {
		return nil, false, err
	}
	if len(mappedUsername) == 0 || len(mappedUID) == 0 {
		// Couldn't find the username or couldn't bind using the password.
		return nil, false, nil
	}

	response := &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   mappedUsername,
			UID:    mappedUID,
			Groups: []string{}, // Support for group search coming soon.
		},
	}
	return response, true, nil
}

func (p *Provider) validateConfig() error {
	if p.c.UserSearch.UsernameAttribute == distinguishedNameAttributeName && len(p.c.UserSearch.Filter) == 0 {
		// LDAP search filters do not allow searching by DN, so we would have no reasonable default for Filter.
		return fmt.Errorf(`must specify UserSearch Filter when UserSearch UsernameAttribute is "dn"`)
	}
	return nil
}

func (p *Provider) searchAndBindUser(conn Conn, username string, bindFunc func(conn Conn, foundUserDN string) error) (string, string, error) {
	searchResult, err := conn.Search(p.userSearchRequest(username))
	if err != nil {
		return "", "", fmt.Errorf(`error searching for user "%s": %w`, username, err)
	}
	if len(searchResult.Entries) == 0 {
		plog.Debug("error finding user: user not found (if this username is valid, please check the user search configuration)",
			"upstreamName", p.GetName(), "username", username)
		return "", "", nil
	}
	if len(searchResult.Entries) > 1 {
		return "", "", fmt.Errorf(`searching for user "%s" resulted in %d search results, but expected 1 result`,
			username, len(searchResult.Entries),
		)
	}
	userEntry := searchResult.Entries[0]
	if len(userEntry.DN) == 0 {
		return "", "", fmt.Errorf(`searching for user "%s" resulted in search result without DN`, username)
	}

	mappedUsername, err := p.getSearchResultAttributeValue(p.c.UserSearch.UsernameAttribute, userEntry, username)
	if err != nil {
		return "", "", err
	}

	mappedUID, err := p.getSearchResultAttributeValue(p.c.UserSearch.UIDAttribute, userEntry, username)
	if err != nil {
		return "", "", err
	}

	// Caution: Note that any other LDAP commands after this bind will be run as this user instead of as the configured BindUsername!
	err = bindFunc(conn, userEntry.DN)
	if err != nil {
		plog.DebugErr("error binding for user (if this is not the expected dn for this username, please check the user search configuration)",
			err, "upstreamName", p.GetName(), "username", username, "dn", userEntry.DN)
		if strings.HasPrefix(err.Error(), invalidCredentialsErrorPrefix) {
			return "", "", nil
		}
		return "", "", fmt.Errorf(`error binding for user "%s" using provided password against DN "%s": %w`, username, userEntry.DN, err)
	}

	return mappedUsername, mappedUID, nil
}

func (p *Provider) userSearchRequest(username string) *ldap.SearchRequest {
	// See https://ldap.com/the-ldap-search-operation for general documentation of LDAP search options.
	return &ldap.SearchRequest{
		BaseDN:       p.c.UserSearch.Base,
		Scope:        ldap.ScopeWholeSubtree,
		DerefAliases: ldap.DerefAlways, // TODO what's the best value here?
		SizeLimit:    2,
		TimeLimit:    90,
		TypesOnly:    false,
		Filter:       p.userSearchFilter(username),
		Attributes:   p.userSearchRequestedAttributes(),
		Controls:     nil, // this could be used to enable paging, but we're already limiting the result max size
	}
}

func (p *Provider) userSearchRequestedAttributes() []string {
	attributes := []string{}
	if p.c.UserSearch.UsernameAttribute != distinguishedNameAttributeName {
		attributes = append(attributes, p.c.UserSearch.UsernameAttribute)
	}
	if p.c.UserSearch.UIDAttribute != distinguishedNameAttributeName {
		attributes = append(attributes, p.c.UserSearch.UIDAttribute)
	}
	return attributes
}

func (p *Provider) userSearchFilter(username string) string {
	safeUsername := p.escapeUsernameForSearchFilter(username)
	if len(p.c.UserSearch.Filter) == 0 {
		return fmt.Sprintf("(%s=%s)", p.c.UserSearch.UsernameAttribute, safeUsername)
	}
	filter := strings.ReplaceAll(p.c.UserSearch.Filter, userSearchFilterInterpolationLocationMarker, safeUsername)
	if strings.HasPrefix(filter, "(") && strings.HasSuffix(filter, ")") {
		return filter
	}
	return "(" + filter + ")"
}

func (p *Provider) escapeUsernameForSearchFilter(username string) string {
	// The username is end user input, so it should be escaped before being included in a search to prevent query injection.
	return ldap.EscapeFilter(username)
}

func (p *Provider) getSearchResultAttributeValue(attributeName string, fromUserEntry *ldap.Entry, username string) (string, error) {
	if attributeName == distinguishedNameAttributeName {
		return fromUserEntry.DN, nil
	}

	attributeValues := fromUserEntry.GetAttributeValues(attributeName)

	if len(attributeValues) != 1 {
		return "", fmt.Errorf(`found %d values for attribute "%s" while searching for user "%s", but expected 1 result`,
			len(attributeValues), attributeName, username,
		)
	}

	attributeValue := attributeValues[0]
	if len(attributeValue) == 0 {
		return "", fmt.Errorf(`found empty value for attribute "%s" while searching for user "%s", but expected value to be non-empty`,
			attributeName, username,
		)
	}

	return attributeValue, nil
}
