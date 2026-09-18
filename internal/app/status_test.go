package app

import (
	"strings"
	"testing"

	"github.com/78tacos/dnscrypt-updater/internal/check"
)

func TestTooltipAndMenu(t *testing.T) {
	t.Parallel()
	up := check.Result{LocalVersion: "2.1.14", RemoteVersion: "2.1.18", UpdateAvailable: true, LocalSource: "binary"}
	if !strings.Contains(tooltipFor(up), "2.1.18 available") {
		t.Fatal(tooltipFor(up))
	}
	if !strings.Contains(remoteMenuTitle(up), "update available") {
		t.Fatal(remoteMenuTitle(up))
	}
	ok := check.Result{LocalVersion: "2.1.18", RemoteVersion: "2.1.18", LocalSource: "override"}
	if !strings.Contains(tooltipFor(ok), "up to date") {
		t.Fatal(tooltipFor(ok))
	}
	nf := check.Result{NotFound: true}
	if !strings.Contains(tooltipFor(nf), "not found") {
		t.Fatal(tooltipFor(nf))
	}
	if statusMenuTitle(nf) != "Local: not found" {
		t.Fatal(statusMenuTitle(nf))
	}
	if !strings.Contains(assetMenuTitle(up), "see GitHub") && !strings.Contains(assetMenuTitle(check.Result{OfficialAsset: "dnscrypt-proxy-win64-2.1.18.zip"}), ".minisig") {
		t.Fatal(assetMenuTitle(up))
	}
	if got := assetMenuTitle(check.Result{OfficialAsset: "dnscrypt-proxy-win64-2.1.18.zip"}); !strings.Contains(got, "win64") {
		t.Fatal(got)
	}
	if installMenuTitle(nf) != "Install dnscrypt-proxy" {
		t.Fatal(installMenuTitle(nf))
	}
	if installMenuTitle(up) != "Update dnscrypt-proxy now" {
		t.Fatal(installMenuTitle(up))
	}
}

func TestIconsEmbedded(t *testing.T) {
	t.Parallel()
	if len(iconICO) < 16 || len(iconPNG) < 16 {
		t.Fatalf("icons missing ico=%d png=%d", len(iconICO), len(iconPNG))
	}
	if string(iconPNG[1:4]) != "PNG" {
		t.Fatal("png signature")
	}
}
