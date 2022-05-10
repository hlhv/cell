# cell

Cell is a module that facilitates the creation of HLHV cells. In the future,
this module will contain many convenience functions to make creating a cell
easier.

## Creating a Cell

Compared to the previous way of creating cells, this way involves far less
boilerplate. A very basic cell:

```
package main

import "github.com/hlhv/cell"

func main () {
        // configure cell
        thisCell := &cell.Cell {
                Description:   "Example cell",
                MountPoint:    cell.Mount { "@", "/" },
                DataDirectory: "/var/hlhv/cells/example/",
                QueenAddress:  "localhost:2001",
                Key:           "example key",

                OnHTTP:        onHTTP,
        }

        // run cell
        thisCell.Be()
}

func onHTTP (band *cell.Band, head *cell.HTTPReqHead) {
        // passing nil writes no headers
        band.WriteHTTPHead(200, nil)
        
        // write response body
        band.WriteHTTPBody([]byte("hello, world!"))
}
```

Note: Running two cells within the same program will cause issues. This may be
fixed in the future, however doing this is generally a bad idea and defeats the
purpose of cells.
