package client

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hlhv/fsock"
	"github.com/hlhv/protocol"
	"github.com/hlhv/scribe"
	"io"
	"net"
)

type Band struct {
	conn      net.Conn
	reader    *fsock.Reader
	writer    *fsock.Writer
	listening bool
	isGarbage bool
	callback  func(*Band, protocol.FrameKind, []byte)

	stopNotify chan int
}

func spawnBand(
	address string,
	uuid string,
	key string,
	callback func(*Band, protocol.FrameKind, []byte),
	tlsConf *tls.Config,
) (
	band *Band,
	err error,
) {
	scribe.PrintProgress(scribe.LogLevelDebug, "connecting new band")

	scribe.PrintProgress(scribe.LogLevelDebug, "dialing")
	conn, err := tls.Dial("tcp", address, tlsConf)
	if err != nil {
		conn.Close()
		return nil, err
	}

	reader := fsock.NewReader(conn)
	writer := fsock.NewWriter(conn)

	scribe.PrintProgress(scribe.LogLevelDebug, "requesting band status")
	_, err = protocol.WriteMarshalFrame(writer, &protocol.FrameIAm{
		ConnKind: protocol.ConnKindBand,
		Uuid:     uuid,
		Key:      key,
	})
	if err != nil {
		conn.Close()
		return nil, err
	}

	kind, data, err := protocol.ReadParseFrame(reader)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if kind != protocol.FrameKindAccept {
		conn.Close()
		return nil, errors.New(fmt.Sprint(
			"server sent strange response:", kind))
	}

	frame := protocol.FrameAccept{}
	err = json.Unmarshal(data, &frame)
	if err != nil {
		conn.Close()
		return nil, err
	}
	scribe.PrintDone(scribe.LogLevelDebug, "band accepted")

	band = &Band{
		conn:     conn,
		reader:   reader,
		writer:   writer,
		callback: callback,
	}

	go band.listen()
	return band, nil
}

func (band *Band) listen() {
	scribe.PrintInfo(
		scribe.LogLevelDebug,
		"band listening")
	band.listening = true
	defer func() {
		band.listening = false
		band.isGarbage = true
		scribe.PrintInfo(
			scribe.LogLevelDebug,
			"band no longer listening")
	}()

	for {
		kind, data, err := protocol.ReadParseFrame(band.reader)

		if band.stopNotify != nil {
			scribe.PrintInfo(
				scribe.LogLevelDebug,
				"band recieved stop request, replying on",
				"stopNotify")
			band.stopNotify <- 0
			break
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			scribe.PrintError(
				scribe.LogLevelError, "band error:", err)
			break
		}
		if band.callback == nil {
			scribe.PrintError(
				scribe.LogLevelError,
				"band callback not registered")
		} else {
			band.callback(band, kind, data)
		}
	}
}

/* Close closes the connection and marks the band as closed so that it can be
 * removed from the list later.
 */
func (band *Band) Close() {
	// if we aren't listening, we need to exit because there won't be
	// anything to respond to stopNotify.
	if !band.listening {
		return
	}

	scribe.PrintProgress(scribe.LogLevelDebug, "closing band")
	band.stopNotify = make(chan int)
	band.conn.Close()
	<-band.stopNotify
	scribe.PrintDone(scribe.LogLevelDebug, "band closed")
}

/* ReadParseFrame reads a single frame and parses it, separating the kind and
 * the data.
 */
func (band *Band) ReadParseFrame() (
	kind protocol.FrameKind,
	data []byte,
	err error,
) {
	kind, data, err = protocol.ReadParseFrame(band.reader)
	if err != nil {
		band.Close()
	}
	return
}

/* WriteMarshalFrame marshals and writes a Frame.
 */
func (band *Band) WriteMarshalFrame(frame protocol.Frame) (nn int, err error) {
	nn, err = protocol.WriteMarshalFrame(band.writer, frame)
	if err != nil {
		band.Close()
	}
	return
}

/* WriteHTTPHead writes HTTP header information. It should only be called once
 * when serving an HTTP response.
 */
func (band *Band) WriteHTTPHead(
	code int,
	headers map[string][]string,
) (
	nn int,
	err error,
) {
	if headers == nil {
		headers = make(map[string][]string)
	}
	return band.WriteMarshalFrame(&protocol.FrameHTTPResHead{
		StatusCode: code,
		Headers:    headers,
	})
}

/* WriteHTTPBody writes a chunk of the response body.
 */
func (band *Band) WriteHTTPBody(data []byte) (nn int, err error) {
	return band.writer.WriteFrame(
		append(
			[]byte{byte(protocol.FrameKindHTTPResBody)},
			data...,
		),
	)
}

/* writeHTTPEnd ends the HTTP response. This function should be called
 * automatically by the internal callback set by the leash.
 */
func (band *Band) writeHTTPEnd() (nn int, err error) {
	return band.writer.WriteFrame(
		[]byte{byte(protocol.FrameKindHTTPResEnd)},
	)
}

/* AskForHTTPBody requests the http body data from the queen. The queen will
 * return at maximum the amount of data specified with maxSize.
 */
func (band *Band) AskForHTTPBody(maxSize int) (nn int, err error) {
	return band.WriteMarshalFrame(&protocol.FrameHTTPResWant{
		MaxSize: maxSize,
	})
}

/* ReadHTTPBody reads a chunk of the request body. This function returns true
 * for getNext if the chunk was successfully read, and false if it encountered
 * an error or the request ended.
 */
func (band *Band) ReadHTTPBody() (getNext bool, data []byte, err error) {
	getNext = false

	kind, data, err := band.ReadParseFrame()
	if err != nil {
		return false, nil, err
	}

	if kind == protocol.FrameKindHTTPReqBody {
		return true, data, nil
	} else if kind == protocol.FrameKindHTTPReqEnd {
		return false, data, nil
	}

	return false, data, errors.New(fmt.Sprint(
		"got unexpected kind code while processing http req:",
		kind,
	))
}

/* ReadHTTPBodyFull reads all chunks of the request body, and returns the data
 * read as []byte.
 */
func (band *Band) ReadHTTPBodyFull() (body []byte, err error) {
	for {
		getNext, data, err := band.ReadHTTPBody()
		if err != nil {
			return data, err
		}
		body = append(body, data...)
		if !getNext {
			break
		}
	}
	return body, nil
}
