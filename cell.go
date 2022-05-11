package cell

import (
        "os"
        "fmt"
        "time"
        "github.com/hlhv/scribe"
        "github.com/hlhv/protocol"
        "github.com/hlhv/cell/store"
        "github.com/hlhv/cell/client"
        "github.com/akamensky/argparse"
)

type HTTPReqHead  protocol.FrameHTTPReqHead

type Cell struct {
        leash         *client.Leash
        store         *store.Store
        logLevel      scribe.LogLevel

        Description   string
        MountPoint    Mount
        DataDirectory string
        QueenAddress  string
        Key           string
        RootCertPath  string
        
        OnHTTP        func (response *HTTPResponse, request *HTTPRequest)
}

/* Mount represents a mount pattern. It has a Host and a Path field.
 */
type Mount client.Mount

func (cell *Cell) Run () {
        cell.parseArgs()
        cell.leash = client.NewLeash()
        cell.leash.OnHTTP(cell.onHTTP)
        cell.store = store.New(cell.DataDirectory)
        
        go cell.ensure()
        for {
                scribe.ListenOnce()
        }
}

func (cell *Cell) onHTTP (band *client.Band, head *protocol.FrameHTTPReqHead) {
        handled, err := cell.store.TryHandle(band, head)
        // TODO: respond with error
        if err != nil { return }
        if handled { return }
        
        response := &HTTPResponse {
                band: band,
        }

        request := &HTTPRequest {
                band: band,
                Head: head,
        }
        
        cell.OnHTTP(response, request)
}

func (cell *Cell) parseArgs () {
        parser := argparse.NewParser ("", cell.Description)
        logLevel := parser.Selector ("l", "log-level", []string {
                "debug",
                "normal",
                "error",
                "none",
        }, &argparse.Options {
                Required: false,
                Default:  "normal",
                Help:     "The amount of logs to produce. Debug prints " +
                          "everything, and none prints nothing",
        })

        err := parser.Parse(os.Args)
        if err != nil {
                fmt.Print(parser.Usage(err))
                os.Exit(1)
        }

        switch *logLevel {
                case "debug":  cell.logLevel = scribe.LogLevelDebug;  break
                default:
                case "normal": cell.logLevel = scribe.LogLevelNormal; break
                case "error":  cell.logLevel = scribe.LogLevelError;  break
                case "none":   cell.logLevel = scribe.LogLevelNone;   break
        }
}

func (cell *Cell) ensure () {
        var retryTime time.Duration = 3
        for {
                worked, err := cell.ensureOnce ()
                if err != nil {
                        scribe.PrintError (
                                scribe.LogLevelError, "connection error:", err)
                }
                if worked {
                        retryTime = 2
                } else if retryTime < 60 {
                        retryTime = (retryTime * 3) / 2
                }
                
                scribe.PrintInfo (
                        scribe.LogLevelNormal,
                        "disconnected. retrying in", retryTime)
                time.Sleep(retryTime * time.Second)
        }
}

func (cell *Cell) ensureOnce () (worked bool, err error) {
        err = cell.leash.Dial(cell.QueenAddress, cell.Key, cell.RootCertPath)
        if err != nil { return false, err }

        err = cell.leash.Mount(cell.MountPoint.Host, cell.MountPoint.Path)
        if err != nil { return true, err }

        scribe.PrintDone(scribe.LogLevelNormal, "mounted")

        return true, cell.leash.Listen()
}
