// package gcp_instances implements a plugin that returns resource records
// from GCP Cloud DNS.
package gcp_instances

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// CloudDNS is a plugin that returns RR from GCP Cloud DNS.
type GCPInstances struct {
	Next plugin.Handler
	Fall fall.F

	client    gcpInstances
	projectID string
	template  *template.Template

	zoneNames []string

	zMu      sync.RWMutex
	records  map[string][]dns.RR
	interval time.Duration
}

type instances []Instance

func New(ctx context.Context, c gcpInstances, projectID string, tpl *template.Template, interval time.Duration) (*GCPInstances, error) {
	return &GCPInstances{
		client:    c,
		template:  tpl,
		projectID: projectID,
		interval:  interval,
		records:   map[string][]dns.RR{},
	}, nil
}

// Run executes first update, spins up an update forever-loop.
// Returns error if first update fails.
func (h *GCPInstances) Run(ctx context.Context) error {
	if err := h.updateInstances(); err != nil {
		return err
	}
	go func() {
		delay := h.interval
		timer := time.NewTimer(delay)
		defer timer.Stop()
		for {
			timer.Reset(delay)
			select {
			case <-ctx.Done():
				log.Debugf("Breaking out of gcp_instances update loop: %v", ctx.Err())
				return
			case <-timer.C:
				log.Debug("Updating gcp_instances records")
				if err := h.updateInstances(); err != nil && ctx.Err() == nil /* Don't log error if ctx expired. */ {
					log.Errorf("Failed to update instances: %v", err)
				}
			}
		}
	}()
	return nil
}

// ServeDNS implements the plugin.Handler interface.
func (h *GCPInstances) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()
	log.Debugf("Received query for %s", qname)

	h.zMu.RLock()
	records := h.records[qname]
	h.zMu.RUnlock()
	if len(records) == 0 {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer = records
	var result file.Result

	if len(m.Answer) == 0 && result != file.NoData && h.Fall.Through(qname) {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	switch result {
	case file.Success:
	case file.NoData:
	case file.NameError:
		m.Rcode = dns.RcodeNameError
	case file.Delegation:
		m.Authoritative = false
	case file.ServerFailure:
		return dns.RcodeServerFailure, nil
	}

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (h *GCPInstances) Name() string { return "gcp_instances" }

func (h *GCPInstances) templateInstance(instance *Instance) (string, error) {
	buff := new(bytes.Buffer)
	err := h.template.Execute(buff, instance)
	if err != nil {
		panic(err)
	}

	return buff.String(), nil
}

func (h *GCPInstances) updateInstances() error {
	instances, err := h.client.listAllInstances(h.projectID)
	if err != nil {
		return err
	}
	newInstances := make(map[string][]dns.RR, len(instances)/2)

	for _, instance := range instances {
		domain, err := h.templateInstance(&instance)
		if err != nil {
			return err
		}
		ip := net.ParseIP(instance.IP)
		dnsRR, err := dns.NewRR(fmt.Sprintf("%s A %s", domain, ip))
		if err != nil {
			return err
		}
		log.Debugf("Found %s", dnsRR)
		newInstances[domain] = append(newInstances[domain], dnsRR)
	}

	h.zMu.Lock()
	h.records = newInstances
	h.zMu.Unlock()

	return nil
}
