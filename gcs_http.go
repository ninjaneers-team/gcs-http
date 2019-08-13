// Package p contains an HTTP Cloud Function.
package p

import (
	"cloud.google.com/go/storage"
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var baseCtx context.Context
var client *storage.Client
var bucket *storage.BucketHandle

var authUsers map[string]string

func init() {
	var err error
	baseCtx = context.Background()
	client, err = storage.NewClient(baseCtx)
	p(err, "creating client")

	bucket = client.Bucket(os.Getenv("BUCKET"))

	basicAuth := os.Getenv("BASICAUTH")
	if basicAuth != "" {
		items := strings.Split(basicAuth, " ")
		authUsers = make(map[string]string)
		for _, pair := range items {
			pair = strings.TrimSpace(pair)
			pairSplit := strings.SplitN(pair, ":", 2)
			if len(pairSplit) != 2 {
				panic("Basicauth format: 'user:pass user2:pass2...'")
			}
			authUsers[pairSplit[0]] = pairSplit[1]
		}
		log.Println("Allowed users: %+v", authUsers)
	}
}

func p(err error, why string) {
	if err != nil {
		log.Panic(why, err)
	}
}

func p2(cancel context.CancelFunc, w http.ResponseWriter, err error, why string, code int) bool {
	if err != nil {
		http.Error(w, fmt.Sprintf("%s: %s", why, err), code)
		cancel()
		return true
	}
	return false
}

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()
	if authUsers != nil && !BasicAuth(w, r, authUsers) {
		return
	}
	ServeFile(ctx, cancel, w, r)
}

func BasicAuth(w http.ResponseWriter, r *http.Request, users map[string]string) bool {
	user, expected, ok := r.BasicAuth()
	if !ok {
		//WWW-Authenticate: Basic realm="RealmName"
		w.Header().Add("WWW-Authenticate", "Basic realm=\"Please log in\"")
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	pwd, ok := users[user]
	return subtle.ConstantTimeCompare([]byte(pwd), []byte(expected)) == 1
}

func ServeFile(ctx context.Context, cancel context.CancelFunc, w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[1:]
	ctx = context.WithValue(ctx, "path", path)

	log.Println("Object path:", path)
	obj := bucket.Object(path)
	reader, err := obj.NewReader(ctx)
	if err == storage.ErrObjectNotExist {
		p2(cancel, w, err, "Not found", http.StatusNotFound)
		return
	}
	if p2(cancel, w, err, "finding object", http.StatusNotFound) {
		return
	}
	defer func() {
		err := reader.Close()
		p2(cancel, w, err, "closing output", http.StatusInternalServerError)
	}()

	_, err = io.Copy(w, reader)
	if p2(cancel, w, err, "streaming object", http.StatusInternalServerError) {
		return
	}
}
