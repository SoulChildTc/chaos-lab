package claims_test

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/claims"
)

func TestVerifyPortReportsListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	vs := claims.Verify(context.Background(), []string{fmt.Sprintf("port:%d", port)})
	if len(vs) != 1 || !vs[0].Checked || vs[0].Released {
		t.Fatalf("expected checked-and-not-released, got %+v", vs)
	}
	if claims.AllReleased(vs) {
		t.Fatal("a listening port must not count as released")
	}
	if r := claims.Residuals(vs); len(r) != 1 || r[0].Claim != vs[0].Claim {
		t.Fatalf("residuals=%+v", r)
	}
}

func TestVerifyPortReportsFree(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	vs := claims.Verify(context.Background(), []string{fmt.Sprintf("port:%d", port)})
	if !vs[0].Checked || !vs[0].Released {
		t.Fatalf("%+v", vs)
	}
	if !claims.AllReleased(vs) {
		t.Fatal("want released")
	}
}

func TestVerifyDirReportsExisting(t *testing.T) {
	vs := claims.Verify(context.Background(), []string{"dir:" + t.TempDir()})
	if vs[0].Released || !vs[0].Checked {
		t.Fatalf("existing dir must be a checked residual: %+v", vs)
	}
}

func TestVerifyDirReportsMissing(t *testing.T) {
	dir := t.TempDir()
	vs := claims.Verify(context.Background(), []string{"dir:" + dir + "/gone"})
	if !vs[0].Checked || !vs[0].Released {
		t.Fatalf("%+v", vs)
	}
}

func TestVerifyUnknownPrefixIsUncheckedNotReleased(t *testing.T) {
	vs := claims.Verify(context.Background(), []string{"remote:jenkins"})
	if vs[0].Checked || vs[0].Released {
		t.Fatalf("%+v", vs)
	}
	if claims.AllReleased(vs) {
		t.Fatal("unchecked claim must never count as released")
	}
	if len(claims.Residuals(vs)) != 0 {
		t.Fatal("unchecked is not a residual (we simply cannot tell)")
	}
}

func TestVerifyKindWithoutBinaryIsUnchecked(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	vs := claims.Verify(context.Background(), []string{"kind:lab-x"})
	if vs[0].Checked {
		t.Fatalf("cannot judge without kind binary, must be unchecked: %+v", vs)
	}
	if claims.AllReleased(vs) {
		t.Fatal("unchecked must not count as released")
	}
}

func TestVerifyDockerWithoutBinaryIsUnchecked(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	vs := claims.Verify(context.Background(), []string{"docker:container:lab-x"})
	if vs[0].Checked {
		t.Fatalf("%+v", vs)
	}
}

func TestVerifyMalformedPortIsUnchecked(t *testing.T) {
	vs := claims.Verify(context.Background(), []string{"port:notanumber"})
	if vs[0].Checked || vs[0].Released {
		t.Fatalf("unparseable claim must be unchecked: %+v", vs)
	}
}

func TestVerifyMixedReportsEach(t *testing.T) {
	vs := claims.Verify(context.Background(), []string{"dir:/definitely/not/here/xyz", "remote:x"})
	if len(vs) != 2 || !vs[0].Checked || vs[1].Checked {
		t.Fatalf("%+v", vs)
	}
}
