package sandbox

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Every run is isolated the same way; these flags are the isolation.
func TestEveryRunIsIsolated(t *testing.T) {
	a := strings.Join(args("pimpo-sandbox-x", "/tmp/job", languages["python"]), " ")
	for _, want := range []string{"--rm", "--network none", "--read-only", "--cap-drop ALL", "--security-opt no-new-privileges", "--user 65534:65534",
		"--memory 512m", "--memory-swap 512m", "--cpus 1", "--pids-limit 128", "--ipc none", "-v /tmp/job:/work", "--tmpfs /tmp:rw,noexec,nosuid,size=64m"} {
		if !strings.Contains(a, want) {
			t.Errorf("missing %q in %s", want, a)
		}
	}
	for _, never := range []string{"--privileged", "--network host", "-e ", "--env", "docker.sock", "--pid host"} {
		if strings.Contains(a, never) {
			t.Errorf("has %q: %s", never, a)
		}
	}
}

func TestJobsAreChecked(t *testing.T) {
	d := Docker{Bin: "/bin/false"}
	for _, j := range []Job{
		{Language: "ruby", Code: "puts 1"},
		{Language: "python", Code: " "},
		{Language: "python", Code: "print(1)", Files: map[string]string{"../escape": "x"}},
		{Language: "python", Code: "print(1)", Files: map[string]string{"/etc/passwd": "x"}},
		{Language: "python", Code: "print(1)", Files: map[string]string{"main.py": "x"}},
		{Language: "python", Code: "print(1)", Files: map[string]string{"out/x": "x"}},
	} {
		if _, err := d.Run(context.Background(), j); err == nil {
			t.Errorf("accepted %+v", j)
		}
	}
}

// With Docker running and PIMPO_SANDBOX_LIVE=1, real programs: output,
// files both ways, no network, no writing outside the folder, a timeout.
func TestLive(t *testing.T) {
	if os.Getenv("PIMPO_SANDBOX_LIVE") == "" {
		t.Skip("set PIMPO_SANDBOX_LIVE=1 with Docker running")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("no docker")
	}
	d := Docker{}
	ctx := context.Background()
	res, err := d.Run(ctx, Job{Language: "python", Code: `import csv, json
rows = list(csv.DictReader(open("gastos.csv")))
total = sum(float(r["valor"]) for r in rows)
open("out/total.json", "w").write(json.dumps({"total": total}))
print("linhas", len(rows))`, Files: map[string]string{"gastos.csv": "item,valor\ncafe,4.5\nlivro,50\n"}})
	if err != nil || res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "linhas 2" || res.Files["total.json"] != `{"total": 54.5}` {
		t.Fatalf("%+v %v", res, err)
	}
	res, err = d.Run(ctx, Job{Language: "shell", Code: `wget -q -T 3 -O- http://example.com >/dev/null 2>&1 && echo NET; touch /etc/x 2>/dev/null && echo ROOTFS; touch /usr/x 2>/dev/null && echo USR; id -u; env | grep -c . `})
	if err != nil || strings.Contains(res.Stdout, "NET") || strings.Contains(res.Stdout, "ROOTFS") || strings.Contains(res.Stdout, "USR") || !strings.Contains(res.Stdout, "65534") {
		t.Fatalf("isolation broken: %+v %v", res, err)
	}
	res, err = d.Run(ctx, Job{Language: "shell", Code: "sleep 90"})
	if err != nil || !res.TimedOut {
		t.Fatalf("no timeout: %+v %v", res, err)
	}
}
