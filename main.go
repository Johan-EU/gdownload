// This code is a modified version of https://github.com/googleapis/google-api-go-client/blob/master/examples/main.go
//
// Copyright 2011 Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
package main

//go:generate go run gobin.go -init-only -var credentials -o credentials.go credentials.json

import (
	"context"
	"encoding/gob"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Flags
var (
	credentialsFile *string
	outDir          = flag.String("o", ".", "Output directory")
	cacheToken      = flag.Bool("cachetoken", false, "Cache the OAuth 2.0 token for later invocations of the program")
	debug           = flag.Bool("debug", false, "Show HTTP traffic")
)

var (
	progName    string
	credentials []byte
)

func init() {
	_, progName = filepath.Split(os.Args[0])
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [<options>] <search query> \n\nPossible options:\n", progName)
		flag.PrintDefaults()
		os.Exit(2)
	}
}

func main() {
	// Only add flag to specify credentials when it is not embedded
	if credentials == nil {
		credentialsFile = flag.String("credentials-file", "",
			"Credentials file from https://console.developers.google.com/")
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
	}
	query := flag.Arg(0)

	if credentials == nil {
		var err error
		credentials, err = ioutil.ReadFile(*credentialsFile)
		if err != nil {
			log.Fatalf("Unable to read credentials file: %v", err)
		}
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(credentials, gmail.GmailReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse credentials file to config: %v", err)
	}

	ctx := context.Background()
	if *debug {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
			Transport: &logTransport{http.DefaultTransport},
		})
	}
	c := newOAuthClient(ctx, config)
	svc, err := gmail.NewService(ctx, option.WithHTTPClient(c))
	if err != nil {
		log.Fatalf("Unable to create Gmail service: %v", err)
	}

	gmailDownloadAttachments(svc, query, *outDir)
}

func osUserCacheDir() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Caches")
	case "linux", "freebsd":
		return filepath.Join(os.Getenv("HOME"), ".cache")
	}
	log.Printf("TODO: osUserCacheDir on GOOS %q", runtime.GOOS)
	return "."
}

func tokenCacheFile(config *oauth2.Config) string {
	hash := fnv.New32a()
	hash.Write([]byte(config.ClientID))
	hash.Write([]byte(config.ClientSecret))
	hash.Write([]byte(strings.Join(config.Scopes, " ")))
	fn := fmt.Sprintf("%v-tok%v", progName, hash.Sum32())
	return filepath.Join(osUserCacheDir(), url.QueryEscape(fn))
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	if !*cacheToken {
		return nil, errors.New("--cachetoken is false")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := new(oauth2.Token)
	err = gob.NewDecoder(f).Decode(t)
	return t, err
}

func saveToken(file string, token *oauth2.Token) {
	f, err := os.Create(file)
	if err != nil {
		log.Printf("Warning: failed to cache oauth token: %v", err)
		return
	}
	defer f.Close()
	gob.NewEncoder(f).Encode(token)
	log.Printf("Saved oauth token for later use in file: %v", file)
}

func newOAuthClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile := tokenCacheFile(config)
	token, err := tokenFromFile(cacheFile)
	if err != nil {
		token = tokenFromWeb(ctx, config)
		if *cacheToken {
			saveToken(cacheFile, token)
		}
	} else {
		if *debug {
			log.Printf("Using cached token %#v from %q", token, cacheFile)
		} else {
			log.Printf("Using cached token from %q", cacheFile)
		}
	}

	return config.Client(ctx, token)
}

func tokenFromWeb(ctx context.Context, config *oauth2.Config) *oauth2.Token {
	ch := make(chan string)
	randState := fmt.Sprintf("st%d", time.Now().UnixNano())
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/favicon.ico" {
			http.Error(rw, "", 404)
			return
		}
		if req.FormValue("state") != randState {
			log.Printf("State doesn't match: req = %#v", req)
			http.Error(rw, "", 500)
			return
		}
		if code := req.FormValue("code"); code != "" {
			fmt.Fprintf(rw, "<h1>Success</h1>Authorized.")
			rw.(http.Flusher).Flush()
			ch <- code
			return
		}
		log.Printf("no code")
		http.Error(rw, "", 500)
	}))
	defer ts.Close()

	config.RedirectURL = ts.URL
	authURL := config.AuthCodeURL(randState)
	go openURL(authURL)
	log.Printf("Authorize this app at: %s", authURL)
	code := <-ch
	log.Printf("Authorized")

	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("Token exchange error: %v", err)
	}
	return token
}

func openURL(url string) {
	try := []string{"xdg-open", "google-chrome", "open"}
	for _, bin := range try {
		err := exec.Command(bin, url).Run()
		if err == nil {
			return
		}
	}
	log.Printf("Error opening URL in browser.")
}

func valueOrFileContents(value string, filename string) string {
	if value != "" {
		return value
	}
	slurp, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading %q: %v", filename, err)
	}
	return strings.TrimSpace(string(slurp))
}
