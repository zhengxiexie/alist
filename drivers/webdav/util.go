package webdav

import (
	"net/http"

	"github.com/alist-org/alist/v3/drivers/webdav/odrvcookie"
	"github.com/alist-org/alist/v3/pkg/gowebdav"
)

// do others that not defined in Driver interface

func (d *WebDav) isSharepoint() bool {
	return d.Vendor == "sharepoint"
}

func (d *WebDav) setClient() error {
	c := gowebdav.NewClient(d.Address, d.Username, d.Password)
	if d.isSharepoint() {
		cookie, err := odrvcookie.GetCookie(d.Username, d.Password, d.Address)
		if err == nil {
			c.SetInterceptor(func(method string, rq *http.Request) {
				rq.Header.Del("Authorization")
				rq.Header.Set("Cookie", cookie)
			})
		} else {
			return err
		}
	}
	d.client = c
	return nil
}
