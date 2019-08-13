// Package p contains an HTTP Cloud Function.
package p

import (
	"cloud.google.com/go/storage"
	"context"
	"io"
	"log"
	"net/http"
	"os"
)

var ctx context.Context
var client *storage.Client
var bucket *storage.BucketHandle

func init() {
	var err error
	ctx = context.Background()
	client, err = storage.NewClient(ctx)
	check(err, "creating client")

	bucket = client.Bucket(os.Getenv("BUCKET"))
}

func check(err error, why string) {
	if err != nil {
		log.Panic(why, err)
	}
}

// HelloWorld prints the JSON encoded "message" field in the body
// of the request or "Hello, World!" if there isn't one.
func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[1:]
	myCtx := context.WithValue(ctx, "path", path)

	log.Println("Object path:", path)
	obj := bucket.Object(path)
	reader, err := obj.NewReader(myCtx)
	if err == storage.ErrObjectNotExist {
		http.Error(w, "Not found", 404)
		return
	}
	check(err, "finding object")
	defer reader.Close()

	_, err = io.Copy(w, reader)
	check(err, "serving data")
}
