package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

var baseCtx context.Context
var client *storage.Client
var bucket *storage.BucketHandle

var authUsers map[string]string

var upstreamUrl string
var debug = false

func init() {
	baseCtx = context.Background()
	client = createStorageClient()

	bucket = client.Bucket(os.Getenv("BUCKET"))

	authUsers = DecodeBasicAuth(os.Getenv("BASICAUTH"))

	upstreamUrl = os.Getenv("UPSTREAM_URL")
	debug = strings.ToLower(os.Getenv("DEBUG")) == "true"
}

func DecodeBasicAuth(basicAuthEnv string) map[string]string {
	if basicAuthEnv != "" {
		items := strings.Split(basicAuthEnv, " ")
		output := make(map[string]string)
		for _, pair := range items {
			pair = strings.TrimSpace(pair)
			pairSplit := strings.SplitN(pair, ":", 2)
			if len(pairSplit) != 2 {
				panic("Basicauth format: 'user:pass user2:pass2...'")
			}
			output[pairSplit[0]] = pairSplit[1]
		}
		log.Println("Allowed users:", output)
		return output
	}
	return nil
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
	_, _ = fmt.Fprint(w, "ok")
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
		Debug(why, err)
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

	Debug("GET ", path)

	//TODO check bucket first and only fetch fresh if it's old
	//TODO add local caching with go-cache
	if strings.HasSuffix(path, "-SNAPSHOT/maven-metadata.xml") {
		Debug("Trying upstream first", path)
		data, err := FetchFromUpstream(path)
		if err == nil {
			Debug("Fresh from upstream", path)
			_, _ = w.Write(data)
			ctx.Done()
			return
		}
		// otherwise, try the regular path
	}

	obj := bucket.Object(path)
	reader, err := obj.NewReader(ctx)
	if err == storage.ErrObjectNotExist {

		Debug("Not found in Bucket, trying upstream", path)
		data, err := FetchFromUpstream(path)
		if p2(cancel, w, err, "Not Found, upstream: ", http.StatusNotFound) {
			return
		}

		if WriteToBucket(obj, ctx, err, data, cancel, w, path) {
			return
		}
		_, _ = w.Write(data)
		ctx.Done()
		return
	} else if p2(cancel, w, err, "Not found", http.StatusNotFound) {
		return
	} else {
		Debug("From bucket", path)
		outputReader(reader, err, w)
		ctx.Done()
	}
}

func WriteToBucket(obj *storage.ObjectHandle, ctx context.Context, err error, data []byte, cancel context.CancelFunc, w http.ResponseWriter, path string) bool {
	writer := obj.NewWriter(ctx)
	_, err = writer.Write(data)
	if p2(cancel, w, err, "Caching upstream failed", http.StatusServiceUnavailable) {
		return true
	}
	err = writer.Close()
	if p2(cancel, w, err, "Caching upstream failed", http.StatusServiceUnavailable) {
		return true
	}
	Debug("Cached upstream", path, len(data))
	return false
}

func Debug(args ...interface{}) {
	if debug {
		log.Println(args...)
	}
}

func outputReader(reader *storage.Reader, err error, w http.ResponseWriter) {
	defer func() {
		err := reader.Close()
		if err != nil {
			log.Println("Error closing output", err)
		}
	}()
	_, err = io.Copy(w, reader)
	if err != nil {
		log.Println("Error streaming object", err)
	}
}

func FetchFromUpstream(path string) ([]byte, error) {
	if upstreamUrl == "" {
		return nil, errors.New("No Upstream specified")
	}

	response, err := http.DefaultClient.Get(upstreamUrl + path)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		return nil, errors.New("Upstream: " + response.Status)
	}
	data, err := ioutil.ReadAll(response.Body)
	return data, err
}
