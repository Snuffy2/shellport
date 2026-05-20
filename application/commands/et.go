// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package commands

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/rw"
)

// Server-to-client stream marker constants for ET.
const (
	ETServerRemoteStdOut               = 0x00
	ETServerHookOutputBeforeConnecting = 0x01
	ETServerConnectFailed              = 0x02
	ETServerConnectSucceed             = 0x03
	ETServerConnectVerifyFingerprint   = 0x05
	ETServerConnectRequestCredential   = 0x06
)

// Client-to-server stream marker constants for ET.
const (
	ETClientStdIn              = 0x00
	ETClientResize             = 0x01
	ETClientRespondFingerprint = 0x02
	ETClientRespondCredential  = 0x03
)

// ET bootup stream error codes.
const (
	ETRequestErrorBadUserName      = command.StreamError(0x01)
	ETRequestErrorBadRemoteAddress = command.StreamError(0x02)
	ETRequestErrorBadAuthMethod    = command.StreamError(0x03)
	ETRequestErrorUnsupportedProxy = command.StreamError(0x04)
	ETRequestErrorBadMetadata      = command.StreamError(0x05)
)

const (
	etMaxUsernameLen = 127
	etMaxHostnameLen = 255
)

var (
	ErrETSocks5Unsupported = errors.New(
		"ET does not support SOCKS5 proxying in this version")

	ErrETUnsupportedAuthMethod = errors.New(
		"invalid ET auth method")

	ErrETRemoteUnavailable = errors.New(
		"remote ET process is unavailable")
)

type etClient struct {
	w     command.StreamResponder
	l     log.Logger
	hooks command.Hooks
	cfg   command.Configuration

	meta       etMetadata
	bufferPool *command.BufferPool

	baseCtx         context.Context
	baseCtxCancel   func()
	remoteCloseWait sync.WaitGroup

	credentialReceive                       chan []byte
	credentialReceiveCloseOnce              sync.Once
	fingerprintVerifyResultReceive          chan bool
	fingerprintVerifyResultReceiveCloseOnce sync.Once
	credentialProcessed                     bool
	fingerprintProcessed                    bool
	credentialReceiveClosed                 bool
	fingerprintVerifyResultReceiveClosed    bool
	processedLock                           sync.Mutex

	remoteStarter func(user string, address string, authMethodBuilder sshAuthMethodBuilder, metadata etMetadata, presetID string)

	privateKey            []byte
	privateKeyLock        sync.Mutex
	sendCredentialRequest func([]byte) error
}

func newET(
	l log.Logger,
	hooks command.Hooks,
	w command.StreamResponder,
	cfg command.Configuration,
	bufferPool *command.BufferPool,
) command.FSMMachine {
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &etClient{
		w:             w,
		l:             l,
		hooks:         hooks,
		cfg:           cfg,
		meta:          defaultETMetadata(),
		bufferPool:    bufferPool,
		baseCtx:       ctx,
		baseCtxCancel: sync.OnceFunc(ctxCancel),

		credentialReceive:                    make(chan []byte, 1),
		credentialProcessed:                  false,
		credentialReceiveClosed:              false,
		fingerprintVerifyResultReceive:       make(chan bool, 1),
		fingerprintProcessed:                 false,
		fingerprintVerifyResultReceiveClosed: false,
		processedLock:                        sync.Mutex{},
		privateKey:                           nil,
		privateKeyLock:                       sync.Mutex{},
	}
}

func parseETConfig(p configuration.Preset) (configuration.Preset, error) {
	return parseSSHConfig(p)
}

func (d *etClient) Bootup(
	r *rw.LimitedReader,
	b []byte,
) (command.FSMState, command.FSMError) {
	if err := d.validateProxySupport(); err != nil {
		return nil, command.ToFSMError(err, ETRequestErrorUnsupportedProxy)
	}

	sBuf := d.bufferPool.Get()
	defer d.bufferPool.Put(sBuf)

	userName, userNameErr := ParseString(r.Read, (*sBuf)[:etMaxUsernameLen])
	if userNameErr != nil {
		return nil, command.ToFSMError(userNameErr, ETRequestErrorBadUserName)
	}
	userNameStr := string(userName.Data())

	addr, addrErr := ParseAddress(r.Read, (*sBuf)[:etMaxHostnameLen])
	if addrErr != nil {
		return nil, command.ToFSMError(addrErr, ETRequestErrorBadRemoteAddress)
	}
	addrStr := addr.String()
	if addrStr == "" {
		return nil, command.ToFSMError(ErrSSHInvalidAddress, ETRequestErrorBadRemoteAddress)
	}

	authData, authErr := rw.FetchOneByte(r.Fetch)
	if authErr != nil {
		return nil, command.ToFSMError(authErr, ETRequestErrorBadAuthMethod)
	}

	metadata, metadataErr := parseETMetadata(r, (*sBuf)[:])
	if metadataErr != nil {
		return nil, command.ToFSMError(metadataErr, ETRequestErrorBadMetadata)
	}
	d.meta = metadata

	presetID, presetIDErr := parseOptionalPresetID(r, (*sBuf)[:configuration.MaxPresetIDLength])
	if presetIDErr != nil {
		return nil, command.ToFSMError(presetIDErr, ETRequestErrorBadMetadata)
	}

	authMethodBuilder, authMethodErr := d.buildAuthMethod(authData[0], presetID, userNameStr, addrStr)
	if authMethodErr != nil {
		return nil, command.ToFSMError(authMethodErr, ETRequestErrorBadAuthMethod)
	}

	d.remoteCloseWait.Add(1)
	if d.remoteStarter != nil {
		go d.remoteStarter(userNameStr, addrStr, authMethodBuilder, metadata, presetID)
	} else {
		go d.remote(userNameStr, addrStr, authMethodBuilder, metadata, presetID)
	}

	return d.local, command.NoFSMError()
}

func (d *etClient) remote(
	user string,
	address string,
	authMethodBuilder sshAuthMethodBuilder,
	metadata etMetadata,
	presetID string,
) {
	defer d.remoteCloseWait.Done()
	d.baseCtxCancel()
	_ = user
	_ = address
	_ = authMethodBuilder
	_ = metadata
	_ = presetID
}

func (d *etClient) validateProxySupport() error {
	if d.cfg.Socks5Configured {
		return ErrETSocks5Unsupported
	}

	return nil
}

func (d *etClient) buildAuthMethod(
	methodType byte,
	_ string,
	_ string,
	_ string,
) (sshAuthMethodBuilder, error) {
	switch methodType {
	case SSHAuthMethodPrivateKey:
		return func(b []byte) []ssh.AuthMethod {
			return []ssh.AuthMethod{
				ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
					privateKeyBytes, privateKeyErr := d.requestPrivateKey(b)
					if privateKeyErr != nil {
						return nil, privateKeyErr
					}

					signer, signerErr := ssh.ParsePrivateKey(privateKeyBytes)
					if signerErr != nil {
						return nil, signerErr
					}

					d.cachePrivateKey(privateKeyBytes)

					return []ssh.Signer{signer}, nil
				}),
			}
		}, nil
	default:
		return nil, ErrETUnsupportedAuthMethod
	}
}

func (d *etClient) requestPrivateKey(b []byte) ([]byte, error) {
	if privateKey, ok := d.privateKeyForET(); ok {
		privateKeyClone := make([]byte, len(privateKey))
		copy(privateKeyClone, privateKey)
		return privateKeyClone, nil
	}

	var wErr error
	if d.sendCredentialRequest != nil {
		wErr = d.sendCredentialRequest(b[d.w.HeaderSize():])
	} else {
		wErr = d.w.SendManual(
			ETServerConnectRequestCredential,
			b[d.w.HeaderSize():],
		)
	}
	if wErr != nil {
		return nil, wErr
	}

	credentialBytes, credentialOK := <-d.credentialReceive
	if !credentialOK {
		return nil, ErrSSHAuthCancelled
	}

	privateKeyClone := make([]byte, len(credentialBytes))
	copy(privateKeyClone, credentialBytes)
	return privateKeyClone, nil
}

func (d *etClient) cachedPrivateKey() ([]byte, bool) {
	d.privateKeyLock.Lock()
	defer d.privateKeyLock.Unlock()

	if d.privateKey == nil {
		return nil, false
	}

	keyClone := make([]byte, len(d.privateKey))
	copy(keyClone, d.privateKey)
	return keyClone, true
}

func (d *etClient) cachePrivateKey(privateKey []byte) {
	d.privateKeyLock.Lock()
	defer d.privateKeyLock.Unlock()

	privateKeyClone := make([]byte, len(privateKey))
	copy(privateKeyClone, privateKey)
	d.privateKey = privateKeyClone
}

func (d *etClient) privateKeyForET() ([]byte, bool) {
	return d.cachedPrivateKey()
}

func (d *etClient) clearPrivateKey() {
	d.privateKeyLock.Lock()
	defer d.privateKeyLock.Unlock()

	for idx := range d.privateKey {
		d.privateKey[idx] = 0
	}
	d.privateKey = nil
}

func (d *etClient) local(
	f *command.FSM,
	r *rw.LimitedReader,
	h command.StreamHeader,
	b []byte,
) error {
	switch h.Marker() {
	case ETClientStdIn:
		if d.meta != defaultETMetadata() {
			return ErrETRemoteUnavailable
		}
	}

	return ErrETRemoteUnavailable
}

func (d *etClient) Close() error {
	d.processedLock.Lock()
	d.credentialProcessed = true
	d.fingerprintProcessed = true
	d.processedLock.Unlock()

	d.credentialReceiveCloseOnce.Do(func() {
		d.credentialReceiveClosed = true
		close(d.credentialReceive)
	})
	d.fingerprintVerifyResultReceiveCloseOnce.Do(func() {
		d.fingerprintVerifyResultReceiveClosed = true
		close(d.fingerprintVerifyResultReceive)
	})

	d.baseCtxCancel()
	d.clearPrivateKey()
	d.remoteCloseWait.Wait()
	return nil
}

func (d *etClient) Release() error {
	d.clearPrivateKey()
	d.baseCtxCancel()
	return nil
}
