package westspy

import (
	"bytes"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"

	"appengine"
	aemail "appengine/mail"
)

func init() {
	http.HandleFunc("/_ah/mail/", incomingMail)
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

func incomingMail(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	b := &bytes.Buffer{}

	inmsg, err := mail.ReadMessage(io.TeeReader(r.Body, b))
	if err != nil {
		c.Errorf("Error parsing incoming mail: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	msgex := &msgExtractor{
		atts: []aemail.Attachment{
			{Name: "original.eml", Data: b.Bytes()}},
	}

	_, params, err := mime.ParseMediaType(inmsg.Header.Get("content-type"))
	if err != nil {
		c.Errorf("Error parsing incoming mail: %v", err)
	} else {
		msgex.run(c, b, params["boundary"])
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
