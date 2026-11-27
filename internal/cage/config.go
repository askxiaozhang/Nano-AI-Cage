// Package cage supplies the configuration and network boundary of Claude Cage.
package cage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/sylvinus/agent-vm/internal/guestprofile"
)

const ConfigEnv = "CLAUDE_CAGE_CONFIG"

type Config struct {
	GuestProfile    *guestprofile.Profile `json:"-"`
	ProxyURL        string                `json:"proxy_url"`
	ProxyRevision   string                `json:"proxy_revision,omitempty"`
	DNSURL          string                `json:"dns_url"`
	Timezone        string                `json:"timezone"`
	Locale          string                `json:"locale"`
	BrowserLanguage string                `json:"browser_language"`
	ExitPolicy      ExitPolicy            `json:"exit_policy"`
	// LANAccess lets the guest reach this machine's own local networks. It is
	// off unless the user turns it on: the environment's isolation is what the
	// rest of this package checks, and this is the one deliberate opening.
	LANAccess bool `json:"lan_access,omitempty"`
	// LANSubnets names local networks the user can reach beyond the ones this
	// computer's interfaces reveal, e.g. a lab network behind the same router.
	// One comma-separated string keeps Config comparable; every entry is an
	// IPv4 network in CIDR form.
	LANSubnets string `json:"lan_subnets,omitempty"`
	// DNS holds backup and bootstrap resolvers from dns.json beside this file.
	DNS DNSOptions `json:"-"`
	// TLSClientHelloSplit is loaded from workbench.json beside this file so
	// older versions with DisallowUnknownFields on config.json stay compatible.
	TLSClientHelloSplit bool `json:"-"`
}

func DefaultConfig() Config {
	return Config{ProxyURL: "socks5://127.0.0.1:17896", DNSURL: "https://1.1.1.1/dns-query", Timezone: "Etc/UTC", Locale: "en_US.UTF-8", BrowserLanguage: "en-US"}
}

func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("cannot read cage configuration: %w", err)
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 64<<10))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, errors.New("invalid cage JSON configuration")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("cage configuration must contain one JSON object")
	}
	c.ExitPolicy = c.ExitPolicy.Normalized()
	if err = c.Validate(); err != nil {
		return c, err
	}
	if c.DNS, err = loadDNSOptions(filepath.Dir(path)); err != nil {
		return c, err
	}
	c.TLSClientHelloSplit = loadTLSClientHelloSplit(filepath.Dir(path))
	c.GuestProfile, err = guestprofile.Load(filepath.Dir(path))
	return c, err
}

// LANEntries splits the named local networks on commas and spaces.
func LANEntries(list string) []string {
	return strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
}

// LANPrefixes parses the named local networks. Validate has already refused
// anything that would not parse.
func (c Config) LANPrefixes() []netip.Prefix {
	var prefixes []netip.Prefix
	for _, entry := range LANEntries(c.LANSubnets) {
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			prefixes = append(prefixes, prefix.Masked())
		}
	}
	return prefixes
}

// ParseLANSubnet reads one named local network: IPv4, private, and a network
// rather than a single host.
func ParseLANSubnet(entry string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(entry))
	if err != nil {
		return netip.Prefix{}, errors.New("局域网网段需为 CIDR 形式，例如 192.168.70.0/24")
	}
	prefix = prefix.Masked()
	if !prefix.Addr().Is4() || !prefix.Addr().IsPrivate() || prefix.Bits() > 24 {
		return netip.Prefix{}, errors.New("局域网网段需为私有 IPv4 网段，且不小于 /24")
	}
	return prefix, nil
}

func loadTLSClientHelloSplit(dir string) bool {
	var prefs struct {
		TLSClientHelloSplit bool `json:"tls_client_hello_split"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "workbench.json"))
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, &prefs) != nil {
		return false
	}
	return prefs.TLSClientHelloSplit
}

func (c Config) Validate() error {
	u, err := url.Parse(c.ProxyURL)
	if err != nil || u == nil {
		return errors.New("invalid proxy URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h" {
		return errors.New("proxy must use http, https or socks5")
	}
	// The proxy endpoint itself must not require the host DNS resolver.
	if net.ParseIP(u.Hostname()) == nil {
		return errors.New("proxy endpoint must be a numeric IP address; DNS bootstrap is not allowed")
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil || p < 1 || p > 65535 {
		return errors.New("proxy requires a valid explicit port")
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("proxy URL must not contain a path, query or fragment")
	}
	if _, err := validDoH(c.DNSURL); err != nil {
		return err
	}
	if err := c.DNS.Validate(); err != nil {
		return err
	}
	for _, entry := range LANEntries(c.LANSubnets) {
		if _, err := ParseLANSubnet(entry); err != nil {
			return err
		}
	}
	if c.Timezone == "" || c.Timezone == "Local" {
		return errors.New("timezone must be an explicit IANA name, e.g. Etc/UTC")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return errors.New("unknown cage timezone")
	}
	if !regexp.MustCompile(`^[A-Za-z]{2,3}_[A-Za-z]{2,3}\.UTF-8$`).MatchString(c.Locale) {
		return errors.New("locale must have the form en_US.UTF-8")
	}
	if !regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`).MatchString(c.BrowserLanguage) {
		return errors.New("invalid browser language tag")
	}
	return c.ExitPolicy.Validate()
}
