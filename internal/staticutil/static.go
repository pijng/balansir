package staticutil

import (
	"balansir/internal/configutil"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//IsStatic ...
func IsStatic(URLpath string) bool {
	staticFolder := configutil.GetConfig().StaticAlias
	return strings.Contains(URLpath, staticFolder)
}

//TryServeStatic ...
func TryServeStatic(w http.ResponseWriter, r *http.Request) error {
	path, contentType, err := getFileMeta(r.URL.Path)
	if err != nil {
		return err
	}

	w.Header().Add("Content-Type", contentType)

	http.ServeFile(w, r, path)
	return nil
}

func getFileMeta(URLpath string) (string, string, error) {
	alias := configutil.GetConfig().StaticAlias
	staticFolder := configutil.GetConfig().StaticFolder

	filePath := strings.Split(URLpath, alias)[1]
	path := filepath.Join(staticFolder, filePath)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", "", fmt.Errorf("file %s doesn't exist", path)
	}

	extension := filepath.Ext(path)
	contentType := MatchType(extension)

	return path, contentType, nil
}
