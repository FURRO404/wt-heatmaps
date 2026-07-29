package lux

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/DataDog/zstd"
	"github.com/rs/zerolog"
	"github.com/shamaton/msgpack/v3"
	"golang.org/x/net/websocket"
)

type FetchPreferences struct {
	Maps   []string `json:"maps"`
	UIDs   []string `json:"uids"`
	Groups []string `json:"groups"`
}

func FetchFromLux(log zerolog.Logger, exitChan <-chan struct{}, carvesChan chan<- *LuxCarve, preferences <-chan FetchPreferences, token string) error {
	ws, err := dialLux(log, token)
	if err != nil {
		return fmt.Errorf("dial lux: %w", err)
	}
	shouldCloseWriter := make(chan struct{})
	wsClose := sync.OnceFunc(func() {
		ws.Close()
		close(shouldCloseWriter)
	})
	defer wsClose()

	var wg sync.WaitGroup
	var errWrite, errRead error

	wg.Go(func() {
		defer log.Info().Msg("write pump exited")
		for {
			select {
			case <-exitChan:
				wsClose()
				return
			case <-shouldCloseWriter:
				return
			case v, ok := <-preferences:
				if !ok {
					wsClose()
					return
				}
				errWrite = rawMessageCodec.Send(ws, v)
				if errWrite != nil {
					wsClose()
					return
				}
			}
		}
	})

	wg.Go(func() {
		defer log.Info().Msg("read pump exited")
		msgpack.StructAsArray = false
		msgDecompressed := make([]byte, 0, 20_000_000)
		for {
			var msg []byte
			errRead = rawMessageCodec.Receive(ws, &msg)
			if errRead != nil {
				wsClose()
				return
			}
			msgDecompressed, errRead = zstd.Decompress(msgDecompressed, msg)
			if errRead != nil {
				wsClose()
				return
			}
			var carve LuxCarve
			errRead = msgpack.Unmarshal(msgDecompressed, &carve)
			if errRead != nil {
				wsClose()
				return
			}

			select {
			case carvesChan <- &carve:
			default:
			}
		}
	})

	log.Info().Msg("lux ws open")

	wg.Wait()

	return errors.Join(errWrite, errRead)
}

func dialLux(log zerolog.Logger, token string) (*websocket.Conn, error) {
	wsConfig, err := websocket.NewConfig("wss://wtapi.dev/v1/replays/ws/random", "https://wtapi.dev/")
	if err != nil {
		return nil, err
	}
	wsConfig.Header.Add("Authorization", token)
	wsConfig.TlsConfig = &tls.Config{
		ServerName:         "wtapi.dev",
		InsecureSkipVerify: false,
		MinVersion:         0,
		MaxVersion:         0,
	}
	log.Info().Msg("dialing lux")
	wsConn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", "wtapi.dev:443", wsConfig.TlsConfig.Clone())
	if err != nil {
		return nil, err
	}
	log.Info().Msg("lux connected")
	wsConn.SetDeadline(time.Now().Add(5 * time.Second))
	ws, err := websocket.NewClient(wsConfig, wsConn)
	wsConn.SetDeadline(time.Time{})
	return ws, err
}

var rawMessageCodec = websocket.Codec{
	Marshal: func(v any) (data []byte, payloadType byte, err error) {
		data, err = json.Marshal(v)
		return data, websocket.TextFrame, err
	},
	Unmarshal: func(data []byte, payloadType byte, v any) error {
		*(v.(*[]byte)) = data
		return nil
	},
}
