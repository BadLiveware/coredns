package gcp_instances

import (
	"context"
	"html/template"
	"strings"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	gcp "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

var log = clog.NewWithPlugin("gcp_instances")

func init() { plugin.Register("gcp_instances", setup) }

// exposed for testing
var f = func(ctx context.Context, opt option.ClientOption) (gcpInstances, error) {
	var err error
	var client *gcp.Service
	if opt != nil {
		client, err = gcp.NewService(ctx, opt)
	} else {
		// if credentials file is not provided in the Corefile
		// authenticate the client using env variables
		client, err = gcp.NewService(ctx)
	}
	return gcpClient{client}, err
}

func setup(c *caddy.Controller) error {
	for c.Next() {
		keyPairs := map[string]struct{}{}
		keys := map[string][]string{}

		var fall fall.F

		args := c.RemainingArgs()

		for i := 0; i < len(args); i++ {
			parts := strings.SplitN(args[i], ":", 3)
			if len(parts) != 3 {
				return plugin.Error("gcp_instances", c.Errf("invalid zone %q", args[i]))
			}
			dnsName, projectName, hostedZone := parts[0], parts[1], parts[2]
			if dnsName == "" || projectName == "" || hostedZone == "" {
				return plugin.Error("gcp_instances", c.Errf("invalid zone %q", args[i]))
			}
			if _, ok := keyPairs[args[i]]; ok {
				return plugin.Error("gcp_instances", c.Errf("conflict zone %q", args[i]))
			}

			keyPairs[args[i]] = struct{}{}
			keys[dnsName] = append(keys[dnsName], projectName+":"+hostedZone)
		}

		var opt option.ClientOption
		var tmpl *template.Template
		var projectID string
		interval := 20 * time.Second
		for c.NextBlock() {
			switch c.Val() {
			case "project":
				projectID = c.RemainingArgs()[0]
			case "template":
				tmpl = template.Must(template.New("instance").Funcs(sprig.FuncMap()).Parse(strings.Join(c.RemainingArgs(), "")))
			case "interval":
				var err error
				interval, err = time.ParseDuration(c.RemainingArgs()[0])
				if err != nil {
					return plugin.Error("gcp_instances", c.Errf("invalid interval %q", c.RemainingArgs()[0]))
				}
			case "credentials":
				if c.NextArg() {
					opt = option.WithCredentialsFile(c.Val())
				} else {
					return plugin.Error("gcp_instances", c.ArgErr())
				}
			case "fallthrough":
				fall.SetZonesFromArgs(c.RemainingArgs())
			default:
				return plugin.Error("gcp_instances", c.Errf("unknown property %q", c.Val()))
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		client, err := f(ctx, opt)
		if err != nil {
			cancel()
			return err
		}

		h, err := New(ctx, client, projectID, tmpl, interval)
		if err != nil {
			cancel()
			return plugin.Error("gcp_instances", c.Errf("failed to create plugin: %v", err))
		}
		h.Fall = fall

		if err := h.Run(ctx); err != nil {
			cancel()
			return plugin.Error("gcp_instances", c.Errf("failed to initialize plugin: %v", err))
		}

		dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
			h.Next = next
			return h
		})
		c.OnShutdown(func() error { cancel(); return nil })
	}

	return nil
}
