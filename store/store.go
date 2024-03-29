package store

import (
	"errors"
	"github.com/hlhv/cell/client"
	"github.com/hlhv/protocol"
	"github.com/hlhv/scribe"
	"path/filepath"
	"time"
)

/* Store is a simple resource manager for serving static file resources. Files
 * can be registered and unregistered dynamically, and are loaded lazily. It can
 * be combined with any other system for serving files and pages.
 */

// TODO: store separate map containing registered LazyDirs, and do a separate
// check after stripping the basename from the filepath, then if match send
// original filepath to the matched LazyDir.
type Store struct {
	lazyFiles map[string]*LazyFile
	lazyDirs  map[string]*LazyDir
	root      string
	maxAge    time.Duration
}

/* New creates a new Store.
 */
func New(root string) (store *Store) {
	lastIndex := len(root) - 1
	if root[lastIndex] == '/' {
		root = root[:lastIndex]
	}
	return &Store{
		lazyFiles: make(map[string]*LazyFile),
		lazyDirs:  make(map[string]*LazyDir),
		root:      root,
		maxAge:    time.Hour * 4,
	}
}

/* RegisterFile registers a file located at the filepath on the specific url
 * path.
 */
func (store *Store) RegisterFile(
	filePath string,
	webPath string,
	autoReload bool,
) (
	err error,
) {
	if filePath[0] != '/' {
		filePath = "/" + filePath
	}
	if webPath[0] != '/' {
		webPath = "/" + webPath
	}

	filePath = store.root + filePath

	store.lazyFiles[webPath] = &LazyFile{
		FilePath:   filePath,
		AutoReload: autoReload,
	}

	scribe.PrintInfo(
		scribe.LogLevelDebug,
		"registered file", filePath, "on", webPath)
	return nil
}

/* RegisterDir registers a directory located at the directory path on the
 * specific url path.
 */
func (store *Store) RegisterDir(
	dirPath string,
	webPath string,
	active bool,
) (
	err error,
) {
	if dirPath[0] != '/' {
		dirPath = "/" + dirPath
	}
	if webPath[0] != '/' {
		webPath = "/" + webPath
	}

	if dirPath[len(dirPath)-1] != '/' {
		dirPath += "/"
	}
	if webPath[len(webPath)-1] != '/' {
		webPath += "/"
	}

	dirPath = store.root + dirPath

	store.lazyDirs[webPath] = &LazyDir{
		DirPath: dirPath,
		WebPath: webPath,
		Active:  active,
		items:   make(map[string]*LazyFile),
	}

	scribe.PrintInfo(
		scribe.LogLevelDebug,
		"registered dir", dirPath, "on", webPath)
	return nil
}

/* UnregisterFile finds the file registered at the specified url path and
 * unregisters it, freeing it from memory
 */
func (store *Store) UnregisterFile(webPath string) (err error) {
	_, exists := store.lazyFiles[webPath]
	if !exists {
		return errors.New("path " + webPath + " is not registered")
	}
	delete(store.lazyFiles, webPath)

	scribe.PrintInfo(
		scribe.LogLevelDebug,
		"unregistered file from", webPath)
	return nil
}

/* UnregisterDir finds the directory registered at the specified url path and
 * unregisters it, freeing it from memory
 */
func (store *Store) UnregisterDir(webPath string) (err error) {
	_, exists := store.lazyDirs[webPath]
	if !exists {
		return errors.New("path " + webPath + " is not registered")
	}
	delete(store.lazyDirs, webPath)

	scribe.PrintInfo(
		scribe.LogLevelDebug,
		"unregistered dir from", webPath)
	return nil
}

/* TryHandle checks the request path against the map of registered files, and
 * serves a match if it finds it. The function returns wether it served a file
 * or not. If this function returns false, the request needs to be handled
 * still.
 */
func (store *Store) TryHandle(
	band *client.Band,
	head *protocol.FrameHTTPReqHead,
) (
	handled bool,
	err error,
) {
	// look in registered lazy files
	scribe.PrintProgress(
		scribe.LogLevelDebug,
		"looking for match in files for", head.Path)
	lazyFile, matched := store.lazyFiles[head.Path]
	if matched {
		err = lazyFile.Send(band, head, store.maxAge)
		return true, err
	}

	// look in registered lazy dirs
	scribe.PrintProgress(
		scribe.LogLevelDebug,
		"looking for match in dirs for", head.Path)

	parentDir := filepath.Dir(head.Path)
	if parentDir[len(parentDir)-1] != '/' {
		parentDir += "/"
	}
	lazyDir, matched := store.lazyDirs[parentDir]

	if matched {
		lazyFile, err = lazyDir.Find(head.Path)
		if err != nil {
			return false, err
		}
		if lazyFile == nil {
			return false, nil
		}

		err = lazyFile.Send(band, head, store.maxAge)
		return true, err
	}
	return false, nil
}

/* Returns the root path of the store. This can be helpful for doing things such
 * as registering an entire directory while doing operations on the files inside
 * of it.
 */
func (store *Store) GetRoot() (root string) {
	return store.root
}

/* SetCacheMaxAge sets the max age field of the cache-control header returned
 * when reponding to an HTTPS request.
 */
func (store *Store) SetCacheMaxAge(maxAge time.Duration) {
	store.maxAge = maxAge
}
