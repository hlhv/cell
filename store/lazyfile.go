package store

import (
	"github.com/hlhv/cell/client"
	"github.com/hlhv/protocol"
	"github.com/hlhv/scribe"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

/* chunkSize does not refer to actual chunked encoding. This is just so the
 * client doesn't have to wait for the cell to send everything over and the
 * queen to send that over before receiving everything. It should be at least
 * 512 in order for accurate mime-type detection.
 */
const chunkSize int = 1024

/* LazyFile is a struct capable of serving a file. The file is cached into
 * memory when it is first loaded, hence the name.
 */
type LazyFile struct {
	FilePath   string
	AutoReload bool

	mime      string
	chunks    []fileChunk
	timestamp time.Time

	totalSize       int64
	totalSizeString string
}

type fileChunk []byte

/* Send sends the file along with a content-type header.
 */
func (item *LazyFile) Send(
	band *client.Band,
	head *protocol.FrameHTTPReqHead,
	maxAge time.Duration,
) (
	err error,
) {
	scribe.PrintProgress(scribe.LogLevelDebug, "sending file")
	if item.AutoReload {
		// check to see if file needs to be reloaded
		newTimestamp, err := item.getCurrentTimestamp()
		if err != nil {
			return err
		}

		if newTimestamp.After(item.timestamp) {
			item.timestamp = newTimestamp
			item.chunks = nil
		}
	}

	if item.chunks == nil {
		err = item.loadAndSend(band, head, maxAge)
		return err
	}

	err = item.sendHeaders(band, maxAge)
	if err != nil {
		return err
	}

	for _, chunk := range item.chunks {
		_, err = band.WriteHTTPBody(chunk)
		if err != nil {
			return err
		}
	}

	scribe.PrintDone(scribe.LogLevelDebug, "file sent")
	return nil
}

/* sendHeaders creates builds and sends applicable HTTP headers.
 */
func (item *LazyFile) sendHeaders(
	band *client.Band,
	maxAge time.Duration,
) (
	err error,
) {
	headers := map[string][]string{
		"content-type":   {item.mime},
		"content-length": {item.totalSizeString},
	}

	if maxAge > 0 && item.mime != "text/html" {
		headers["cache-control"] = []string{
			"max-age=" +
				strconv.Itoa(int(maxAge.Seconds())),
		}
	}

	_, err = band.WriteHTTPHead(200, headers)
	return
}

/* getCurrentTimestamp returns the current timestamp of the file on disk.
 */
func (item *LazyFile) getCurrentTimestamp() (timestamp time.Time, err error) {
	fileInfo, err := os.Stat(item.FilePath)
	if err != nil {
		return time.Time{}, err
	}
	return fileInfo.ModTime(), nil
}

/* loadAndSend loads the file from disk while sending it in response to an http
 * request. This should be called when there is an http request for this file
 * but it has not been loaded yet.
 */
func (item *LazyFile) loadAndSend(
	band *client.Band,
	head *protocol.FrameHTTPReqHead,
	maxAge time.Duration,
) (
	err error,
) {
	scribe.PrintProgress(scribe.LogLevelDebug, "loading and sending file")
	file, err := os.Open(item.FilePath)
	defer file.Close()
	if err != nil {
		return err
	}

	// get file size
	fileInformation, err := file.Stat()
	if err != nil {
		return err
	}
	item.totalSize = fileInformation.Size()
	item.totalSizeString = strconv.FormatInt(item.totalSize, 10)

	needMime := true
	for {
		chunk := make([]byte, chunkSize)
		bytesRead, err := io.ReadFull(file, chunk)
		chunk = chunk[:bytesRead]

		fileEnded := err == io.ErrUnexpectedEOF || err == io.EOF
		if err != nil && !fileEnded {
			return err
		}

		if needMime {
			needMime = false
			item.mime = mimeSniff(item.FilePath, chunk)

			err = item.sendHeaders(band, maxAge)
			if err != nil {
				return err
			}
		}

		item.chunks = append(item.chunks, chunk)
		band.WriteHTTPBody(chunk)

		if fileEnded {
			break
		}
	}

	scribe.PrintDone(scribe.LogLevelDebug, "file loaded and sent")
	return nil
}

/* mimeSniff determines the content type of a byte array and an associated name.
 * This isn't very good as of now but it works!
 */
func mimeSniff(name string, data []byte) (mime string) {
	extension := filepath.Ext(name)
	mime = http.DetectContentType(data)

	// go's mime type sniffer will return text/plain when it sees plain
	// text, and we only want that if the file is actually a text file.
	wrongType := strings.HasPrefix(mime, "text/plain") &&
		extension != ".txt" &&
		extension != ""

	if wrongType {
		// check for cases where the file is detected as text but does
		// not have a mime type that falls under "text/"
		switch extension {
		case ".svg":
			return "image/svg+xml"
		case ".js":
			return "application/javascript"

		// normal case
		default:
			return strings.Replace(mime, "plain", extension[1:], 1)
		}
	}

	scribe.PrintInfo(scribe.LogLevelDebug, "file has mimetype of "+mime)
	return mime
}
