package cell

import (
        "github.com/hlhv/cell/client"
)

/* HTTPResponse stores information about an HTTP response, and has function for
 * writing its response body
 */
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
