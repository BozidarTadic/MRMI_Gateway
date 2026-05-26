package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"
	"time"
)

type nodeOpts struct {
	url    string
	token  string
	apiKey string
}

func parseNodeFlags(subcmd string, args []string) (nodeOpts, error) {
	fs := flag.NewFlagSet("node "+subcmd, flag.ContinueOnError)
	url := fs.String("url", "", "Node HTTP address, e.g. http://localhost:8080 (required)")
	token := fs.String("token", "", "JWT bearer token for authenticated endpoints")
	apiKey := fs.String("api-key", "", "X-MRMI-Key value for authenticated endpoints")
	if err := fs.Parse(args); err != nil {
		return nodeOpts{}, err
	}
	if *url == "" {
		return nodeOpts{}, fmt.Errorf("--url is required")
	}
	return nodeOpts{url: *url, token: *token, apiKey: *apiKey}, nil
}

func apiGet(opts nodeOpts, path string) ([]byte, error) {
	c := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, opts.url+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if opts.token != "" {
		req.Header.Set("Authorization", "Bearer "+opts.token)
	}
	if opts.apiKey != "" {
		req.Header.Set("X-MRMI-Key", opts.apiKey)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errBody) == nil && errBody.Error != "" {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, errBody.Error)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, trimNewline(body))
	}
	return body, nil
}

func cmdNode(args []string) error {
	if len(args) < 1 {
		usageNode(os.Stderr)
		return fmt.Errorf("subcommand required: status | peers | dlq | apps")
	}
	switch args[0] {
	case "status":
		return cmdNodeStatus(args[1:])
	case "peers":
		return cmdNodePeers(args[1:])
	case "dlq":
		return cmdNodeDLQ(args[1:])
	case "apps":
		return cmdNodeApps(args[1:])
	default:
		usageNode(os.Stderr)
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func cmdNodeStatus(args []string) error {
	opts, err := parseNodeFlags("status", args)
	if err != nil {
		return err
	}
	return nodeStatus(os.Stdout, opts)
}

func nodeStatus(w io.Writer, opts nodeOpts) error {
	body, err := apiGet(opts, "/api/v1/status")
	if err != nil {
		return err
	}
	var s struct {
		NodeID        string `json:"node_id"`
		Region        string `json:"region"`
		NodeScope     string `json:"node_scope"`
		Profile       string `json:"profile"`
		ApplicableLaw string `json:"applicable_law"`
		AppVersion    string `json:"app_version"`
		ADRVersion    string `json:"adr_version"`
		UptimeSecs    int64  `json:"uptime_seconds"`
	}
	if err := json.Unmarshal(body, &s); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw,"node_id:\t%s\n", s.NodeID)
	_, _ = fmt.Fprintf(tw,"region:\t%s\n", s.Region)
	_, _ = fmt.Fprintf(tw,"node_scope:\t%s\n", s.NodeScope)
	_, _ = fmt.Fprintf(tw,"profile:\t%s\n", s.Profile)
	_, _ = fmt.Fprintf(tw,"applicable_law:\t%s\n", s.ApplicableLaw)
	_, _ = fmt.Fprintf(tw,"app_version:\t%s\n", s.AppVersion)
	_, _ = fmt.Fprintf(tw,"adr_version:\t%s\n", s.ADRVersion)
	_, _ = fmt.Fprintf(tw,"uptime:\t%s\n", formatUptime(s.UptimeSecs))
	return tw.Flush()
}

func cmdNodePeers(args []string) error {
	opts, err := parseNodeFlags("peers", args)
	if err != nil {
		return err
	}
	return nodePeers(os.Stdout, opts)
}

func nodePeers(w io.Writer, opts nodeOpts) error {
	body, err := apiGet(opts, "/api/v1/peers")
	if err != nil {
		return err
	}
	var peers []struct {
		NodeID    string `json:"node_id"`
		Addr      string `json:"addr"`
		NodeScope string `json:"node_scope"`
		Region    string `json:"region"`
		Source    string `json:"source"`
	}
	if err := json.Unmarshal(body, &peers); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw,"NODE_ID\tADDR\tSCOPE\tREGION\tSOURCE\n")
	for _, p := range peers {
		_, _ = fmt.Fprintf(tw,"%s\t%s\t%s\t%s\t%s\n", p.NodeID, p.Addr, p.NodeScope, p.Region, p.Source)
	}
	return tw.Flush()
}

func cmdNodeDLQ(args []string) error {
	opts, err := parseNodeFlags("dlq", args)
	if err != nil {
		return err
	}
	return nodeDLQ(os.Stdout, opts)
}

func nodeDLQ(w io.Writer, opts nodeOpts) error {
	body, err := apiGet(opts, "/api/v1/dlq")
	if err != nil {
		return err
	}
	var entries []struct {
		Index           int    `json:"index"`
		PeerAddr        string `json:"peer_addr"`
		Attempts        int    `json:"attempts"`
		LastError       string `json:"last_error"`
		FirstSeenUnix   int64  `json:"first_seen_unix"`
		EnvelopeID      string `json:"envelope_id"`
		SenderRegion    string `json:"sender_region"`
		RecipientRegion string `json:"recipient_region"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw,"INDEX\tENVELOPE_ID\tPEER_ADDR\tATTEMPTS\tFIRST_SEEN\tLAST_ERR\n")
	for _, e := range entries {
		firstSeen := "-"
		if e.FirstSeenUnix > 0 {
			firstSeen = time.Unix(e.FirstSeenUnix, 0).UTC().Format(time.RFC3339)
		}
		_, _ = fmt.Fprintf(tw,"%d\t%s\t%s\t%d\t%s\t%s\n",
			e.Index, e.EnvelopeID, e.PeerAddr, e.Attempts, firstSeen, e.LastError)
	}
	return tw.Flush()
}

func cmdNodeApps(args []string) error {
	opts, err := parseNodeFlags("apps", args)
	if err != nil {
		return err
	}
	return nodeApps(os.Stdout, opts)
}

func nodeApps(w io.Writer, opts nodeOpts) error {
	body, err := apiGet(opts, "/api/v1/apps")
	if err != nil {
		return err
	}
	var apps []struct {
		AppID      string `json:"app_id"`
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.Unmarshal(body, &apps); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw,"APP_ID\tWEBHOOK_URL\n")
	for _, a := range apps {
		_, _ = fmt.Fprintf(tw,"%s\t%s\n", a.AppID, a.WebhookURL)
	}
	return tw.Flush()
}

func formatUptime(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func trimNewline(b []byte) string {
	s := string(b)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func usageNode(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage:
  mrmi node status --url <http-addr> [--token <jwt>] [--api-key <key>]
  mrmi node peers  --url <http-addr> [--token <jwt>] [--api-key <key>]
  mrmi node dlq    --url <http-addr> [--token <jwt>] [--api-key <key>]
  mrmi node apps   --url <http-addr> [--token <jwt>] [--api-key <key>]
`)
}
