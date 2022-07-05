package client

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hlhv/fsock"
	"github.com/hlhv/protocol"
	"github.com/hlhv/scribe"
	"io"
	"io/ioutil"
	"net"
	"sync"
)

/* Leash represents a connection to the server. Through it, the cell and the
 * server can communicate. Leashes are associated with a number of bands, which
 * are automatically created and destroyed as needed.
 */
type Leash struct {
	queue  chan Req
	uuid   string
	key    string
	conn   net.Conn
	reader *fsock.Reader
	writer *fsock.Writer

	bands      map[*Band]interface{}
	bandsMutex sync.RWMutex

	handles leashHandles
	retry   bool
	tlsConf *tls.Config
}

/* leashHandles stores event handler functions for a leash.
 */
type leashHandles struct {
	onHTTP func(band *Band, head *protocol.FrameHTTPReqHead)
}

/* Mount represents a mount pattern. It has a Host and a Path field.
 */
type Mount struct {
	Host string
	Path string
}

/* NewLeash creates a new leash object. It does not connect the leash to a
 * server, this needs to be done via the Ensure() or Dial() function.
 */
func NewLeash() (leash *Leash) {
	return &Leash{
		queue:  make(chan Req, 16),
		conn:   nil,
		reader: nil,
		writer: nil,
		bands:  make(map[*Band]interface{}),
		retry:  true,
	}
}

/* Dial connects the leash to a server. This function is only useful in some
 * cases, Ensure is usually a better option.
 */
func (leash *Leash) Dial(
	address string,
	key string,
	rootCertPath string,
) (
	err error,
) {
	if leash.conn != nil {
		// we already have a connection, so close it
		leash.Close()
	}

	scribe.PrintProgress(scribe.LogLevelNormal, "connecting new leash")

	if rootCertPath != "" {
		scribe.PrintProgress(scribe.LogLevelDebug, "reading root cert")

		rootPEM, err := ioutil.ReadFile(rootCertPath)
		if err != nil {
			return err
		}

		roots := x509.NewCertPool()
		ok := roots.AppendCertsFromPEM(rootPEM)
		if !ok {
			return errors.New("couldn't parse root cert")
		}

		leash.tlsConf = &tls.Config{
			RootCAs: roots,
		}
	} else {
		scribe.PrintWarning(
			scribe.LogLevelError,
			"WARNING!\nCONTINUING WITHOUT TLS AUTHENTICATION.\n"+
				"THIS SHOULD ONLY BE USED FOR TESTING. DOING THIS\n"+
				"IN A PRODUCTION ENVIRONMENT COULD LEAVE YOUR\n"+
				"SYSTEM OPEN TO ATTACK.")
		leash.tlsConf = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	scribe.PrintProgress(scribe.LogLevelNormal, "dialing")
	conn, err := tls.Dial("tcp", address, leash.tlsConf)
	if err != nil {
		return err
	}

	leash.conn = conn
	leash.reader = fsock.NewReader(leash.conn)
	leash.writer = fsock.NewWriter(leash.conn)

	scribe.PrintProgress(scribe.LogLevelDebug, "requesting cell status")
	// hangs?
	_, err = leash.writeMarshalFrame(&protocol.FrameIAm{
		ConnKind: protocol.ConnKindCell,
		Key:      key,
	})
	if err != nil {
		return err
	}

	kind, data, err := leash.readParseFrame()
	if err != nil {
		return err
	}
	if kind != protocol.FrameKindAccept {
		leash.conn.Close()
		return errors.New(fmt.Sprint(
			"server sent strange response:", kind))
	}

	frame := protocol.FrameAccept{}
	err = json.Unmarshal(data, &frame)
	if err != nil {
		return err
	}

	leash.uuid = frame.Uuid
	leash.key = frame.Key
	scribe.PrintDone(
		scribe.LogLevelNormal, "leash accepted, uuid is", leash.uuid)

	go leash.respond()
	return nil
}

/* Close closes the leash, and all bands in it. If the connection is ensured,
 * this will just re-connect afterwards. To stop this from happening, call the
 * Stop() method instead.
 */
func (leash *Leash) Close() {
	leash.conn.Close()

	leash.bandsMutex.RLock()
	defer leash.bandsMutex.RUnlock()
	for band := range leash.bands {
		band.Close()
	}
}

/* Stop closes the leash, and all bands in it, preventing the leash from
 * reconnecting if it is ensured.
 */
func (leash *Leash) Stop() {
	leash.retry = false
	leash.Close()
}

/* cleanBands Removes references to closed bands so that they can be garbage
 * collected. This should run every so often, but it doesn't need to be run a
 * whole lot. Currently it is run every time a new band is created.
 */
func (leash *Leash) cleanBands() {
	leash.bandsMutex.Lock()
	defer leash.bandsMutex.Unlock()

	for band := range leash.bands {
		if !band.open {
			delete(leash.bands, band)
		}
	}
}

/* NewBand Creates a new band specifically for this leash, and adds it to the
 * list of bands.
 */
func (leash *Leash) NewBand() (err error) {
	// create and add band
	band, err := spawnBand(
		leash.conn.RemoteAddr().String(),
		leash.uuid,
		leash.key,
		leash.handleBandFrame,
		leash.tlsConf,
	)

	leash.bandsMutex.Lock()
	leash.bands[band] = nil
	leash.bandsMutex.Unlock()

	// we need to run this every so often, might as well be here
	leash.cleanBands()
	return err
}

/* Listen listens for data sent over the leash.
 */
func (leash *Leash) Listen() (err error) {
	for {
		var kind protocol.FrameKind
		var data []byte
		kind, data, err = protocol.ReadParseFrame(leash.reader)
		scribe.PrintRequest(
			scribe.LogLevelDebug, "received command over leash")

		if err == io.EOF {
			break
		}
		if err != nil {
			scribe.PrintError(
				scribe.LogLevelError, "leash error:", err)
		}

		leash.handleFrame(kind, data)
	}
	scribe.PrintDisconnect(
		scribe.LogLevelNormal, "disconnected")
	return err
}

/* handleFrame handles a frame sent over the leash.
 */
func (leash *Leash) handleFrame(kind protocol.FrameKind, data []byte) {
	switch kind {
	case protocol.FrameKindNeedBand:
		scribe.PrintInfo(scribe.LogLevelDebug, "server needs new band")
		err := leash.NewBand()
		if err != nil {
			scribe.PrintError(
				scribe.LogLevelError, "cant add band:", err)
		}
		break
	}
}

/* handleBandFrame handles an incoming server request over a band.
 */
func (leash *Leash) handleBandFrame(
	band *Band,
	kind protocol.FrameKind,
	data []byte,
) {
	switch kind {
	case protocol.FrameKindHTTPReqHead:
		frame := &protocol.FrameHTTPReqHead{}
		json.Unmarshal(data, frame)
		scribe.PrintRequest(
			scribe.LogLevelNormal,
			"request for \""+frame.Host+frame.Path+"\"",
			"by", frame.RemoteAddr)
		leash.handles.onHTTP(band, frame)
		band.writeHTTPEnd()
		break
	}
}

/* ReadParseFrame reads a single frame and parses it, separating the kind and
 * the data.
 */
func (leash *Leash) readParseFrame() (
	kind protocol.FrameKind,
	data []byte,
	err error,
) {
	return protocol.ReadParseFrame(leash.reader)
}

/* WriteMarshalFrame marshals and writes a Frame.
 */
func (leash *Leash) writeMarshalFrame(
	frame protocol.Frame,
) (
	nn int,
	err error,
) {
	return protocol.WriteMarshalFrame(leash.writer, frame)
}
