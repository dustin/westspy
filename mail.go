package westspy

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	"appengine"
	aemail "appengine/mail"
	"appengine/memcache"
)

func init() {
	http.HandleFunc("/_ah/mail/", incomingMail)
	http.HandleFunc("/admin/enableMail", enableMail)
}

func slurp(c appengine.Context, r io.Reader) []byte {
	x, err := ioutil.ReadAll(r)
	if err != nil {
		c.Errorf("Error reading reader: %v", err)
	}
	return x
}

type msgExtractor struct {
	body, hbody string
	atts        []aemail.Attachment
}

func (m *msgExtractor) run(c appengine.Context, r io.Reader, boundary string) error {
	mr := multipart.NewReader(r, boundary)
	for {
		p, err := mr.NextPart()
		switch err {
		case nil:
		case io.EOF:
			return nil
		default:
			return err
		}
		if p == nil {
			return nil
		}

		c.Infof("Got part with headers: %v", p.Header)

		ctype, params, err := mime.ParseMediaType(p.Header.Get("content-type"))
		switch {
		case ctype == "multipart/alternative":
			m.run(c, p, params["boundary"])
		case m.body == "" && ctype == "text/plain":
			m.body = string(slurp(c, p))
		case m.hbody == "" && ctype == "text/html":
			m.hbody = string(slurp(c, p))
		case p.FileName() != "":
			c.Infof("Got file named %v", p.FileName())
			m.atts = append(m.atts, aemail.Attachment{
				Name:      p.FileName(),
				Data:      slurp(c, p),
				ContentID: p.Header.Get("content-id"),
			})
		}
	}
}

func (m *msgExtractor) parsePlain(c appengine.Context, r io.Reader) error {
	tr := textproto.NewReader(bufio.NewReader(r))
	_, err := tr.ReadMIMEHeader()
	if err != nil {
		return err
	}
	m.body = string(slurp(c, tr.R))
	return nil
}

func incomingMail(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	addr := strings.Split(r.URL.Path, "@")[0][len("/_ah/mail/"):]
	_, err := memcache.Get(c, "email-"+addr)
	if err != nil {
		c.Infof("Can't confirm %q is OK: %v", addr, err)
		http.Error(w, err.Error(), 403)
		return
	}

	b := &bytes.Buffer{}

	inmsg, err := mail.ReadMessage(io.TeeReader(r.Body, b))
	if err != nil {
		c.Errorf("Error parsing incoming mail: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	fullBody := b.Bytes()
	msgex := &msgExtractor{
		atts: []aemail.Attachment{
			{Name: "original.eml", Data: fullBody}},
	}

	_, params, err := mime.ParseMediaType(inmsg.Header.Get("content-type"))
	if err != nil {
		c.Errorf("Error parsing incoming mail: %v", err)
	} else {
		c.Infof("Parsing multipart with params: %v", params)
		msgex.run(c, b, params["boundary"])
	}

	if msgex.body == "" {
		c.Infof("No body found.  Sticking the full text body in.")
		msgex.parsePlain(c, bytes.NewReader(fullBody))
	}

	msg := &aemail.Message{
		Sender:      "westspy@west-spy.appspotmail.com",
		To:          []string{"dustin@spy.net"},
		Subject:     inmsg.Header.Get("subject"),
		Body:        msgex.body,
		HTMLBody:    msgex.hbody,
		Attachments: msgex.atts,
	}
	if err := aemail.Send(c, msg); err != nil {
		c.Errorf("Couldn't send email: %v", err)
	}

}

func enableMail(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html")
		templates.ExecuteTemplate(w, "mailform.html", nil)
		return
	}

	d, err := time.ParseDuration(r.FormValue("duration"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	token := &memcache.Item{
		Key:        "email-" + r.FormValue("addr"),
		Value:      []byte{},
		Expiration: d,
	}

	if err := memcache.Set(c, token); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/admin/enableMail", http.StatusFound)
}
