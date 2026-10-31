package proxyenv

import (
	"slices"
	"testing"
)

// The proxy's variables in either case pass, and nothing else of the
// environment does.
func TestOfKeepsOnlyTheProxyInBothCases(t *testing.T) {
	env := []string{
		"HOME=/home/someone", "http_proxy=http://p:3128", "HTTPS_PROXY=http://p:3128",
		"all_proxy=socks5://p:1080", "NO_PROXY=localhost", "no_proxy=127.0.0.1",
		"Http_Proxy=odd", "PROXY=x", "ftp_proxy=f",
	}
	want := []string{
		"http_proxy=http://p:3128", "HTTPS_PROXY=http://p:3128", "all_proxy=socks5://p:1080",
		"NO_PROXY=localhost", "no_proxy=127.0.0.1",
	}
	if got := Of(env); !slices.Equal(got, want) {
		t.Errorf("Of = %q, want %q", got, want)
	}
}
