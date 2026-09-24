package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Cookie mirrors chrome.cookies.Cookie
// (https://developer.chrome.com/docs/extensions/reference/api/cookies#type-Cookie).
type Cookie struct {
	Name           string   `json:"name"`
	Value          string   `json:"value"`
	Domain         string   `json:"domain"`
	HostOnly       bool     `json:"hostOnly"`
	Path           string   `json:"path"`
	Secure         bool     `json:"secure"`
	HTTPOnly       bool     `json:"httpOnly"`
	SameSite       string   `json:"sameSite,omitempty"` // no_restriction | lax | strict | unspecified
	Session        bool     `json:"session"`
	ExpirationDate *float64 `json:"expirationDate,omitempty"`
	StoreID        string   `json:"storeId,omitempty"`
}

// CookieDump is the on-disk JSON format for cookie export/import.
type CookieDump struct {
	URL     string   `json:"url,omitempty"` // page the dump was taken from
	Cookies []Cookie `json:"cookies"`
}

// CookieSetDetails mirrors chrome.cookies.SetDetails.
type CookieSetDetails struct {
	URL            string   `json:"url"`
	Name           string   `json:"name"`
	Value          string   `json:"value"`
	Domain         string   `json:"domain,omitempty"`
	Path           string   `json:"path,omitempty"`
	Secure         bool     `json:"secure,omitempty"`
	HTTPOnly       bool     `json:"httpOnly,omitempty"`
	SameSite       string   `json:"sameSite,omitempty"`
	ExpirationDate *float64 `json:"expirationDate,omitempty"`
	StoreID        string   `json:"storeId,omitempty"`
}

// ToSetDetails converts an exported cookie back into chrome.cookies.set
// arguments. Host-only cookies must be set without a domain attribute.
func (c Cookie) ToSetDetails() (CookieSetDetails, error) {
	if c.Name == "" && c.Value == "" {
		return CookieSetDetails{}, fmt.Errorf("cookie without name and value")
	}
	host := strings.TrimPrefix(c.Domain, ".")
	if host == "" {
		return CookieSetDetails{}, fmt.Errorf("cookie %q has no domain", c.Name)
	}
	scheme := "http"
	if c.Secure {
		scheme = "https"
	}
	path := c.Path
	if path == "" {
		path = "/"
	}
	d := CookieSetDetails{
		URL:      scheme + "://" + host + path,
		Name:     c.Name,
		Value:    c.Value,
		Path:     path,
		Secure:   c.Secure,
		HTTPOnly: c.HTTPOnly,
		SameSite: c.SameSite,
		StoreID:  c.StoreID,
	}
	if !c.HostOnly {
		d.Domain = c.Domain
	}
	if !c.Session && c.ExpirationDate != nil {
		d.ExpirationDate = c.ExpirationDate
	}
	return d, nil
}

// SaveCookies writes a dump as indented JSON (0600: cookies are secrets).
func SaveCookies(path string, d CookieDump) error {
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// LoadCookies reads either a CookieDump object or a bare JSON array of cookies
// (the format produced by common "export cookies" browser extensions).
func LoadCookies(path string) (CookieDump, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CookieDump{}, err
	}
	var d CookieDump
	trim := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trim, "[") {
		err = json.Unmarshal(raw, &d.Cookies)
	} else {
		err = json.Unmarshal(raw, &d)
	}
	if err != nil {
		return CookieDump{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return d, nil
}
