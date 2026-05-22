// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"fmt"

	"github.com/Snuffy2/shellport/application/log"
)

const (
	loaderChainTypeName = "LoaderChain"
)

// Redundant creates a group of loaders. They will be executed one by one until
// one of it successfully returned a configuration
func Redundant(loaders ...Loader) Loader {
	return func(log log.Logger) (string, Configuration, error) {
		ll := log.Context(loaderChainTypeName)
		for i := range loaders {
			if lLoaderName, lCfg, lErr := loaders[i](ll); lErr != nil {
				ll.Warning("Unable to load configuration from \"%s\": %s",
					lLoaderName, lErr)
				continue
			} else {
				return lLoaderName, lCfg, nil
			}
		}
		return loaderChainTypeName, Configuration{}, fmt.Errorf(
			"all configuration loaders failed")
	}
}
