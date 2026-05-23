// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Snuffy2/shellport/application/command"
	"github.com/Snuffy2/shellport/application/configuration"
	"github.com/Snuffy2/shellport/application/log"
	"github.com/Snuffy2/shellport/application/rw"
)

// Errors returned by the socket controller during WebSocket upgrade and
// message processing.
var (
	// ErrSocketInvalidAuthKey is returned when the client omits the X-Key
	// header but a user password is configured, indicating authentication is
	// required.
	ErrSocketInvalidAuthKey = NewError(
		http.StatusForbidden,
		"To use Websocket interface, a valid Auth Key must be provided")

	// ErrSocketAuthFailed is returned when the X-Key value provided by the
	// client does not match the time-windowed HMAC derived from the user password.
	ErrSocketAuthFailed = NewError(
		http.StatusForbidden,
		"Authentication has failed with provided Auth Key")

	// ErrSocketUnableToGenerateKey is returned when the server cannot generate
	// a cryptographic key due to an entropy failure.
	ErrSocketUnableToGenerateKey = NewError(
		http.StatusInternalServerError,
		"Unable to generate crypto key")

	// ErrSocketInvalidDataPackage is returned when a received WebSocket frame
	// carries an out-of-range or otherwise malformed length prefix.
	ErrSocketInvalidDataPackage = NewError(
		http.StatusBadRequest, "Invalid data package")
)

const (
	// socketGCMStandardNonceSize is the byte length of the nonce used with
	// AES-GCM for encrypting and authenticating WebSocket data frames.
	socketGCMStandardNonceSize = 12
)

// socket is the controller for the "/shellport/socket" WebSocket endpoint. It
// upgrades HTTP connections to WebSocket, performs AES-GCM-based handshake
// authentication, and then hands the framed connection to the command layer for
// proxying SSH and other protocol traffic.
type socket struct {
	baseController

	commonCfg        configuration.Common
	serverCfg        configuration.Server
	upgrader         websocket.Upgrader
	commander        command.Commander
	hks              command.Hooks
	socketBufferPool *command.BufferPool
}

// hashCombineSocketKeys computes an HMAC-SHA-512 digest of addedKey using
// privateKey as the HMAC secret. It is used to derive session keys and
// verification tokens from a combination of user-supplied and server-side
// secret material.
func hashCombineSocketKeys(addedKey string, privateKey string) []byte {
	h := hmac.New(sha512.New, []byte(privateKey))

	h.Write([]byte(addedKey))

	return h.Sum(nil)
}

// newSocketCtl constructs a socket controller initialized with the given common
// and server configurations, command set, lifecycle hooks, and a pre-allocated
// buffer pool for encrypting WebSocket frames.
func newSocketCtl(
	commonCfg configuration.Common,
	cfg configuration.Server,
	cmds command.Commands,
	hooks command.Hooks,
	socketBufferPool *command.BufferPool,
) socket {
	return socket{
		commonCfg:        commonCfg,
		serverCfg:        cfg,
		upgrader:         buildWebsocketUpgrader(cfg),
		commander:        command.New(cmds),
		hks:              hooks,
		socketBufferPool: socketBufferPool,
	}
}

// websocketWriter wraps a *websocket.Conn and adapts it to the io.Writer
// interface by sending every Write call as a single binary WebSocket message.
type websocketWriter struct {
	*websocket.Conn
	writeTimeout     time.Duration
	now              func() time.Time
	writeMessage     func(int, []byte) error
	setWriteDeadline func(time.Time) error
}

// websocketLivenessConn captures only the websocket behaviors needed to run heartbeat
// and timeout handlers during socket lifetime.
type websocketLivenessConn interface {
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
	SetPongHandler(func(string) error)
	WriteMessage(int, []byte) error
	Close() error
}

// websocketTicker abstracts the ticker interface used for heartbeat scheduling.
type websocketTicker interface {
	C() <-chan time.Time
	Stop()
}

type websocketTickerImpl struct {
	*time.Ticker
}

func (w websocketTickerImpl) C() <-chan time.Time {
	return w.Ticker.C
}

func websocketTickerDefaultFactory(interval time.Duration) websocketTicker {
	return websocketTickerImpl{Ticker: time.NewTicker(interval)}
}

// newWebsocketWriter initializes a deadline-aware websocketWriter.
func newWebsocketWriter(
	conn *websocket.Conn,
	writeTimeout time.Duration,
) websocketWriter {
	return websocketWriter{
		Conn:             conn,
		writeTimeout:     writeTimeout,
		now:              time.Now,
		writeMessage:     conn.WriteMessage,
		setWriteDeadline: conn.SetWriteDeadline,
	}
}

// Write sends b as a binary WebSocket message. It sets a write deadline when
// configured, returns the number of bytes written, and returns error when the
// underlying write fails.
func (w websocketWriter) Write(b []byte) (int, error) {
	wErr := w.writeMessageWithDeadline(websocket.BinaryMessage, b)

	if wErr != nil {
		return 0, wErr
	}

	return len(b), nil
}

func (w websocketWriter) writeMessageWithDeadline(
	messageType int,
	message []byte,
) error {
	if w.writeTimeout > 0 && w.setWriteDeadline != nil {
		if nowFn := w.now; nowFn == nil {
			if wErr := w.setWriteDeadline(time.Now().Add(w.writeTimeout)); wErr != nil {
				return wErr
			}
		} else {
			if wErr := w.setWriteDeadline(nowFn().Add(w.writeTimeout)); wErr != nil {
				return wErr
			}
		}
	}

	if w.writeMessage == nil {
		return fmt.Errorf("websocket writer write callback is not configured")
	}

	return w.writeMessage(messageType, message)
}

// socketPackageWriter is an io.Writer that fragments and encrypts data through
// a caller-supplied packager function before forwarding it to the underlying
// websocketWriter. It is used to apply AES-GCM framing to outbound WebSocket
// messages.
type socketPackageWriter struct {
	w        websocketWriter
	packager func(w websocketWriter, b []byte) error
}

// Write passes b through the packager function and reports the number of bytes
// consumed. It returns (0, err) if the packager encounters an error.
func (s socketPackageWriter) Write(b []byte) (int, error) {
	packageWriteErr := s.packager(s.w, b)

	if packageWriteErr != nil {
		return 0, packageWriteErr
	}

	return len(b), nil
}

// buildWebsocketUpgrader constructs a websocket.Upgrader configured with the
// handshake timeout from cfg. The origin check always returns true, allowing
// cross-origin WebSocket connections. Upgrade errors are silently swallowed by
// the Error hook to avoid double-writing to the response.
func buildWebsocketUpgrader(cfg configuration.Server) websocket.Upgrader {
	return websocket.Upgrader{
		HandshakeTimeout: cfg.InitialTimeout,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		Error: func(
			w http.ResponseWriter,
			r *http.Request,
			status int,
			reason error,
		) {
		},
	}
}

// Options handles HTTP OPTIONS requests for the socket endpoint by setting the
// CORS headers required to allow cross-origin WebSocket upgrade negotiation.
func (s socket) Options(
	w *ResponseWriter, r *http.Request, l log.Logger) error {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "X-Key")
	return nil
}

// buildWSFetcher returns a FetchReaderFetcher that reads the next binary
// WebSocket message from c. It loops past any non-binary frames and returns
// an error if ReadMessage fails or a non-binary message type is received.
func (s socket) buildWSFetcher(c *websocket.Conn) rw.FetchReaderFetcher {
	return func() ([]byte, error) {
		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				return nil, err
			}
			if mt != websocket.BinaryMessage {
				return nil, NewError(
					http.StatusBadRequest,
					fmt.Sprintf("Received unknown type of data: %d", mt))
			}
			return message, nil
		}
	}
}

// configureWebsocketLiveness configures read deadlines, pong handlers, and ping
// heartbeats for the WebSocket connection. It returns a stop function that should
// be called on request exit.
func (s socket) configureWebsocketLiveness(
	conn websocketLivenessConn,
	nowFn func() time.Time,
	tickerFn func(time.Duration) websocketTicker,
	writeMu *sync.Mutex,
) (func(), error) {
	if nowFn == nil {
		nowFn = time.Now
	}
	if tickerFn == nil {
		tickerFn = websocketTickerDefaultFactory
	}

	if s.serverCfg.ReadTimeout > 0 {
		if setErr := conn.SetReadDeadline(nowFn().Add(s.serverCfg.ReadTimeout)); setErr != nil {
			return func() {}, setErr
		}
	}

	conn.SetPongHandler(func(_ string) error {
		if s.serverCfg.ReadTimeout <= 0 {
			return nil
		}
		return conn.SetReadDeadline(nowFn().Add(s.serverCfg.ReadTimeout))
	})

	if s.serverCfg.HeartbeatTimeout <= 0 {
		return func() {}, nil
	}

	done := make(chan struct{})
	stopped := make(chan struct{})
	ticker := tickerFn(s.serverCfg.HeartbeatTimeout)
	stop := sync.Once{}
	var stopTicker sync.Once
	stopTickerFn := func() {
		stopTicker.Do(func() {
			ticker.Stop()
		})
	}
	stopFn := func() {
		stop.Do(func() {
			close(done)
			stopTickerFn()
			_ = conn.Close()
		})
		<-stopped
	}

	go func() {
		defer close(stopped)
		for {
			select {
			case <-done:
				return
			case <-ticker.C():
				if writeMu != nil {
					writeMu.Lock()
				}
				var doneErr error
				if s.serverCfg.WriteTimeout > 0 {
					if setErr := conn.SetWriteDeadline(nowFn().Add(s.serverCfg.WriteTimeout)); setErr != nil {
						doneErr = setErr
					}
				}
				if doneErr == nil {
					doneErr = conn.WriteMessage(websocket.PingMessage, nil)
				}
				if writeMu != nil {
					writeMu.Unlock()
				}
				if doneErr != nil {
					_ = conn.Close()
					stopTickerFn()
					return
				}
			}
		}
	}()

	return stopFn, nil
}

// generateNonce fills nonce[:socketGCMStandardNonceSize] with cryptographically
// random bytes using crypto/rand. It returns an error if the read fails.
func (s socket) generateNonce(nonce []byte) error {
	_, rErr := io.ReadFull(rand.Reader, nonce[:socketGCMStandardNonceSize])
	return rErr
}

// increaseNonce increments the big-endian counter stored in nonce by one,
// carrying over into higher-order bytes as needed. This advances the AES-GCM
// nonce so that each encrypted frame uses a unique nonce without requiring
// additional random bytes.
func (s socket) increaseNonce(nonce []byte) {
	for i := len(nonce); i > 0; i-- {
		nonce[i-1]++
		if nonce[i-1] <= 0 {
			continue
		}
		break
	}
}

func (s socket) writeServerNonce(
	writeMu *sync.Mutex,
	w websocketWriter,
	nonce []byte,
) error {
	if writeMu != nil {
		writeMu.Lock()
		defer writeMu.Unlock()
	}

	if _, writeErr := w.Write(nonce); writeErr != nil {
		return writeErr
	}

	return nil
}

// createCipher builds two independent AES-GCM AEAD instances—one for reading
// and one for writing—from the same key material. Separate instances are used
// so that the read and write nonce counters can advance independently. It
// returns (readAEAD, writeAEAD, nil) on success, or (nil, nil, err) if any
// AES or GCM initialization step fails.
func (s socket) createCipher(key []byte) (cipher.AEAD, cipher.AEAD, error) {
	readCipher, readCipherErr := aes.NewCipher(key)
	if readCipherErr != nil {
		return nil, nil, readCipherErr
	}

	writeCipher, writeCipherErr := aes.NewCipher(key)
	if writeCipherErr != nil {
		return nil, nil, writeCipherErr
	}

	gcmRead, gcmReadErr := cipher.NewGCMWithNonceSize(
		readCipher, socketGCMStandardNonceSize)
	if gcmReadErr != nil {
		return nil, nil, gcmReadErr
	}

	gcmWrite, gcmWriteErr := cipher.NewGCMWithNonceSize(
		writeCipher, socketGCMStandardNonceSize)
	if gcmWriteErr != nil {
		return nil, nil, gcmWriteErr
	}

	return gcmRead, gcmWrite, nil
}

// mixerKey derives a per-request mixer value by hashing the client's
// User-Agent against a combination of the user password and configured hostname.
// The result is used as a component of the cipher key and as the "X-Key"
// response value sent back to the client during socket verification.
func (s socket) mixerKey(r *http.Request) []byte {
	return hashCombineSocketKeys(
		r.UserAgent(), s.commonCfg.UserPassword+"+"+s.commonCfg.HostName)
}

// keyTimeTruncater is the divisor applied to the Unix timestamp before it is
// incorporated into the cipher key, creating a time window of approximately
// 100 seconds during which the same key is valid.
const keyTimeTruncater = 100

// buildCipherKey derives a 16-byte AES key for the current request by hashing
// a truncated Unix timestamp against the mixer key and the user password. The
// time truncation means the key rotates every keyTimeTruncater seconds,
// limiting the window in which a captured key remains useful.
func (s socket) buildCipherKey(r *http.Request) [16]byte {
	key := [16]byte{}
	copy(key[:], hashCombineSocketKeys(
		strconv.FormatInt(time.Now().Unix()/keyTimeTruncater, 10),
		string(s.mixerKey(r))+"+"+s.commonCfg.UserPassword,
	))
	return key
}

// Get handles HTTP GET requests by upgrading the connection to WebSocket,
// performing the AES-GCM nonce exchange and key derivation, and then running
// the command executor loop that proxies SSH and other protocol traffic. It
// returns a controller Error if the upgrade, nonce exchange, cipher setup, or
// command initialization fails. Once the command loop is running, errors are
// handled internally and this method returns the result of cmdExec.Handle.
func (s socket) Get(
	w *ResponseWriter, r *http.Request, l log.Logger) error {
	// Error will not be returned when Websocket already handled
	// (i.e. returned the error to client). We just log the error and that's it
	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return NewError(http.StatusBadRequest, err.Error())
	}
	defer w.disable()
	senderLock := sync.Mutex{}
	wsWriter := newWebsocketWriter(
		c,
		s.serverCfg.WriteTimeout,
	)
	stopHeartbeat, heartbeatErr := s.configureWebsocketLiveness(
		c,
		time.Now,
		websocketTickerDefaultFactory,
		&senderLock,
	)
	if heartbeatErr != nil {
		_ = c.Close()
		return NewError(http.StatusBadRequest, heartbeatErr.Error())
	}
	defer stopHeartbeat()
	defer c.Close()

	wsReader := rw.NewFetchReader(s.buildWSFetcher(c))

	// Initialize ciphers
	//
	// WARNING: The AES-GCM cipher is here for authenticating user login. Yeah
	//          it is overkill and probably not correct. But I eventually decide
	//          to keep it as long as it authenticates (Hopefully in a safe and
	//          secured way).
	//
	//          DO NOT rely on this if you want to secured communitcation, in
	//          that case, you need to use HTTPS.
	//
	readNonce := [socketGCMStandardNonceSize]byte{}
	_, nonceReadErr := io.ReadFull(&wsReader, readNonce[:])
	if nonceReadErr != nil {
		return NewError(http.StatusBadRequest, fmt.Sprintf(
			"Unable to read initial client nonce: %s", nonceReadErr.Error()))
	}

	writeNonce := [socketGCMStandardNonceSize]byte{}
	nonceReadErr = s.generateNonce(writeNonce[:])
	if nonceReadErr != nil {
		return NewError(http.StatusBadRequest, fmt.Sprintf(
			"Unable to generate initial server nonce: %s",
			nonceReadErr.Error()))
	}

	if nonceSendErr := s.writeServerNonce(
		&senderLock,
		wsWriter,
		writeNonce[:],
	); nonceSendErr != nil {
		return NewError(http.StatusBadRequest, fmt.Sprintf(
			"Unable to send server nonce to client: %s", nonceSendErr.Error()))
	}

	cipherKey := s.buildCipherKey(r)
	readCipher, writeCipher, cipherCreationErr := s.createCipher(cipherKey[:])
	if cipherCreationErr != nil {
		return NewError(http.StatusInternalServerError, fmt.Sprintf(
			"Unable to create cipher: %s", cipherCreationErr.Error()))
	}

	// Start service
	const cipherReadBufSize = 4096

	cipherReadBuf := [cipherReadBufSize]byte{}
	cipherWriteBuf := [cipherReadBufSize]byte{}
	maxWriteLen := int(cipherReadBufSize) - (writeCipher.Overhead() + 2)

	cmdExec, cmdExecErr := s.commander.New(
		command.Configuration{
			Dial:                   s.commonCfg.Dialer,
			DialTimeout:            s.commonCfg.DecideDialTimeout(s.serverCfg.ReadTimeout),
			Socks5Configured:       s.commonCfg.Socks5Configured,
			Presets:                s.commonCfg.CurrentPresets(),
			PresetRepository:       s.commonCfg.PresetRepository,
			OnlyAllowPresetRemotes: s.commonCfg.OnlyAllowPresetRemotes,
		},
		rw.NewFetchReader(func() ([]byte, error) {
			defer s.increaseNonce(readNonce[:])
			// Size is unencrypted
			_, rErr := io.ReadFull(&wsReader, cipherReadBuf[:2])
			if rErr != nil {
				return nil, rErr
			}
			// Read full size
			packageSize := uint16(cipherReadBuf[0])
			packageSize <<= 8
			packageSize |= uint16(cipherReadBuf[1])
			if packageSize <= 0 || packageSize > cipherReadBufSize {
				return nil, ErrSocketInvalidDataPackage
			}
			if int(packageSize) <= wsReader.Remain() {
				rData, rErr := wsReader.Export(int(packageSize))
				if rErr != nil {
					return nil, rErr
				}
				return readCipher.Open(
					cipherReadBuf[:0], readNonce[:], rData, nil)
			}
			_, rErr = io.ReadFull(&wsReader, cipherReadBuf[:packageSize])
			if rErr != nil {
				return nil, rErr
			}
			return readCipher.Open(
				cipherReadBuf[:0],
				readNonce[:],
				cipherReadBuf[:packageSize],
				nil)
		}),
		socketPackageWriter{
			w: wsWriter,
			packager: func(w websocketWriter, b []byte) error {
				start := 0
				bLen := len(b)
				readLen := bLen

				for start < bLen {
					if readLen > maxWriteLen {
						readLen = maxWriteLen
					}
					encrypted := writeCipher.Seal(
						cipherWriteBuf[2:2],
						writeNonce[:],
						b[start:start+readLen],
						nil)
					s.increaseNonce(writeNonce[:])
					encryptedSize := uint16(len(encrypted))
					if encryptedSize <= 0 {
						return ErrSocketInvalidDataPackage
					}
					cipherWriteBuf[0] = byte(encryptedSize >> 8)
					cipherWriteBuf[1] = byte(encryptedSize)
					_, wErr := w.Write(cipherWriteBuf[:encryptedSize+2])
					if wErr != nil {
						return wErr
					}
					start += readLen
					readLen = bLen - start
				}
				return nil
			},
		},
		&senderLock,
		s.serverCfg.ReadDelay,
		s.serverCfg.WriteDelay,
		l,
		s.hks,
		s.socketBufferPool,
	)
	if cmdExecErr != nil {
		return NewError(http.StatusBadRequest, cmdExecErr.Error())
	}
	return cmdExec.Handle()
}
