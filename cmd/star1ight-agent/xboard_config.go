package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"star1ight-agent/panelapi"
)

type xboardGenerateOptions struct {
	PanelURL        string
	PanelToken      string
	PanelNodeID     string
	PanelNodeType   string
	NodeMode        string
	VLESSNodeID     string
	HY2NodeID       string
	Out             string
	NodeTypeOutPath string
	CertPath        string
	KeyPath         string
}

func runXboardGenerateConfig(args []string) int {
	fs := flag.NewFlagSet("xboard-generate-config", flag.ContinueOnError)
	var opts xboardGenerateOptions
	fs.StringVar(&opts.PanelURL, "panel-url", "", "Panel API base URL")
	fs.StringVar(&opts.PanelToken, "panel-token", "", "Panel API node token")
	fs.StringVar(&opts.PanelNodeID, "panel-node-id", "", "Panel API node id for single-node mode")
	fs.StringVar(&opts.PanelNodeType, "panel-node-type", "", "deprecated; use --node-mode")
	fs.StringVar(&opts.NodeMode, "node-mode", "vless", "node mode: vless, hy2, ss, both")
	fs.StringVar(&opts.VLESSNodeID, "vless-node-id", "", "VLESS Reality node id; default uses --panel-node-id")
	fs.StringVar(&opts.HY2NodeID, "hy2-node-id", "", "HY2 node id; default uses --panel-node-id")
	fs.StringVar(&opts.Out, "out", "config.generated.json", "output sing-box config path")
	fs.StringVar(&opts.NodeTypeOutPath, "node-type-out", "", "optional file to write selected node mode")
	fs.StringVar(&opts.CertPath, "cert", "", "HY2 certificate path; default is cert.pem next to --out")
	fs.StringVar(&opts.KeyPath, "key", "", "HY2 private key path; default is key.pem next to --out")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if opts.PanelURL == "" || opts.PanelToken == "" || opts.Out == "" {
		fmt.Fprintln(os.Stderr, "missing required --panel-url/--panel-token/--out")
		return 2
	}
	if opts.NodeMode == "" && opts.PanelNodeType != "" {
		opts.NodeMode = normalizeNodeMode(opts.PanelNodeType)
	}
	if opts.NodeMode == "" {
		opts.NodeMode = "vless"
	}
	mode, err := generateXboardConfig(context.Background(), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("generated %s for node_mode=%s\n", opts.Out, mode)
	return 0
}

func normalizeNodeMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "vless", "vless-reality", "reality":
		return "vless"
	case "hy2", "hysteria", "hysteria2":
		return "hy2"
	case "ss", "shadowsocks", "ss2022":
		return "ss"
	case "both", "dual", "all":
		return "both"
	default:
		return strings.ToLower(strings.TrimSpace(v))
	}
}

func generateXboardConfig(ctx context.Context, opts xboardGenerateOptions) (string, error) {
	mode := normalizeNodeMode(opts.NodeMode)
	if mode == "" || mode == "auto" {
		mode = normalizeNodeMode(opts.PanelNodeType)
	}
	if mode == "" || mode == "auto" {
		mode = "vless"
	}
	var inbounds []any
	var routeConfig map[string]any
	var dnsConfig map[string]any
	var outbounds []any
	switch mode {
	case "vless":
		id := firstNonEmpty(opts.VLESSNodeID, opts.PanelNodeID)
		if id == "" {
			return "", fmt.Errorf("vless mode requires --panel-node-id or --vless-node-id")
		}
		cfg, err := panelapi.NewClient(opts.PanelURL, opts.PanelToken, id, "vless").FetchNodeConfig(ctx)
		if err != nil {
			return "", err
		}
		inbound, err := inboundFromNodeConfig(cfg, opts, defaultListen(cfg.ListenIP))
		if err != nil {
			return "", err
		}
		inbounds = append(inbounds, inbound)
		outbounds, routeConfig, dnsConfig, err = buildCustomRouting(cfg)
		if err != nil {
			return "", err
		}
	case "hy2":
		id := firstNonEmpty(opts.HY2NodeID, opts.PanelNodeID)
		if id == "" {
			return "", fmt.Errorf("hy2 mode requires --panel-node-id or --hy2-node-id")
		}
		cfg, err := panelapi.NewClient(opts.PanelURL, opts.PanelToken, id, "hysteria").FetchNodeConfig(ctx)
		if err != nil {
			return "", err
		}
		inbound, err := inboundFromNodeConfig(cfg, opts, defaultListen(cfg.ListenIP))
		if err != nil {
			return "", err
		}
		inbounds = append(inbounds, inbound)
		outbounds, routeConfig, dnsConfig, err = buildCustomRouting(cfg)
		if err != nil {
			return "", err
		}
	case "both":
		vlessID := firstNonEmpty(opts.VLESSNodeID, opts.PanelNodeID)
		hy2ID := opts.HY2NodeID
		if vlessID == "" || hy2ID == "" {
			return "", fmt.Errorf("both mode requires --vless-node-id and --hy2-node-id")
		}
		vlessCfg, err := panelapi.NewClient(opts.PanelURL, opts.PanelToken, vlessID, "vless").FetchNodeConfig(ctx)
		if err != nil {
			return "", fmt.Errorf("fetch vless node config: %w", err)
		}
		hy2Cfg, err := panelapi.NewClient(opts.PanelURL, opts.PanelToken, hy2ID, "hysteria").FetchNodeConfig(ctx)
		if err != nil {
			return "", fmt.Errorf("fetch hy2 node config: %w", err)
		}
		vlessInbound, err := inboundFromNodeConfig(vlessCfg, opts, defaultListen(vlessCfg.ListenIP))
		if err != nil {
			return "", err
		}
		hy2Inbound, err := inboundFromNodeConfig(hy2Cfg, opts, defaultListen(hy2Cfg.ListenIP))
		if err != nil {
			return "", err
		}
		inbounds = append(inbounds, vlessInbound, hy2Inbound)
		outbounds, routeConfig, dnsConfig, err = mergeCustomRouting(vlessCfg, hy2Cfg)
		if err != nil {
			return "", err
		}
	case "ss":
		id := firstNonEmpty(opts.PanelNodeID)
		if id == "" {
			return "", fmt.Errorf("ss mode requires --panel-node-id")
		}
		cfg, err := panelapi.NewClient(opts.PanelURL, opts.PanelToken, id, "shadowsocks").FetchNodeConfig(ctx)
		if err != nil {
			return "", err
		}
		inbound, err := inboundFromNodeConfig(cfg, opts, defaultListen(cfg.ListenIP))
		if err != nil {
			return "", err
		}
		inbounds = append(inbounds, inbound)
		outbounds, routeConfig, dnsConfig, err = buildCustomRouting(cfg)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("--node-mode must be vless, hy2, ss, or both")
	}
	data, err := buildSingBoxConfigFromInbounds(inbounds, outbounds, routeConfig, dnsConfig)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(opts.Out), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(opts.Out, data, 0600); err != nil {
		return "", err
	}
	if opts.NodeTypeOutPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.NodeTypeOutPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(opts.NodeTypeOutPath, []byte(mode+"\n"), 0600); err != nil {
			return "", err
		}
	}
	return mode, nil
}

func defaultListen(listen string) string {
	if listen == "" {
		return "::"
	}
	return listen
}

func buildSingBoxConfigFromInbounds(inbounds []any, customOutbounds []any, routeConfig map[string]any, dnsConfig map[string]any) ([]byte, error) {
	outbounds := []any{
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
	}
	outbounds = append(outbounds, customOutbounds...)
	route := map[string]any{"final": "direct"}
	if routeConfig != nil {
		route = routeConfig
		if _, ok := route["final"]; !ok {
			route["final"] = "direct"
		}
	}
	root := map[string]any{
		"log":       map[string]any{"level": "warn", "timestamp": true},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     route,
	}
	if len(dnsConfig) > 0 {
		root["dns"] = dnsConfig
	}
	return json.MarshalIndent(root, "", "  ")
}

func buildSingBoxConfigFromNode(cfg panelapi.NodeConfig, opts xboardGenerateOptions) ([]byte, error) {
	inbound, err := inboundFromNodeConfig(cfg, opts, defaultListen(cfg.ListenIP))
	if err != nil {
		return nil, err
	}
	outbounds, routeConfig, dnsConfig, err := buildCustomRouting(cfg)
	if err != nil {
		return nil, err
	}
	return buildSingBoxConfigFromInbounds([]any{inbound}, outbounds, routeConfig, dnsConfig)
}

func inboundFromNodeConfig(cfg panelapi.NodeConfig, opts xboardGenerateOptions, listen string) (map[string]any, error) {
	switch strings.ToLower(cfg.Protocol) {
	case "vless":
		return vlessInboundFromNodeConfig(cfg, listen)
	case "hysteria", "hysteria2":
		if cfg.Version != 0 && cfg.Version != 2 {
			return nil, fmt.Errorf("unsupported hysteria version %d", cfg.Version)
		}
		return hy2InboundFromNodeConfig(cfg, opts, listen)
	case "shadowsocks":
		return shadowsocksInboundFromNodeConfig(cfg, listen)
	default:
		return nil, fmt.Errorf("unsupported node protocol %q", cfg.Protocol)
	}
}

func vlessInboundFromNodeConfig(cfg panelapi.NodeConfig, listen string) (map[string]any, error) {
	if cfg.ServerPort <= 0 {
		return nil, fmt.Errorf("vless server_port is missing")
	}
	if cfg.TLSSettings.PrivateKey == "" {
		return nil, fmt.Errorf("vless reality private_key is missing")
	}
	serverName := firstNonEmpty(cfg.TLSSettings.ServerName, "www.microsoft.com")
	handshakePort := 443
	if cfg.TLSSettings.ServerPort.String() != "" {
		if p, err := strconv.Atoi(cfg.TLSSettings.ServerPort.String()); err == nil && p > 0 {
			handshakePort = p
		}
	}
	shortID := cfg.TLSSettings.ShortID
	shortIDs := []string{}
	if shortID != "" {
		shortIDs = []string{shortID}
	}
	in := map[string]any{
		"type":        "vless",
		"tag":         "vless-in",
		"listen":      listen,
		"listen_port": cfg.ServerPort,
		"users":       []any{},
		"tls": map[string]any{
			"enabled":     true,
			"server_name": serverName,
			"reality": map[string]any{
				"enabled":     true,
				"handshake":   map[string]any{"server": serverName, "server_port": handshakePort},
				"private_key": cfg.TLSSettings.PrivateKey,
				"short_id":    shortIDs,
			},
		},
	}
	return in, nil
}

func hy2InboundFromNodeConfig(cfg panelapi.NodeConfig, opts xboardGenerateOptions, listen string) (map[string]any, error) {
	if cfg.ServerPort <= 0 {
		return nil, fmt.Errorf("hysteria2 server_port is missing")
	}
	outDir := filepath.Dir(opts.Out)
	certPath := opts.CertPath
	if certPath == "" {
		certPath = filepath.Join(outDir, "cert.pem")
	}
	keyPath := opts.KeyPath
	if keyPath == "" {
		keyPath = filepath.Join(outDir, "key.pem")
	}
	serverName := firstNonEmpty(cfg.ServerName, cfg.TLSSettings.ServerName, "bing.com")
	if err := ensureSelfSignedCert(certPath, keyPath, serverName); err != nil {
		return nil, err
	}
	in := map[string]any{
		"type":        "hysteria2",
		"tag":         "hy2-in",
		"listen":      listen,
		"listen_port": cfg.ServerPort,
		"users":       []any{},
		"tls": map[string]any{
			"enabled":          true,
			"server_name":      serverName,
			"certificate_path": certPath,
			"key_path":         keyPath,
		},
	}
	if cfg.UpMbps > 0 {
		in["up_mbps"] = cfg.UpMbps
	}
	if cfg.DownMbps > 0 {
		in["down_mbps"] = cfg.DownMbps
	}
	if cfg.Obfs != "" && cfg.ObfsPassword != "" {
		in["obfs"] = map[string]any{"type": cfg.Obfs, "password": cfg.ObfsPassword}
	}
	return in, nil
}

func shadowsocksInboundFromNodeConfig(cfg panelapi.NodeConfig, listen string) (map[string]any, error) {
	if cfg.ServerPort <= 0 {
		return nil, fmt.Errorf("shadowsocks server_port is missing")
	}
	if strings.TrimSpace(cfg.Cipher) == "" {
		return nil, fmt.Errorf("shadowsocks cipher is missing")
	}
	if strings.TrimSpace(cfg.ServerKey) == "" {
		return nil, fmt.Errorf("shadowsocks server_key is missing")
	}
	in := map[string]any{
		"type":        "shadowsocks",
		"tag":         "ss-in",
		"listen":      listen,
		"listen_port": cfg.ServerPort,
		"method":      cfg.Cipher,
		"password":    cfg.ServerKey,
		"managed":     true,
	}
	if networks := normalizeInboundNetworks(cfg.Network); len(networks) > 0 {
		in["network"] = networks
	}
	return in, nil
}

func ensureSelfSignedCert(certPath, keyPath, serverName string) error {
	if certPath == "" || keyPath == "" {
		return fmt.Errorf("cert/key path is empty")
	}
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	now := time.Now()
	tpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: serverName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(serverName); ip != nil {
		tpl.IPAddresses = []net.IP{ip}
	} else {
		tpl.DNSNames = []string{serverName}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		certFile.Close()
		return err
	}
	if err := certFile.Close(); err != nil {
		return err
	}
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		keyFile.Close()
		return err
	}
	return keyFile.Close()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func normalizeInboundNetworks(raw string) []string {
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.ToLower(strings.TrimSpace(field))
		if field == "" {
			continue
		}
		if field == "tcp" || field == "udp" {
			out = append(out, field)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeCustomRouting(configs ...panelapi.NodeConfig) ([]any, map[string]any, map[string]any, error) {
	var outbounds []any
	var rules []any
	var routeOptions map[string]any
	var dnsConfig map[string]any
	seenTags := map[string]struct{}{
		"direct": {},
		"block":  {},
	}

	for _, cfg := range configs {
		if dnsConfig == nil && len(cfg.CustomDNS) > 0 {
			dnsConfig = cloneAnyMap(cfg.CustomDNS)
		}
		if routeOptions == nil && len(cfg.RouteOptions) > 0 {
			routeOptions = cloneAnyMap(cfg.RouteOptions)
		}
		for _, raw := range cfg.CustomOutbounds {
			built, tag, err := customOutboundToSingBox(raw)
			if err != nil {
				return nil, nil, nil, err
			}
			key := strings.ToLower(strings.TrimSpace(tag))
			if _, exists := seenTags[key]; exists {
				continue
			}
			seenTags[key] = struct{}{}
			outbounds = append(outbounds, built)
		}
	}

	for _, cfg := range configs {
		builtRules, err := buildStructuredRouteRules(cfg.CustomRouteRules, seenTags)
		if err != nil {
			return nil, nil, nil, err
		}
		rules = append(rules, builtRules...)
		if len(cfg.CustomRoutes) > 0 {
			for _, raw := range cfg.CustomRoutes {
				if maybeDNS, ok := raw["dns"].(map[string]any); ok && dnsConfig == nil {
					dnsConfig = cloneAnyMap(maybeDNS)
					continue
				}
				if maybeOptions, ok := raw["route_options"].(map[string]any); ok && routeOptions == nil {
					routeOptions = cloneAnyMap(maybeOptions)
					continue
				}
				rules = append(rules, raw)
			}
		}
	}

	if len(rules) == 0 && len(routeOptions) == 0 {
		return outbounds, nil, dnsConfig, nil
	}
	route := cloneAnyMap(routeOptions)
	if route == nil {
		route = map[string]any{}
	}
	if len(rules) > 0 {
		route["rules"] = rules
	}
	if _, ok := route["final"]; !ok {
		route["final"] = "direct"
	}
	return outbounds, route, dnsConfig, nil
}

func buildCustomRouting(cfg panelapi.NodeConfig) ([]any, map[string]any, map[string]any, error) {
	return mergeCustomRouting(cfg)
}

func customOutboundToSingBox(cfg panelapi.OutboundConfig) (map[string]any, string, error) {
	tag := strings.TrimSpace(cfg.Tag)
	if tag == "" {
		return nil, "", fmt.Errorf("custom_outbounds.tag is required")
	}
	protocol := normalizeOutboundProtocol(cfg.Protocol)
	if protocol == "" {
		return nil, "", fmt.Errorf("custom_outbound %q protocol is required", tag)
	}
	settings := cloneAnyMap(cfg.Settings)
	if len(settings) == 0 {
		return nil, "", fmt.Errorf("custom_outbound %q settings are required", tag)
	}
	settings["type"] = protocol
	settings["tag"] = tag
	if proxyTag := strings.TrimSpace(cfg.ProxyTag); proxyTag != "" {
		settings["detour"] = proxyTag
	}
	return settings, tag, nil
}

func normalizeOutboundProtocol(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "hy2", "hysteria", "hysteria2":
		return "hysteria2"
	default:
		return strings.ToLower(strings.TrimSpace(v))
	}
}

func buildStructuredRouteRules(rules []panelapi.CustomRouteRule, availableTags map[string]struct{}) ([]any, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	out := make([]any, 0, len(rules))
	for _, rule := range rules {
		if rule.Disabled {
			continue
		}
		built, err := customRouteRuleToSingBox(rule, availableTags)
		if err != nil {
			return nil, err
		}
		if built != nil {
			out = append(out, built)
		}
	}
	return out, nil
}

func customRouteRuleToSingBox(rule panelapi.CustomRouteRule, availableTags map[string]struct{}) (map[string]any, error) {
	match := map[string]any{}
	appendNonEmptyStringSlice(match, "domain", rule.Match.Domains)
	appendNonEmptyStringSlice(match, "domain_suffix", rule.Match.DomainSuffixes)
	appendNonEmptyStringSlice(match, "ip_cidr", rule.Match.IPCIDRs)
	appendNonEmptyStringSlice(match, "source_ip_cidr", rule.Match.SourceCIDRs)
	appendNonEmptyStringSlice(match, "network", rule.Match.Networks)
	appendPorts(match, "port", rule.Match.Ports)
	appendPorts(match, "source_port", rule.Match.SourcePorts)
	if len(match) == 0 {
		return nil, nil
	}

	actionType := strings.ToLower(strings.TrimSpace(rule.Action.Type))
	switch actionType {
	case "", "route":
		target := strings.TrimSpace(rule.Action.Target)
		if target == "" {
			return nil, fmt.Errorf("custom route %q route action requires target", rule.Name)
		}
		if availableTags != nil {
			if _, ok := availableTags[strings.ToLower(target)]; !ok {
				return nil, fmt.Errorf("custom route %q references unknown outbound %q", rule.Name, target)
			}
		}
		match["outbound"] = target
	case "direct":
		match["outbound"] = "direct"
	case "block":
		match["outbound"] = "block"
	default:
		return nil, fmt.Errorf("custom route %q action %q is not supported", rule.Name, rule.Action.Type)
	}
	return match, nil
}

func appendNonEmptyStringSlice(dst map[string]any, key string, values []string) {
	if len(values) == 0 {
		return
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return
	}
	dst[key] = out
}

func appendPorts(dst map[string]any, key string, values []string) {
	if len(values) == 0 {
		return
	}
	numericPorts := make([]uint16, 0, len(values))
	stringPorts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.Contains(value, "-") || strings.Contains(value, ":") {
			stringPorts = append(stringPorts, value)
			continue
		}
		port, err := strconv.ParseUint(value, 10, 16)
		if err != nil {
			stringPorts = append(stringPorts, value)
			continue
		}
		numericPorts = append(numericPorts, uint16(port))
	}
	if len(stringPorts) == 0 {
		if len(numericPorts) == 1 {
			dst[key] = numericPorts[0]
		} else if len(numericPorts) > 1 {
			dst[key] = numericPorts
		}
		return
	}
	if len(numericPorts) == 0 {
		if len(stringPorts) == 1 {
			dst[key] = stringPorts[0]
		} else {
			dst[key] = stringPorts
		}
		return
	}
	combined := make([]any, 0, len(numericPorts)+len(stringPorts))
	for _, port := range numericPorts {
		combined = append(combined, port)
	}
	for _, value := range stringPorts {
		combined = append(combined, value)
	}
	dst[key] = combined
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
