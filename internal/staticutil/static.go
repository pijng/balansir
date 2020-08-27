package staticutil

import (
	"balansir/internal/configutil"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//IsStatic ...
func IsStatic(URLpath string) bool {
	staticFolder := configutil.GetConfig().StaticFolderAlias
	return strings.Contains(URLpath, staticFolder)
}

//TryServeStatic ...
func TryServeStatic(w http.ResponseWriter, URLpath string) error {
	file, contentType, err := getFileBytes(URLpath)
	if err != nil {
		return err
	}

	w.Header().Add("Content-Type", contentType)
	w.Write(file)
	return nil
}

func getFileBytes(URLpath string) ([]byte, string, error) {
	alias := configutil.GetConfig().StaticFolderAlias
	staticFolder := configutil.GetConfig().StaticFolder
	filePath := strings.Split(URLpath, alias)[1]
	path := filepath.Join(staticFolder, filePath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("file %s doesn't exist", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("error opening %s file: %s", path, err.Error())
	}
	defer file.Close()

	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, "", fmt.Errorf("error reading %s file content: %s", path, err.Error())
	}

	extension := filepath.Ext(path)
	contentType := MatchType(extension)

	return fileBytes, contentType, nil
}
