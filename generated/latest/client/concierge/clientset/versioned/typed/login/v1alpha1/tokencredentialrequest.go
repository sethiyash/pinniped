// Copyright 2020-2022 the Pinniped contributors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"

	v1alpha1 "go.pinniped.dev/generated/latest/apis/concierge/login/v1alpha1"
	scheme "go.pinniped.dev/generated/latest/client/concierge/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rest "k8s.io/client-go/rest"
)

// TokenCredentialRequestsGetter has a method to return a TokenCredentialRequestInterface.
// A group's client should implement this interface.
type TokenCredentialRequestsGetter interface {
	TokenCredentialRequests() TokenCredentialRequestInterface
}

// TokenCredentialRequestInterface has methods to work with TokenCredentialRequest resources.
type TokenCredentialRequestInterface interface {
	Create(ctx context.Context, tokenCredentialRequest *v1alpha1.TokenCredentialRequest, opts v1.CreateOptions) (*v1alpha1.TokenCredentialRequest, error)
	TokenCredentialRequestExpansion
}

// tokenCredentialRequests implements TokenCredentialRequestInterface
type tokenCredentialRequests struct {
	client rest.Interface
}

// newTokenCredentialRequests returns a TokenCredentialRequests
func newTokenCredentialRequests(c *LoginV1alpha1Client) *tokenCredentialRequests {
	return &tokenCredentialRequests{
		client: c.RESTClient(),
	}
}

// Create takes the representation of a tokenCredentialRequest and creates it.  Returns the server's representation of the tokenCredentialRequest, and an error, if there is any.
func (c *tokenCredentialRequests) Create(ctx context.Context, tokenCredentialRequest *v1alpha1.TokenCredentialRequest, opts v1.CreateOptions) (result *v1alpha1.TokenCredentialRequest, err error) {
	result = &v1alpha1.TokenCredentialRequest{}
	err = c.client.Post().
		Resource("tokencredentialrequests").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(tokenCredentialRequest).
		Do(ctx).
		Into(result)
	return
}
