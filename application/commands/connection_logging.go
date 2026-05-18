// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Snuffy2/shellport/application/log"
)

type connectionDebugDetails struct {
	Protocol   string
	User       string
	Address    string
	Network    string
	AuthMethod string
	PresetID   string
}

func sshAuthMethodDebugName(method byte) string {
	switch method {
	case SSHAuthMethodNone:
		return "none"
	case SSHAuthMethodPassphrase:
		return "password"
	case SSHAuthMethodPrivateKey:
		return "private_key"
	default:
		return fmt.Sprintf("unknown(%d)", method)
	}
}

func (d connectionDebugDetails) fields() string {
	fields := make([]string, 0, 5)
	if d.User != "" {
		fields = append(fields, "user="+strconv.Quote(d.User))
	}
	if d.Address != "" {
		fields = append(fields, "address="+strconv.Quote(d.Address))
	}
	if d.Network != "" {
		fields = append(fields, "network="+strconv.Quote(d.Network))
	}
	if d.AuthMethod != "" {
		fields = append(fields, "auth_method="+strconv.Quote(d.AuthMethod))
	}
	if d.PresetID != "" {
		fields = append(fields, "preset_id="+strconv.Quote(d.PresetID))
	}

	return strings.Join(fields, " ")
}

func debugConnectionAttempt(l log.Logger, details connectionDebugDetails) {
	fields := details.fields()
	if fields == "" {
		l.Debug("Attempting %s connection", details.Protocol)
		return
	}
	l.Debug("Attempting %s connection: %s", details.Protocol, fields)
}

func debugConnectionFailed(l log.Logger, details connectionDebugDetails, err error) {
	l.Debug("%s connection failed: %s error=%s", details.Protocol, details.fields(), err)
}

func debugConnectionEstablished(l log.Logger, details connectionDebugDetails) {
	l.Debug("%s connection established: %s", details.Protocol, details.fields())
}

func debugConnectionDisconnected(l log.Logger, details connectionDebugDetails, reason string, err error) {
	if err == nil {
		l.Debug("%s connection disconnected: %s reason=%s", details.Protocol, details.fields(), reason)
		return
	}
	l.Debug("%s connection disconnected: %s reason=%s error=%s", details.Protocol, details.fields(), reason, err)
}
