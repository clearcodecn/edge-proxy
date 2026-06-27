package handler

import (
	"errors"
	"testing"
)

func TestParseUpstreamLine_AddrOnly(t *testing.T) {
	got, err := ParseUpstreamLine("10.0.0.5:80")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Addr != "10.0.0.5:80" {
		t.Errorf("addr = %q", got.Addr)
	}
	if got.Weight != 1 {
		t.Errorf("default weight = %d", got.Weight)
	}
	if got.IsBackup {
		t.Error("default backup = true")
	}
	if got.Remark != "" {
		t.Errorf("default remark = %q", got.Remark)
	}
}

func TestParseUpstreamLine_AllFourFields(t *testing.T) {
	got, err := ParseUpstreamLine(`10.0.0.7:80, 2, backup, "rack-A 主力"`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Addr != "10.0.0.7:80" {
		t.Errorf("addr = %q", got.Addr)
	}
	if got.Weight != 2 {
		t.Errorf("weight = %d", got.Weight)
	}
	if !got.IsBackup {
		t.Error("backup should be true")
	}
	if got.Remark != "rack-A 主力" {
		t.Errorf("remark = %q", got.Remark)
	}
}

func TestParseUpstreamLine_BlankMiddleFields(t *testing.T) {
	got, err := ParseUpstreamLine(`10.0.0.8:8080, , , "rack-B"`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Weight != 1 || got.IsBackup || got.Remark != "rack-B" {
		t.Errorf("got = %+v, want defaults + remark rack-B", got)
	}
}

func TestParseUpstreamLine_RemarkWithCommaInQuotes(t *testing.T) {
	got, err := ParseUpstreamLine(`10.0.0.9:80, 1, main, "primary, west-1"`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Remark != "primary, west-1" {
		t.Errorf("remark = %q", got.Remark)
	}
	if got.IsBackup {
		t.Error("main should yield IsBackup=false")
	}
}

func TestParseUpstreamLine_BadAddrMissing(t *testing.T) {
	_, err := ParseUpstreamLine(", 1, backup, x")
	if err == nil {
		t.Error("missing addr should error")
	}
}

func TestParseUpstreamLine_BadWeight(t *testing.T) {
	for _, bad := range []string{"a:80, abc", "a:80, -1", "a:80, 0", "a:80, 500"} {
		_, err := ParseUpstreamLine(bad)
		if err == nil {
			t.Errorf("%q should error on weight", bad)
		}
	}
}

func TestParseUpstreamLine_BadBackup(t *testing.T) {
	_, err := ParseUpstreamLine("a:80, 1, mystery")
	if err == nil {
		t.Error("unknown backup token should error")
	}
}

func TestParseUpstreamLine_EmptyLine(t *testing.T) {
	_, err := ParseUpstreamLine("   ")
	if !errors.Is(err, errEmptyLine) {
		t.Errorf("expected errEmptyLine, got %v", err)
	}
}
