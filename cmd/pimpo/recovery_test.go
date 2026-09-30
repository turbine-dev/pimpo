package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/snapshot"
)

func freeAddr(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

// A damaged database at start is moved aside, Pimpo serves only the
// recovery page to the owner, and going back to the newest good copy
// ends it without touching the damaged file.
func TestDamagedDatabaseStartsRecovery(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()
	s, _ := event.Open(filepath.Join(home, "pimpo.db"))
	for i := 0; i < 200; i++ {
		s.Append(ctx, "note", "system", map[string]string{"text": strings.Repeat("x", 200)})
	}
	s.Put(ctx, "session_token", "owner-link")
	good, err := snapshot.Create(s.DB(), home, "daily")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	f, _ := os.OpenFile(filepath.Join(home, "pimpo.db"), os.O_WRONLY, 0)
	f.WriteAt(bytes.Repeat([]byte{0xde, 0xad}, 4096), 4096*3)
	f.Close()
	damaged, _ := os.ReadFile(filepath.Join(home, "pimpo.db"))

	if err := checkDatabase(home); err != nil {
		t.Fatal(err)
	}
	rec, ok := snapshot.Recovering(home)
	if !ok {
		t.Fatal("a damaged database did not start recovery")
	}
	if b, _ := os.ReadFile(rec.Quarantined(home)); !bytes.Equal(b, damaged) {
		t.Fatal("the damaged file was not kept as it was")
	}
	if err := snapshots("snapshot", []string{"--data", home}); err != errRecovering {
		t.Fatalf("a snapshot during recovery: %v", err)
	}

	addr := freeAddr(t)
	ended := make(chan error, 1)
	go func() { ended <- recoveryMode(ctx, home, addr, rec) }()
	base := "http://" + addr
	call := func(method, path, token string, body any) (int, map[string]any) {
		var rd *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		} else {
			rd = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, base+path, rd)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, nil
		}
		defer res.Body.Close()
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	for i := 0; i < 50; i++ {
		if code, _ := call("GET", "/api/health", "", nil); code == 200 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code, out := call("GET", "/api/state", "", nil); code != 503 || out["recovery"] != true {
		t.Fatalf("state during recovery: %d %v", code, out)
	}
	if code, _ := call("GET", "/api/recovery", "", nil); code != 401 {
		t.Fatalf("recovery without signing in: %d", code)
	}
	if code, _ := call("GET", "/api/recovery", "someone-else", nil); code != 401 {
		t.Fatalf("recovery with a wrong token: %d", code)
	}
	code, out := call("GET", "/api/recovery", "owner-link", nil)
	if code != 200 || out["newest_good"] != good.Name {
		t.Fatalf("recovery page with the owner's link: %d %v", code, out)
	}
	if code, _ := call("POST", "/api/recovery/restore", rec.Token, map[string]string{"name": good.Name}); code != 200 {
		t.Fatalf("restore: %d", code)
	}
	select {
	case err := <-ended:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recovery did not end")
	}
	if _, ok := snapshot.Recovering(home); ok {
		t.Fatal("still in recovery")
	}
	if err := event.CheckFile(filepath.Join(home, "pimpo.db")); err != nil {
		t.Fatalf("the restored database: %v", err)
	}
	if b, _ := os.ReadFile(rec.Quarantined(home)); !bytes.Equal(b, damaged) {
		t.Fatal("the damaged file was written over")
	}
}
