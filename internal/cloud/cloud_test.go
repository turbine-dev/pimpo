package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// fakeS3 checks every signature by signing the request it received again.
func fakeS3(t *testing.T, bucket, access, secret string) *httptest.Server {
	var mu sync.Mutex
	objects := map[string][]byte{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		when, _ := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
		again, _ := http.NewRequest(r.Method, "http://"+r.Host+r.RequestURI, bytes.NewReader(data))
		again.Header.Set("X-Amz-Content-Sha256", r.Header.Get("X-Amz-Content-Sha256"))
		again.ContentLength = r.ContentLength
		v4.NewSigner().SignHTTP(context.Background(), aws.Credentials{AccessKeyID: access, SecretAccessKey: secret}, again, r.Header.Get("X-Amz-Content-Sha256"), "s3", "auto", when)
		if again.Header.Get("Authorization") != r.Header.Get("Authorization") {
			w.WriteHeader(403)
			io.WriteString(w, `<Error><Code>SignatureDoesNotMatch</Code></Error>`)
			return
		}
		key, ok := strings.CutPrefix(r.URL.Path, "/"+bucket+"/")
		if !ok {
			w.WriteHeader(404)
			io.WriteString(w, `<Error><Code>NoSuchBucket</Code></Error>`)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "PUT":
			objects[key] = data
		case r.Method == "DELETE":
			delete(objects, key)
		case r.Method == "GET" && key == "":
			type item struct {
				Key          string
				Size         int
				LastModified time.Time
			}
			var page struct {
				XMLName  xml.Name `xml:"ListBucketResult"`
				Contents []item
			}
			for k, v := range objects {
				if strings.HasPrefix(k, r.URL.Query().Get("prefix")) {
					page.Contents = append(page.Contents, item{k, len(v), time.Now()})
				}
			}
			xml.NewEncoder(w).Encode(page)
		case r.Method == "GET":
			b, ok := objects[key]
			if !ok {
				w.WriteHeader(404)
				io.WriteString(w, `<Error><Code>NoSuchKey</Code></Error>`)
				return
			}
			w.Write(b)
		}
	}))
}

func exercise(t *testing.T, s Store) {
	ctx := context.Background()
	for _, n := range []string{"pimpo-1.pimpo", "pimpo-2.pimpo", "pimpo-3 (cópia).pimpo"} {
		if err := s.Put(ctx, n, []byte("sealed "+n)); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, o := range list {
		names = append(names, o.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "pimpo-1.pimpo,pimpo-2.pimpo,pimpo-3 (cópia).pimpo" {
		t.Fatalf("%v", names)
	}
	if b, err := s.Get(ctx, "pimpo-3 (cópia).pimpo"); err != nil || string(b) != "sealed pimpo-3 (cópia).pimpo" {
		t.Fatalf("%q %v", b, err)
	}
	if err := s.Delete(ctx, "pimpo-1.pimpo"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(ctx); len(list) != 2 {
		t.Fatalf("%v", list)
	}
}

func TestS3SignsEveryRequest(t *testing.T) {
	srv := fakeS3(t, "backups", "AKID", "s3cr3t")
	defer srv.Close()
	exercise(t, &S3{Endpoint: srv.URL, Bucket: "backups", Prefix: "casa/", AccessKey: "AKID", SecretKey: "s3cr3t"})

	wrong := &S3{Endpoint: srv.URL, Bucket: "backups", AccessKey: "AKID", SecretKey: "nope"}
	if err := wrong.Put(context.Background(), "x", nil); err == nil || !strings.Contains(err.Error(), "access key or secret") {
		t.Fatalf("wrong secret: %v", err)
	}
	missing := &S3{Endpoint: srv.URL, Bucket: "other", AccessKey: "AKID", SecretKey: "s3cr3t"}
	if _, err := missing.List(context.Background()); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing bucket: %v", err)
	}
}

func TestS3AddressesAmazonByHost(t *testing.T) {
	s := &S3{Bucket: "minha-casa", Region: "sa-east-1"}
	if u, _ := s.url("pimpo 1.pimpo", nil); u != "https://minha-casa.s3.sa-east-1.amazonaws.com/pimpo%201.pimpo" {
		t.Fatal(u)
	}
	if _, err := (&S3{Endpoint: "storage.example.com", Bucket: "bkt"}).url("x", nil); err == nil {
		t.Fatal("accepted an endpoint without a scheme")
	}
}

// Keys and backups never travel in the clear, and a bucket name cannot
// send a signed request elsewhere.
func TestS3RefusesPlainHTTPAndOddBuckets(t *testing.T) {
	for _, ep := range []string{"http://storage.example.com", "http://192.168.1.5:9000", "https://user:pw@storage.example.com"} {
		if _, err := (&S3{Endpoint: ep, Bucket: "casa"}).url("x", nil); err == nil {
			t.Fatalf("accepted %s", ep)
		}
	}
	for _, ep := range []string{"http://127.0.0.1:9000", "http://localhost:9000", "http://[::1]:9000", "https://storage.example.com"} {
		if _, err := (&S3{Endpoint: ep, Bucket: "casa"}).url("x", nil); err != nil {
			t.Fatalf("refused %s: %v", ep, err)
		}
	}
	for _, b := range []string{"evil.com#", "a", "casa/../x", "Casa", "casa..x", "192.168.1.1", "-casa", "casa?x=1", strings.Repeat("a", 64)} {
		if _, err := (&S3{Bucket: b}).url("x", nil); err == nil {
			t.Fatalf("accepted the bucket %q", b)
		}
	}
	for _, b := range []string{"minha-casa", "backups.2026", "abc"} {
		if _, err := (&S3{Bucket: b}).url("x", nil); err != nil {
			t.Fatalf("refused the bucket %q: %v", b, err)
		}
	}
	if _, err := (&S3{Endpoint: "https://s3.eu.example.com", Bucket: "My_Bucket"}).url("x", nil); err != nil {
		t.Fatalf("a compatible service's bucket: %v", err)
	}
}

type fakeDrive struct {
	mu    sync.Mutex
	files map[string]map[string]any
	n     int
	quota bool
}

func (f *fakeDrive) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer tok" {
		w.WriteHeader(401)
		return
	}
	if f.quota {
		w.WriteHeader(403)
		io.WriteString(w, `{"error":{"message":"Google Drive API has not been used in project 123 before or it is disabled.","errors":[{"reason":"accessNotConfigured"}]}}`)
		return
	}
	add := func(meta map[string]any, data []byte) string {
		f.n++
		id := fmt.Sprintf("id%d", f.n)
		meta["id"], meta["data"], meta["modifiedTime"] = id, data, time.Now().Format(time.RFC3339)
		meta["size"] = fmt.Sprint(len(data))
		f.files[id] = meta
		return id
	}
	switch {
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/upload/files"):
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		mr := multipart.NewReader(r.Body, params["boundary"])
		p, _ := mr.NextPart()
		var meta map[string]any
		json.NewDecoder(p).Decode(&meta)
		p, _ = mr.NextPart()
		data, _ := io.ReadAll(p)
		json.NewEncoder(w).Encode(map[string]string{"id": add(meta, data)})
	case r.Method == "POST" && r.URL.Path == "/api/files":
		var meta map[string]any
		json.NewDecoder(r.Body).Decode(&meta)
		json.NewEncoder(w).Encode(map[string]string{"id": add(meta, nil)})
	case r.Method == "GET" && r.URL.Path == "/api/files":
		q := r.URL.Query().Get("q")
		var out []map[string]any
		for _, m := range f.files {
			parents, _ := m["parents"].([]any)
			switch {
			case strings.Contains(q, "in parents") && len(parents) > 0 && strings.Contains(q, "'"+parents[0].(string)+"'"):
			case strings.Contains(q, "mimeType") && m["mimeType"] == folderType && strings.Contains(q, "'"+m["name"].(string)+"'"):
			default:
				continue
			}
			out = append(out, map[string]any{"id": m["id"], "name": m["name"], "size": m["size"], "modifiedTime": m["modifiedTime"]})
		}
		json.NewEncoder(w).Encode(map[string]any{"files": out})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/files/"):
		w.Write(f.files[strings.TrimPrefix(r.URL.Path, "/api/files/")]["data"].([]byte))
	case r.Method == "DELETE":
		delete(f.files, strings.TrimPrefix(r.URL.Path, "/api/files/"))
		w.WriteHeader(204)
	}
}

func TestDriveKeepsBackupsInItsFolder(t *testing.T) {
	fd := &fakeDrive{files: map[string]map[string]any{}}
	srv := httptest.NewServer(fd)
	defer srv.Close()
	tok := func(context.Context) (string, error) { return "tok", nil }
	exercise(t, &Drive{Token: tok, API: srv.URL + "/api", Upload: srv.URL + "/upload"})
	folders := 0
	for _, m := range fd.files {
		if m["mimeType"] == folderType {
			folders++
		}
	}
	if folders != 1 {
		t.Fatalf("%d folders", folders)
	}
	// A second client finds the same folder instead of making another.
	if list, err := (&Drive{Token: tok, API: srv.URL + "/api", Upload: srv.URL + "/upload", Folder: "Pimpo backups"}).List(context.Background()); err != nil || len(list) != 2 {
		t.Fatalf("%v %v", list, err)
	}

	fd.quota = true
	if err := (&Drive{Token: tok, API: srv.URL + "/api", Upload: srv.URL + "/upload"}).Put(context.Background(), "x", nil); err == nil || !strings.Contains(err.Error(), "Google Drive API") {
		t.Fatalf("%v", err)
	}
}
