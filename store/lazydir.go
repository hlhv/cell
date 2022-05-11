package store

import (
        "os"
        "io/ioutil"
        "path/filepath"
)

/* LazyDir is a struct which manages a directory of LazyFiles. 
 */
type LazyDir struct {
        DirPath string
        WebPath string
        Active  bool

        items map[string] *LazyFile
}

func (lazyDir *LazyDir) Find (path string) (file *LazyFile, err error) {
        if lazyDir.Active {
                return lazyDir.findActive(path)
        } else {
                return lazyDir.findLazy(path)
        }
        return
}

func (lazyDir *LazyDir) findLazy (path string) (file *LazyFile, err error) {
        if lazyDir.items == nil {
                lazyDir.items = make(map[string] *LazyFile)
                
                directory, err := ioutil.ReadDir(lazyDir.DirPath)
                if err != nil { return nil, err }

                for _, file := range(directory) {
                        if file.IsDir() { continue }
                        item := &LazyFile {
                                FilePath: lazyDir.DirPath + file.Name(),
                        }
                        lazyDir.items[lazyDir.WebPath + file.Name()] = item
                }
        }
        
        file, _ = lazyDir.items[path]
        return file, nil
}

func (lazyDir *LazyDir) findActive (path string) (file *LazyFile, err error) {
        filePath := lazyDir.DirPath + filepath.Base(path)

        fileInfo, err := os.Stat(filePath)
        if err != nil || fileInfo.IsDir() {
                delete(lazyDir.items, path)
                return nil, nil
        }

        file, exists := lazyDir.items[path]
        if exists { return file, nil }
        
        file = &LazyFile {
                FilePath:   filePath,
                AutoReload: true,
        }
        lazyDir.items[path] = file
        return file, nil
}
