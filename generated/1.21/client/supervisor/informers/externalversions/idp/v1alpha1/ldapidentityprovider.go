// Copyright 2020-2022 the Pinniped contributors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	idpv1alpha1 "go.pinniped.dev/generated/1.21/apis/supervisor/idp/v1alpha1"
	versioned "go.pinniped.dev/generated/1.21/client/supervisor/clientset/versioned"
	internalinterfaces "go.pinniped.dev/generated/1.21/client/supervisor/informers/externalversions/internalinterfaces"
	v1alpha1 "go.pinniped.dev/generated/1.21/client/supervisor/listers/idp/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// LDAPIdentityProviderInformer provides access to a shared informer and lister for
// LDAPIdentityProviders.
type LDAPIdentityProviderInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.LDAPIdentityProviderLister
}

type lDAPIdentityProviderInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewLDAPIdentityProviderInformer constructs a new informer for LDAPIdentityProvider type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewLDAPIdentityProviderInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredLDAPIdentityProviderInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredLDAPIdentityProviderInformer constructs a new informer for LDAPIdentityProvider type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredLDAPIdentityProviderInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.IDPV1alpha1().LDAPIdentityProviders(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.IDPV1alpha1().LDAPIdentityProviders(namespace).Watch(context.TODO(), options)
			},
		},
		&idpv1alpha1.LDAPIdentityProvider{},
		resyncPeriod,
		indexers,
	)
}

func (f *lDAPIdentityProviderInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredLDAPIdentityProviderInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *lDAPIdentityProviderInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&idpv1alpha1.LDAPIdentityProvider{}, f.defaultInformer)
}

func (f *lDAPIdentityProviderInformer) Lister() v1alpha1.LDAPIdentityProviderLister {
	return v1alpha1.NewLDAPIdentityProviderLister(f.Informer().GetIndexer())
}
