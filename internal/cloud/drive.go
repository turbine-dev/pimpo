package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Drive keeps backups in a folder of the owner's Google Drive. With the
// drive.file permission Zodim sees only files it created itself.
type Drive struct {
	Token  func(ctx context.Context) (string, error)
	Folder string
	// API and Upload are Google's; tests replace them.
	API    string
	Upload string
	HTTP   *http.Client

	mu       sync.Mutex
	folderID string
}

const folderType = "application/vnd.google-apps.folder"

func (d *Drive) api() (string, string) {
	a, u := d.API, d.Upload
	if a == "" {
		a = "https://www.googleapis.com/drive/v3"
	}
	if u == "" {
		u = "https://www.googleapis.com/upload/drive/v3"
	}
	return a, u
}

func (d *Drive) folder() string {
	if d.Folder == "" {
		return "Zodim backups"
	}
	return d.Folder
}

func (d *Drive) do(ctx context.Context, method, u, contentType string, data []byte) (*http.Response, error) {
	tok, err := d.Token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client(d.HTTP).Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google Drive is unreachable: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		var e struct {
			Error struct {
				Message string `json:"message"`
				Errors  []struct {
					Reason string `json:"reason"`
				} `json:"errors"`
			} `json:"error"`
		}
		json.Unmarshal(body(resp.Body, 64<<10), &e)
		reason := ""
		if len(e.Error.Errors) > 0 {
			reason = e.Error.Errors[0].Reason
		}
		return nil, driveError(resp.StatusCode, reason, e.Error.Message)
	}
	return resp, nil
}

func driveError(status int, reason, msg string) error {
	switch {
	case reason == "accessNotConfigured" || strings.Contains(msg, "has not been used in project") || strings.Contains(msg, "is disabled"):
		return errors.New("turn on the Google Drive API in your Google Cloud project, then try again")
	case reason == "insufficientPermissions" || status == 403 && strings.Contains(msg, "scope"):
		return errors.New("Zodim may not use your Drive yet: reconnect Google in Connections and allow Drive")
	case reason == "storageQuotaExceeded":
		return errors.New("your Google Drive is full")
	case status == 401:
		return errors.New("Google signed Zodim out; reconnect Google in Connections")
	}
	return fmt.Errorf("Google Drive refused (%d): %s", status, msg)
}

func (d *Drive) search(ctx context.Context, q string) ([]driveFile, error) {
	a, _ := d.api()
	var out []driveFile
	token := ""
	for {
		v := url.Values{"q": {q}, "fields": {"nextPageToken,files(id,name,size,modifiedTime)"}, "pageSize": {"1000"}, "spaces": {"drive"}}
		if token != "" {
			v.Set("pageToken", token)
		}
		resp, err := d.do(ctx, http.MethodGet, a+"/files?"+v.Encode(), "", nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Files []driveFile `json:"files"`
			Next  string      `json:"nextPageToken"`
		}
		err = json.Unmarshal(body(resp.Body, 32<<20), &page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, page.Files...)
		if page.Next == "" {
			return out, nil
		}
		token = page.Next
	}
}

type driveFile struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Size     int64     `json:"size,string"`
	Modified time.Time `json:"modifiedTime"`
}

func quote(s string) string { return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'" }

// folderID finds the backup folder, creating it the first time.
func (d *Drive) folderIDFor(ctx context.Context) (string, error) {
	d.mu.Lock()
	id := d.folderID
	d.mu.Unlock()
	if id != "" {
		return id, nil
	}
	found, err := d.search(ctx, "name = "+quote(d.folder())+" and mimeType = '"+folderType+"' and trashed = false")
	if err != nil {
		return "", err
	}
	if len(found) > 0 {
		id = found[0].ID
	} else {
		a, _ := d.api()
		meta, _ := json.Marshal(map[string]string{"name": d.folder(), "mimeType": folderType})
		resp, err := d.do(ctx, http.MethodPost, a+"/files?fields=id", "application/json", meta)
		if err != nil {
			return "", err
		}
		var f driveFile
		json.Unmarshal(body(resp.Body, 1<<20), &f)
		resp.Body.Close()
		if f.ID == "" {
			return "", errors.New("Google Drive did not create the backup folder")
		}
		id = f.ID
	}
	d.mu.Lock()
	d.folderID = id
	d.mu.Unlock()
	return id, nil
}

func (d *Drive) files(ctx context.Context) ([]driveFile, error) {
	folder, err := d.folderIDFor(ctx)
	if err != nil {
		return nil, err
	}
	return d.search(ctx, quote(folder)+" in parents and trashed = false")
}

func (d *Drive) find(ctx context.Context, name string) (string, error) {
	files, err := d.files(ctx)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if f.Name == name {
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("%s is not in Google Drive", name)
}

func (d *Drive) Put(ctx context.Context, name string, data []byte) error {
	folder, err := d.folderIDFor(ctx)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	meta, _ := json.Marshal(map[string]any{"name": name, "parents": []string{folder}})
	h := textproto.MIMEHeader{"Content-Type": {"application/json; charset=UTF-8"}}
	p, _ := mw.CreatePart(h)
	p.Write(meta)
	p, _ = mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"application/octet-stream"}})
	p.Write(data)
	mw.Close()
	_, u := d.api()
	resp, err := d.do(ctx, http.MethodPost, u+"/files?uploadType=multipart&fields=id", "multipart/related; boundary="+mw.Boundary(), buf.Bytes())
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (d *Drive) List(ctx context.Context) ([]Object, error) {
	files, err := d.files(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Object, 0, len(files))
	for _, f := range files {
		out = append(out, Object{Name: f.Name, Size: f.Size, Modified: f.Modified})
	}
	return out, nil
}

func (d *Drive) Get(ctx context.Context, name string) ([]byte, error) {
	id, err := d.find(ctx, name)
	if err != nil {
		return nil, err
	}
	a, _ := d.api()
	resp, err := d.do(ctx, http.MethodGet, a+"/files/"+url.PathEscape(id)+"?alt=media", "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return body(resp.Body, 4<<30), nil
}

func (d *Drive) Delete(ctx context.Context, name string) error {
	id, err := d.find(ctx, name)
	if err != nil {
		return err
	}
	a, _ := d.api()
	resp, err := d.do(ctx, http.MethodDelete, a+"/files/"+url.PathEscape(id), "", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
