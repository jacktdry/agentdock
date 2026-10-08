package nexusbridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

const identityVersion = 1

type Identity struct {
	Version     int    `json:"version"`
	Endpoint    string `json:"endpoint"`
	NodeID      string `json:"node_id"`
	DeviceID    string `json:"device_id"`
	DeviceToken string `json:"device_token"`
}

// Status 是控制面板可安全展示的配对状态，不暴露 Device Token 本身。
type Status struct {
	Paired            bool   `json:"paired"`
	Endpoint          string `json:"endpoint,omitempty"`
	NodeID            string `json:"node_id,omitempty"`
	DeviceID          string `json:"device_id,omitempty"`
	DeviceTokenStored bool   `json:"device_token_stored"`
}

type PairOptions struct {
	Endpoint string
	Code     string
	Name     string
}

type pairResponse struct {
	Node struct {
		ID string `json:"id"`
	} `json:"node"`
	DeviceToken string `json:"device_token"`
}

func Pair(ctx context.Context, agentDockHome string, options PairOptions) (Identity, error) {
	return pairWithNetwork(ctx, agentDockHome, options, net.DefaultResolver.LookupNetIP, (&net.Dialer{Timeout: 10 * time.Second}).DialContext)
}

// Dependencies are per-call so offline tests never replace process-wide networking.
func pairWithNetwork(ctx context.Context, agentDockHome string, options PairOptions, lookup pairLookup, dial pairDial) (Identity, error) {
	if len(options.Code) > 256 || len(options.Name) > 256 {
		return Identity{}, errors.New("NexusDock 配对输入过长")
	}
	endpoint, err := normalizeEndpoint(options.Endpoint)
	if err != nil {
		return Identity{}, err
	}
	code := strings.TrimSpace(options.Code)
	if code == "" {
		return Identity{}, errors.New("NexusDock 配对码不能为空")
	}
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name, err = os.Hostname()
		if err != nil || strings.TrimSpace(name) == "" {
			name = "AgentDock"
		}
	}
	if len(name) > 256 {
		return Identity{}, errors.New("NexusDock 设备名称过长")
	}
	deviceID, err := newDeviceID()
	if err != nil {
		return Identity{}, err
	}
	body, err := json.Marshal(map[string]string{"code": code, "device_id": deviceID, "name": name})
	if err != nil {
		return Identity{}, fmt.Errorf("编码 NexusDock 配对请求: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/nodes/pair", bytes.NewReader(body))
	if err != nil {
		return Identity{}, errors.New("创建 NexusDock 配对请求失败")
	}
	request.Header.Set("Content-Type", "application/json")
	parsed, _ := url.Parse(endpoint)
	transport := pairTransport(parsed, lookup, dial)
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return Identity{}, errors.New("连接 NexusDock 配对接口失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return Identity{}, fmt.Errorf("NexusDock 配对失败（HTTP %d）", response.StatusCode)
	}
	const maxResponse = 64 << 10
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return Identity{}, errors.New("读取 NexusDock 配对响应失败")
	}
	if len(data) > maxResponse {
		return Identity{}, errors.New("NexusDock 配对响应过长")
	}
	var paired pairResponse
	if err := json.Unmarshal(data, &paired); err != nil {
		return Identity{}, errors.New("NexusDock 配对响应无效")
	}
	if paired.Node.ID == "" || paired.DeviceToken == "" {
		return Identity{}, errors.New("NexusDock 配对响应缺少节点身份")
	}
	identity := Identity{Version: identityVersion, Endpoint: endpoint, NodeID: paired.Node.ID, DeviceID: deviceID, DeviceToken: paired.DeviceToken}
	if err := Save(agentDockHome, identity); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

func Load(agentDockHome string) (Identity, error) {
	data, err := os.ReadFile(identityPath(agentDockHome))
	if errors.Is(err, os.ErrNotExist) {
		return Identity{}, os.ErrNotExist
	}
	if err != nil {
		return Identity{}, fmt.Errorf("读取 NexusDock 设备身份: %w", err)
	}
	var identity Identity
	if err := json.Unmarshal(data, &identity); err != nil {
		return Identity{}, fmt.Errorf("解析 NexusDock 设备身份: %w", err)
	}
	if identity.Version != identityVersion || identity.Endpoint == "" || identity.NodeID == "" || identity.DeviceID == "" || identity.DeviceToken == "" {
		return Identity{}, errors.New("NexusDock 设备身份文件无效")
	}
	return identity, nil
}

func ReadStatus(agentDockHome string) (Status, error) {
	identity, err := Load(agentDockHome)
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	return Status{
		Paired:            true,
		Endpoint:          identity.Endpoint,
		NodeID:            identity.NodeID,
		DeviceID:          identity.DeviceID,
		DeviceTokenStored: true,
	}, nil
}

func Save(agentDockHome string, identity Identity) error {
	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 NexusDock 设备身份: %w", err)
	}
	data = append(data, '\n')
	if err := atomicfile.Write(identityPath(agentDockHome), data, 0o600); err != nil {
		return fmt.Errorf("保存 NexusDock 设备身份: %w", err)
	}
	return nil
}

func identityPath(agentDockHome string) string {
	return filepath.Join(agentDockHome, "nexus", "device.json")
}

func normalizeEndpoint(value string) (string, error) {
	if len(value) > 4096 {
		return "", errors.New("NexusDock endpoint 过长")
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(value, "#") {
		return "", errors.New("NexusDock endpoint 必须是绝对 HTTP(S) 地址")
	}
	host := strings.ToLower(parsed.Hostname())
	if len(host) > 253 {
		return "", errors.New("NexusDock endpoint 主机名无效")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" || (parsed.Scheme == "https" && !publicPairIP(ip)) {
			return "", errors.New("NexusDock endpoint 地址不允许")
		}
	} else {
		if strings.ContainsAny(parsed.Host, "[]") {
			return "", errors.New("NexusDock endpoint 主机名无效")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", errors.New("NexusDock endpoint 主机名无效")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return "", errors.New("NexusDock endpoint 主机名无效")
				}
			}
		}
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return "", errors.New("NexusDock endpoint 端口无效")
	}
	if port := parsed.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("NexusDock endpoint 端口无效")
		}
	}
	// Keep ordinary hosted prefixes, but reject encoded/ambiguous routing paths.
	if parsed.RawPath != "" || strings.Contains(parsed.EscapedPath(), "%") || strings.Contains(parsed.Path, "//") {
		return "", errors.New("NexusDock endpoint 路径无效")
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return "", errors.New("NexusDock endpoint 路径无效")
		}
		for _, c := range segment {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c)) {
				return "", errors.New("NexusDock endpoint 路径无效")
			}
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	switch strings.ToLower(parsed.Scheme) {
	case "https":
	case "http":
		if host != "localhost" && net.ParseIP(host) == nil {
			return "", errors.New("公网 NexusDock endpoint 必须使用 HTTPS")
		}
		if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
			return "", errors.New("公网 NexusDock endpoint 必须使用 HTTPS")
		}
	default:
		return "", errors.New("NexusDock endpoint 必须使用 HTTPS；本机开发可使用 HTTP")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

type pairLookup func(context.Context, string, string) ([]netip.Addr, error)
type pairDial func(context.Context, string, string) (net.Conn, error)

func pairTransport(endpoint *url.URL, lookup pairLookup, dial pairDial) *http.Transport {
	return &http.Transport{
		// No environment proxy or custom TLS dialer: TLS still verifies the URL hostname.
		Proxy:                  nil,
		DisableKeepAlives:      true,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  10 * time.Second,
		MaxResponseHeaderBytes: 16 << 10,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || !strings.EqualFold(host, endpoint.Hostname()) {
				return nil, errors.New("NexusDock 配对目标无效")
			}
			ips := []netip.Addr{}
			if ip, err := netip.ParseAddr(host); err == nil {
				ips = append(ips, ip)
			} else {
				ips, err = lookup(ctx, "ip", host)
				if err != nil {
					return nil, errors.New("NexusDock 配对目标解析失败")
				}
			}
			if len(ips) == 0 {
				return nil, errors.New("NexusDock 配对目标解析失败")
			}
			for _, ip := range ips {
				allowed := publicPairIP(ip)
				if endpoint.Scheme == "http" {
					allowed = ip.IsValid() && ip.Zone() == "" && ip.Unmap().IsLoopback()
				}
				if !allowed {
					return nil, errors.New("NexusDock 配对目标地址不允许")
				}
			}
			// Dial the validated literal, never resolve the hostname a second time.
			return dial(ctx, network, net.JoinHostPort(ips[0].Unmap().String(), port))
		},
	}
}

// Conservative public-unicast policy; IPv6 transition/special-purpose ranges
// are denied because they can embed or route to nonpublic IPv4 destinations.
var blockedPairPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	// Azure's platform virtual IP is globally addressed but host-local infrastructure.
	netip.MustParsePrefix("168.63.129.16/32"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/3"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func publicPairIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || (ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip)) {
		return false
	}
	for _, prefix := range blockedPairPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func newDeviceID() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("生成 AgentDock 设备 ID: %w", err)
	}
	return "device_" + base64.RawURLEncoding.EncodeToString(raw), nil
}
