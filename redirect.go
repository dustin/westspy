package westspy

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"context"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
)

const (
	bucket      = "photo.west.spy.net"
	expDuration = time.Minute * 15
)

var (
	s3Loc = "s3.amazonaws.com"
)

func mkURL(c context.Context, awsID, awsKey, path string, exp int64) string {
	h := hmac.New(sha1.New, []byte(awsKey))
	sts := fmt.Sprintf("GET\n\n\n%v\n/%v%v", exp, bucket, path)
	_, err := h.Write([]byte(sts))
	if err != nil {
		log.Warningf(c, "Error writing to hmac: %v", err)
		return ""
	}
	auth := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("http://%s.%s%s?Signature=%s&Expires=%v&AWSAccessKeyId=%s",
		bucket, s3Loc, path, url.QueryEscape(auth), exp, awsID)
}

func remote(r *http.Request) string {
	rem := r.Header.Get("X-Forwarded-For")
	if rem == "" {
		rem = r.RemoteAddr
	}
	return rem
}

func redirect(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	exp := time.Now().Add(expDuration).Unix()
	awsID := os.Getenv("AWS_ACCESS_KEY_ID")
	awsKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	log.Infof(c, "Redirecting %v for %v with key %q", r.URL.Path[7:], remote(r), awsID)
	http.Redirect(w, r, mkURL(c, awsID, awsKey, r.URL.Path[7:], exp), 302)
}

func init() {
	http.HandleFunc("/s3sign/", redirect)
}
