package main

import (
	"cloud.google.com/go/storage"
	"context"
	"crypto/subtle"
	"fmt"
	"google.golang.org/api/option"
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
	baseCtx = context.Background()
	client = createStorageClient()

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
		log.Println("Allowed users:", authUsers)
	}
}

func createStorageClient() *storage.Client {
	options := []option.ClientOption{option.WithScopes(storage.ScopeReadOnly)}
	auth := os.Getenv("GCS_DEPLOY_SECRET")
	if auth != "" {
		options = append(options, option.WithCredentialsJSON([]byte(auth)))
	}
	client, err := storage.NewClient(baseCtx, options...)
	p(err, "creating client")
	return client
}

func main() {
	http.HandleFunc("/", ServeHTTP)
	http.HandleFunc("/_ah/health", healthCheckHandler)
	log.Print("Listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
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
		requestAuth(w)
		return false
	}
	pwd, ok := users[user]
	valid := subtle.ConstantTimeCompare([]byte(pwd), []byte(expected)) == 1
	if !valid {
		requestAuth(w)
		return false
	}
	return true
}

func requestAuth(w http.ResponseWriter) {
	w.Header().Add("WWW-Authenticate", "Basic realm=\"Please log in\"")
	w.WriteHeader(http.StatusUnauthorized)
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
	} else if p2(cancel, w, err, "Not found", http.StatusNotFound) {
		return
	}
	defer func() {
		err := reader.Close()
		if err != nil {
			log.Println("Error closing output", err)
		}
	}()

	log.Println("attrs: ", reader.Attrs)

	_, err = io.Copy(w, reader)
	if err != nil {
		log.Println("Error streaming object", err)
	}
}
