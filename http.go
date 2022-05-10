package cell

import (
        "github.com/hlhv/protocol"
        "github.com/hlhv/cell/client"
)

type HTTPRequest struct {
        Head         *protocol.FrameHTTPReqHead
        band         *client.Band
        askedForBody bool
        maxBodySize  int
}

/* SetMaxBodySize sets the maximum size for the request body to be sent to the
 * cell. Defaults to 8192 bytes. This function should usually be called before
 * reading the request body.
 */
func (request *HTTPRequest) SetMaxBodySize (maxSize int) {
        request.maxBodySize = maxSize
}

/* ensureBodyRequested determines if the body needs to be asked for from the
 * queen. If it does, it ensures that the maximum body size is set, and then
 * sends the request.
 */
func (request *HTTPRequest) ensureBodyRequested () (err error) {
        if !request.askedForBody {
                if request.maxBodySize == 0 {
                        request.maxBodySize = 8192
                }
                _, err = request.band.AskForHTTPBody(request.maxBodySize)
                if err != nil { return }
                request.askedForBody = true
        }

        return
}

/* ReadBody reads a chunk of the request body. This function returns true for
 * getNext if the chunk was successfully read, and false if it encountered an
 * error or the request ended.
 */
func (request *HTTPRequest) ReadBody () (getNext bool, data []byte, err error) {
        err = request.ensureBodyRequested()
        if err != nil { return }

        return request.band.ReadHTTPBody()
}

/* ReadHTTPBodyFull reads all chunks of the request body, and returns the data
 * read as []byte.
 */
func (request *HTTPRequest) ReadBodyFull () (data []byte, err error) {
        err = request.ensureBodyRequested()
        if err != nil { return }

        return request.band.ReadHTTPBodyFull()
}

type HTTPResponse struct {
        band *client.Band
}

/* WriteHead writes HTTP header information. It should only be called once when
 * serving an HTTP response. Passing nil for headers will send no headers.
 */
func (response *HTTPResponse) WriteHead (
        code int,
        headers map[string] []string,
) (
        err error,
) {
        _, err = response.band.WriteHTTPHead(code, headers)
        return
}

/* WriteBody writes a chunk of the response body.
 */
func (response *HTTPResponse) WriteBody (data []byte) (err error) {
        _, err = response.band.WriteHTTPBody(data)
        return
}
