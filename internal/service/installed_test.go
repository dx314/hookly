package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnitArgs(t *testing.T) {
	systemd := `[Service]
ExecStart=/home/alex/go/bin/hookly "--service-mode" "--config" "/home/alex/my \"x\" dir/hookly.yaml" "--name" "homeboy"
WorkingDirectory=/home/alex/homeboy
`
	args := unitArgs(systemd, ".service")
	if got := argAfter(args, "--config"); got != `/home/alex/my "x" dir/hookly.yaml` {
		t.Errorf("systemd --config = %q", got)
	}
	if got := argAfter(args, "--name"); got != "homeboy" {
		t.Errorf("systemd --name = %q", got)
	}

	plist := `<key>ProgramArguments</key>
	<array>
		<string>/Users/alex/go/bin/hookly</string>
		<string>--service-mode</string>
		<string>--config</string>
		<string>/Users/alex/a&amp;b/hookly.yaml</string>
	</array>`
	if got := argAfter(unitArgs(plist, ".plist"), "--config"); got != "/Users/alex/a&b/hookly.yaml" {
		t.Errorf("plist --config = %q", got)
	}
}

func TestListInstalled(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("no unit files on this platform")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, ext := unitDir(true)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	unit := func(config string) string {
		if ext == ".plist" {
			return "<string>--config</string><string>" + config + "</string>"
		}
		return `ExecStart=/bin/hookly "--service-mode" "--config" "` + config + `"`
	}
	files := map[string]string{
		"hookly" + ext:         unit("/a/hookly.yaml"),
		"hookly-homeboy" + ext: unit("/homeboy/hookly.yaml"),
		"hooklyish" + ext:      unit("/x"),
		"hookly-Bad" + ext:     unit("/x"),
		"other" + ext:          unit("/x"),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ListInstalled(true)
	if err != nil {
		t.Fatal(err)
	}
	want := []Installed{
		{Name: "", Unit: "hookly", ConfigPath: "/a/hookly.yaml"},
		{Name: "homeboy", Unit: "hookly-homeboy", ConfigPath: "/homeboy/hookly.yaml"},
	}
	if len(got) != len(want) {
		t.Fatalf("ListInstalled = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListInstalled[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
