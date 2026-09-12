package reportviewer

import (
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

func TestCompiledViewerResource(t *testing.T) {
	html := HTML()
	if html != HTML() || !strings.HasPrefix(html,"<!doctype html>") || len(html)>256<<10 || URI!="ui://chartworks/report-viewer/v1" { t.Fatal("resource is not stable and bounded") }
	for _, asset:=range []string{script,styles} {
		hash:=sha256.Sum256([]byte(asset))
		if !strings.Contains(html,"'sha256-"+base64.StdEncoding.EncodeToString(hash[:])+"'") { t.Fatal("compiled asset lacks its exact CSP hash") }
	}
	for _, directive:=range []string{"default-src 'none'","connect-src 'none'","frame-src 'none'","object-src 'none'","base-uri 'none'","form-action 'none'","font-src 'none'"} { if !strings.Contains(html,directive) { t.Fatal("missing restrictive CSP",directive) } }
	for _, pattern:=range []string{`(?i)<script[^>]+src=`,`(?i)<link\b`,`(?i)@import\s`, `(?i)\bon\w+\s*=`, `(?i)\beval\s*\(`, `(?i)\bnew\s+Function\b`, `\.innerHTML\s*=`, `\.outerHTML\s*=`} {
		if regexp.MustCompile(pattern).MatchString(html) { t.Fatal("unsafe or non-self-contained compiled resource",pattern) }
	}
	for _, primitive:=range []string{"localStorage","sessionStorage","indexedDB","document.cookie","XMLHttpRequest","fetch(","Authorization:"} { if strings.Contains(script,primitive) { t.Fatal("component bypasses the authorized host bridge",primitive) } }
	for _, method:=range []string{"ui/initialize","ui/notifications/initialized","ui/notifications/tool-result","ui/notifications/host-context-changed","ui/notifications/size-changed","ui/resource-teardown","tools/call"} { if !strings.Contains(script,method) { t.Fatal("established bridge method absent",method) } }
}
