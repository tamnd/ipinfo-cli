package ipinfo

import (
	"context"
	"fmt"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the ipinfo driver.
type Domain struct{}

// Info describes the scheme, hostnames, and binary identity.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "ipinfo",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "ipinfo",
			Short:  "A command line for IP geolocation via ipinfo.io.",
			Long: `A command line for IP geolocation via ipinfo.io.

ipinfo looks up IP addresses against ipinfo.io over HTTPS, shapes the response
into clean records, and prints output that pipes into the rest of your tools.
No API key required for the basic tier (50k lookups/month).`,
			Site: Host,
			Repo: "https://github.com/tamnd/ipinfo-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "lookup", Group: "read", Single: true,
		Summary: "Look up an IP address (omit to look up your own IP)",
		Args:    []kit.Arg{{Name: "ip", Help: "IP address to look up (optional; omit for your own IP)", Optional: true}}}, lookupIP)
}

// newClient builds the Client from kit config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// lookupInput is the input for the lookup operation.
type lookupInput struct {
	IP     string  `kit:"arg" help:"IP address to look up (optional; omit for your own IP)"`
	Client *Client `kit:"inject"`
}

// lookupIP handles the lookup operation. If IP is empty, it looks up the
// caller's own IP via /json; otherwise it calls /{ip}/json.
func lookupIP(ctx context.Context, in lookupInput, emit func(*IPInfo) error) error {
	var (
		info *IPInfo
		err  error
	)
	if in.IP == "" {
		info, err = in.Client.Me(ctx)
	} else {
		info, err = in.Client.LookupIP(ctx, in.IP)
	}
	if err != nil {
		return err
	}
	return emit(info)
}

// Classify turns an IP address into (type, id).
func (Domain) Classify(input string) (string, string, error) {
	return "ip", input, nil
}

// Locate returns the live https URL for a (type, id).
func (Domain) Locate(t, id string) (string, error) {
	switch t {
	case "ip":
		return fmt.Sprintf("https://ipinfo.io/%s", id), nil
	default:
		return "", errs.Usage("ipinfo has no resource type %q", t)
	}
}
