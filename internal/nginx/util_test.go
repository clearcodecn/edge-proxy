package nginx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestUtil(t *testing.T, exec ExecFunc) *Util {
	t.Helper()
	u := New(t.TempDir(), "systemctl reload nginx")
	u.Exec = exec
	return u
}

func TestUtil_WriteFile_Atomic(t *testing.T) {
	u := newTestUtil(t, func(string, ...string) ([]byte, error) { return nil, nil })
	if err := u.WriteFile("a.conf", []byte("hello")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(u.Path("a.conf"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q", got)
	}
	// .tmp should be cleaned up after rename
	if _, err := os.Stat(u.Path("a.conf.tmp")); !os.IsNotExist(err) {
		t.Errorf("tmp leaked, err = %v", err)
	}
}

func TestUtil_Exists_RemoveFile(t *testing.T) {
	u := newTestUtil(t, func(string, ...string) ([]byte, error) { return nil, nil })
	if u.Exists("x.conf") {
		t.Error("Exists should be false before write")
	}
	_ = u.WriteFile("x.conf", []byte("data"))
	if !u.Exists("x.conf") {
		t.Error("Exists should be true after write")
	}
	if err := u.RemoveFile("x.conf"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if u.Exists("x.conf") {
		t.Error("Exists should be false after remove")
	}
	// remove missing file is a no-op
	if err := u.RemoveFile("missing.conf"); err != nil {
		t.Errorf("Remove missing should be nil, got %v", err)
	}
}

func TestUtil_TestConfig_OK(t *testing.T) {
	u := newTestUtil(t, func(name string, args ...string) ([]byte, error) {
		if name != "nginx" || len(args) != 1 || args[0] != "-t" {
			t.Errorf("unexpected exec: %s %v", name, args)
		}
		return []byte("nginx: configuration file ok"), nil
	})
	if err := u.TestConfig(); err != nil {
		t.Errorf("TestConfig: %v", err)
	}
}

func TestUtil_TestConfig_Failure(t *testing.T) {
	u := newTestUtil(t, func(name string, args ...string) ([]byte, error) {
		return []byte("syntax error near line 3"), errors.New("exit 1")
	})
	err := u.TestConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "syntax error") {
		t.Errorf("error should include stderr: %v", err)
	}
}

func TestUtil_Reload_OK(t *testing.T) {
	called := ""
	u := newTestUtil(t, func(name string, args ...string) ([]byte, error) {
		called = fmt.Sprintf("%s %s", name, strings.Join(args, " "))
		return nil, nil
	})
	if err := u.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if called != "systemctl reload nginx" {
		t.Errorf("called = %q", called)
	}
}

func TestUtil_Reload_EmptyCmd(t *testing.T) {
	u := New(t.TempDir(), "")
	if err := u.Reload(); err == nil {
		t.Fatal("expected error for empty reload cmd")
	}
}

func TestUtil_WriteAndApply_RollbackOnTestFail_NoPrior(t *testing.T) {
	calls := []string{}
	u := newTestUtil(t, func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if args[0] == "-t" {
			return []byte("bad config"), errors.New("exit 1")
		}
		return nil, nil
	})

	err := u.WriteAndApply("new.conf", []byte("server {}"))
	if err == nil {
		t.Fatal("expected error")
	}
	// file should be removed (no prior version)
	if _, err := os.Stat(u.Path("new.conf")); !os.IsNotExist(err) {
		t.Errorf("expected file removed, stat err = %v", err)
	}
	// reload must NOT have been called
	for _, c := range calls {
		if strings.Contains(c, "systemctl") {
			t.Errorf("unexpected reload call: %s", c)
		}
	}
}

func TestUtil_WriteAndApply_RollbackOnTestFail_WithPrior(t *testing.T) {
	u := newTestUtil(t, func(name string, args ...string) ([]byte, error) {
		if args[0] == "-t" {
			return []byte("bad"), errors.New("exit 1")
		}
		return nil, nil
	})
	// pre-write a "good" version
	if err := u.WriteFile("p.conf", []byte("good")); err != nil {
		t.Fatalf("prep: %v", err)
	}
	err := u.WriteAndApply("p.conf", []byte("bad-new"))
	if err == nil {
		t.Fatal("expected error")
	}
	got, err := os.ReadFile(u.Path("p.conf"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "good" {
		t.Errorf("expected rollback to good, got %q", got)
	}
}

func TestUtil_WriteAndApply_Success(t *testing.T) {
	calls := []string{}
	u := newTestUtil(t, func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	})
	if err := u.WriteAndApply("ok.conf", []byte("server {}")); err != nil {
		t.Fatalf("WriteAndApply: %v", err)
	}
	got, _ := os.ReadFile(u.Path("ok.conf"))
	if string(got) != "server {}" {
		t.Errorf("content = %q", got)
	}
	if len(calls) != 2 || !strings.Contains(calls[0], "nginx -t") || !strings.Contains(calls[1], "systemctl reload nginx") {
		t.Errorf("calls = %v", calls)
	}
}

func TestUtil_Path(t *testing.T) {
	u := New("/etc/nginx/conf.d", "x")
	want := filepath.Join("/etc/nginx/conf.d", "edge-a.com.conf")
	if got := u.Path(FileNameDomain("a.com")); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}
