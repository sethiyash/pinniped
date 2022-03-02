// Copyright 2020-2022 the Pinniped contributors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "go.pinniped.dev/generated/1.23/client/concierge/clientset/versioned/typed/authentication/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeAuthenticationV1alpha1 struct {
	*testing.Fake
}

func (c *FakeAuthenticationV1alpha1) JWTAuthenticators() v1alpha1.JWTAuthenticatorInterface {
	return &FakeJWTAuthenticators{c}
}

func (c *FakeAuthenticationV1alpha1) WebhookAuthenticators() v1alpha1.WebhookAuthenticatorInterface {
	return &FakeWebhookAuthenticators{c}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeAuthenticationV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
