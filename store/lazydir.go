package store

import (
	"github.com/hlhv/scribe"
	"io/ioutil"
	"os"
	"path/filepath"
)

/* LazyDir is a struct which manages a directory of LazyFiles.
 */
type LazyDir struct {
	DirPath string
	WebPath string
	Active  bool

	items map[string]*LazyFile
}

/* Find returns the LazyFile matching webPath, if there is one in the LazyDir.
 * If there isn't, it returns nil.
 */
func (lazyDir *LazyDir) Find(webPath string) (file *LazyFile, err error) {
	scribe.PrintProgress(scribe.LogLevelDebug, "finding "+webPath)
	if lazyDir.Active {
		return lazyDir.findActive(webPath)
	} else {
		return lazyDir.findLazy(webPath)
	}
	scribe.PrintProgress(scribe.LogLevelDebug, "found "+webPath)
	return
}

/* findLazy first checks if its contents needed to be loaded in. If they do, it
 * loads them, and then finds the file matching webPath. If it doesn't exist, it
 * will return nil.
 */
func (lazyDir *LazyDir) findLazy(
	webPath string,
) (
	file *LazyFile,
	err error,
) {
	if lazyDir.items == nil {
		scribe.PrintProgress(
			scribe.LogLevelDebug, "loading dir item list")
		lazyDir.items = make(map[string]*LazyFile)

		directory, err := ioutil.ReadDir(lazyDir.DirPath)
		if err != nil {
			return nil, err
		}

		for _, file := range directory {
			if file.IsDir() {
				continue
			}
			item := &LazyFile{
				FilePath: lazyDir.DirPath + file.Name(),
			}
			lazyDir.items[lazyDir.WebPath+file.Name()] = item
		}
		scribe.PrintDone(scribe.LogLevelDebug, "loaded")
	}

	file, _ = lazyDir.items[webPath]
	return file, nil
}

/* findActive looks fot the file matching webPath by getting its basename and
 * seeing if a file with that basename exists within itself. If it doesn't, it
 * will return nil. This function dynamically updates the items map if it finds
 * new files, or discovers old files don't exist anymore.
 */
func (lazyDir *LazyDir) findActive(
	webPath string,
) (
	file *LazyFile,
	err error,
) {
	filePath := lazyDir.DirPath + filepath.Base(webPath)

	fileInfo, err := os.Stat(filePath)
	if err != nil || fileInfo.IsDir() {
		scribe.PrintProgress(
			scribe.LogLevelDebug,
			"file doesn't exist, removing entry if it is there")
		delete(lazyDir.items, webPath)
		return nil, nil
	}

	file, exists := lazyDir.items[webPath]
	if exists {
		return file, nil
	}

	scribe.PrintProgress(
		scribe.LogLevelDebug,
		"no entry for extant file, creating")

	file = &LazyFile{
		FilePath:   filePath,
		AutoReload: true,
	}
	lazyDir.items[webPath] = file
	return file, nil
}
